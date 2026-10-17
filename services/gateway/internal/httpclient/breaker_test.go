package httpclient

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"lab2/gateway/internal/models/entities"
)

func TestBreakerThresholdAndProbe(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		b := NewBreaker(BreakerConfig{FailureThreshold: 5, ResetTimeout: time.Second})

		var calls atomic.Int32
		fail := func() error { calls.Add(1); return entities.ErrUpstreamUnavailable }
		for range 6 {
			_ = executeRead(b, t.Context(), fail)
		}
		if calls.Load() != 5 {
			t.Fatalf("calls = %d, want 5", calls.Load())
		}
		time.Sleep(time.Second + time.Nanosecond)
		entered, release := make(chan struct{}), make(chan struct{})
		var wg sync.WaitGroup
		wg.Go(func() {
			_ = executeRead(b, t.Context(), func() error { calls.Add(1); close(entered); <-release; return nil })
		})
		<-entered
		var rejected sync.WaitGroup
		for range 20 {
			rejected.Go(func() { _ = executeRead(b, t.Context(), fail) })
		}
		rejected.Wait()
		if calls.Load() != 6 {
			t.Fatalf("concurrent probe calls = %d, want 6", calls.Load())
		}
		close(release)
		wg.Wait()
		_ = executeRead(b, t.Context(), func() error { calls.Add(1); return nil })
		if calls.Load() != 7 {
			t.Fatalf("recovery calls = %d", calls.Load())
		}
	})
}

func TestBreakerLateResponseAndClassification(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		b := NewBreaker(BreakerConfig{FailureThreshold: 1, ResetTimeout: time.Second})
		entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
		go func() {
			defer close(done)
			_ = executeRead(b, t.Context(), func() error { close(entered); <-release; return nil })
		}()
		<-entered
		_ = executeRead(b, t.Context(), func() error { return entities.ErrUpstreamTimeout })
		close(release)
		<-done
		called := false
		_ = executeRead(b, t.Context(), func() error { called = true; return nil })
		if called {
			t.Fatal("late success closed open breaker")
		}
		for _, cause := range []error{
			entities.ErrInvalidInput, entities.ErrNotFound, context.Canceled,
			entities.NewOperationPanic(entities.ErrUpstreamUnavailable, nil),
		} {
			isolated := NewBreaker(BreakerConfig{FailureThreshold: 1, ResetTimeout: time.Second})
			_ = executeRead(isolated, t.Context(), func() error { return cause })
			calls := 0
			if err := executeRead(isolated, t.Context(), func() error { calls++; return nil }); err != nil || calls != 1 {
				t.Fatalf("nonavailability error %v opened breaker", cause)
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		isolated := NewBreaker(BreakerConfig{FailureThreshold: 1, ResetTimeout: time.Second})
		err := executeRead(isolated, ctx, func() error {
			cancel()
			return errors.Join(entities.ErrUpstreamTimeout, ctx.Err())
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if err := executeRead(isolated, t.Context(), func() error { return nil }); err != nil {
			t.Fatal(err)
		}
	})
}

func TestBreakerFailedProbeReopens(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		b := NewBreaker(BreakerConfig{FailureThreshold: 1, ResetTimeout: time.Second})

		fail := func() error { return entities.ErrUpstreamUnavailable }
		_ = executeRead(b, t.Context(), fail)
		time.Sleep(time.Second + time.Nanosecond)
		_ = executeRead(b, t.Context(), fail)
		called := false
		_ = executeRead(b, t.Context(), func() error { called = true; return nil })
		if called {
			t.Fatal("failed probe did not reopen")
		}
	})
}

func executeRead(b *Breaker, ctx context.Context, call func() error) error {
	_, err := Read(ctx, b, entities.ErrPaymentUnavailable, func() (struct{}, error) {
		return struct{}{}, call()
	})
	return err
}

func TestBreakerCanceledProbeAllowsAnotherProbe(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		b := NewBreaker(BreakerConfig{FailureThreshold: 1, ResetTimeout: time.Second})
		_ = executeRead(b, t.Context(), func() error { return entities.ErrUpstreamUnavailable })
		time.Sleep(time.Second + time.Nanosecond)
		ctx, cancel := context.WithCancel(t.Context())
		_ = executeRead(b, ctx, func() error {
			cancel()
			return errors.Join(entities.ErrUpstreamTimeout, ctx.Err())
		})
		called := false
		err := executeRead(b, t.Context(), func() error { called = true; return nil })
		if err != nil || !called || b.State() != stateClosed {
			t.Fatalf("probe after cancellation: called=%v, state=%v, err=%v", called, b.State(), err)
		}
	})
}

func TestBreakerClientTimeoutCountsAsFailure(t *testing.T) {
	t.Parallel()
	b := NewBreaker(BreakerConfig{FailureThreshold: 1, ResetTimeout: time.Second})
	_ = executeRead(b, t.Context(), func() error {
		return errors.Join(entities.ErrUpstreamTimeout, context.DeadlineExceeded)
	})
	err := executeRead(b, t.Context(), func() error { t.Fatal("open breaker called upstream"); return nil })
	if !errors.Is(err, entities.ErrPaymentUnavailable) {
		t.Fatalf("missing service availability error: %v", err)
	}
}

func TestBreakerRingRequiresConsecutiveFailures(t *testing.T) {
	t.Parallel()
	b := NewBreaker(BreakerConfig{FailureThreshold: 5, ResetTimeout: time.Second})
	fail := func() error { return entities.ErrUpstreamUnavailable }
	succeed := func() error { return nil }
	for range 3 {
		for range 4 {
			_ = executeRead(b, t.Context(), fail)
		}
		if err := executeRead(b, t.Context(), succeed); err != nil {
			t.Fatalf("nonconsecutive failures opened breaker: %v", err)
		}
	}
	for range 5 {
		_ = executeRead(b, t.Context(), fail)
	}
	if err := executeRead(b, t.Context(), func() error {
		t.Fatal("full failure ring called upstream")
		return nil
	}); !errors.Is(err, entities.ErrPaymentUnavailable) {
		t.Fatalf("full failure ring did not open breaker: %v", err)
	}
}
