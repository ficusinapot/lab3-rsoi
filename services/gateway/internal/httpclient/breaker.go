package httpclient

import (
	"context"
	"errors"
	"sync"
	"time"

	"lab2/gateway/internal/models/entities"

	"github.com/samber/oops"
)

type BreakerConfig struct {
	FailureThreshold int           `mapstructure:"failure_threshold"`
	ResetTimeout     time.Duration `mapstructure:"reset_timeout"`
}

type BreakerState uint8

const (
	stateClosed BreakerState = iota
	stateOpen
	stateHalfOpen
)

type Breaker struct {
	mu           sync.Mutex
	failures     []bool
	next         int
	failureCount int
	state        BreakerState
	generation   uint64
	retryAt      time.Time
	resetTimeout time.Duration
	probing      bool
}

func NewBreaker(cfg BreakerConfig) *Breaker {
	return &Breaker{failures: make([]bool, cfg.FailureThreshold), resetTimeout: cfg.ResetTimeout}
}

func (b *Breaker) State() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *Breaker) Allow() (func(error), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == stateOpen {
		if time.Now().Before(b.retryAt) {
			return nil, oops.Wrap(entities.ErrUpstreamUnavailable)
		}
		b.state = stateHalfOpen
		b.generation++
	}
	if b.state == stateHalfOpen {
		if b.probing {
			return nil, oops.Wrap(entities.ErrUpstreamUnavailable)
		}
		b.probing = true
	}
	generation := b.generation
	var once sync.Once
	return func(err error) { once.Do(func() { b.complete(generation, err) }) }, nil
}

func (b *Breaker) complete(generation uint64, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if generation != b.generation {
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		b.probing = false
		return
	}
	failed := err != nil
	if b.state == stateHalfOpen {
		b.probing = false
		if !failed {
			b.state = stateClosed
			b.generation++
			clear(b.failures)
			b.next, b.failureCount = 0, 0
			return
		}
	} else {
		if b.failures[b.next] {
			b.failureCount--
		}
		b.failures[b.next] = failed
		if failed {
			b.failureCount++
		}
		b.next = (b.next + 1) % len(b.failures)
		if b.failureCount < len(b.failures) {
			return
		}
	}
	b.state = stateOpen
	b.generation++
	b.retryAt = time.Now().Add(b.resetTimeout)
}

func Read[T any](
	ctx context.Context, b *Breaker, service error, call func() (T, error),
) (result T, err error) {
	if err = ctx.Err(); err != nil {
		return result, oops.Wrap(err)
	}
	done, err := b.Allow()
	if err != nil {
		return result, ServiceError(ctx, service, entities.ErrUpstreamUnavailable)
	}
	defer func() {
		switch {
		case ctx.Err() != nil:
			done(ctx.Err())
		case entities.Unavailable(ctx, err):
			done(entities.ErrUpstreamUnavailable)
		default:
			done(nil)
		}
	}()
	result, err = call()
	return result, ServiceError(ctx, service, err)
}

func ServiceError(ctx context.Context, service, err error) error {
	if entities.Unavailable(ctx, err) {
		return errors.Join(service, err)
	}
	return err
}
