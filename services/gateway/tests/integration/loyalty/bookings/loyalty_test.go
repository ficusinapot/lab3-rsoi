//go:build integration

package bookings

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samber/oops"
	"lab2/gateway/internal/httpclient"
	loyaltyrepo "lab2/gateway/internal/loyalty/repos"
	"lab2/gateway/internal/models/entities"
)

func TestHTTPClient(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-User-Name") != "user" {
			t.Error("missing user header")
		}
		if r.URL.Path != "/loyalty" {
			t.Error(r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"status":"BRONZE","discount":5,"reservationCount":0}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	c := newLoyalty(t, server.URL, time.Second)
	item, err := c.Loyalty(t.Context(), "user")
	if err != nil || item.Discount != 5 || item.ReservationCount != 0 {
		t.Fatalf("%+v %v", item, err)
	}
}

func TestUpstreamErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		code int
		want error
		body string
	}{
		{
			name: "bad request",
			code: http.StatusBadRequest,
			want: entities.ErrInvalidInput,
		},
		{
			name: "unprocessable input",
			code: http.StatusUnprocessableEntity,
			want: entities.ErrInvalidInput,
		},
		{
			name: "not found",
			code: http.StatusNotFound,
			want: entities.ErrNotFound,
		},
		{
			name: "incomplete JSON",
			code: http.StatusOK,
			want: entities.ErrUpstreamUnavailable,
			body: `{ "status":`,
		},
		{
			name: "server error",
			code: http.StatusInternalServerError,
			want: entities.ErrUpstreamUnavailable,
			body: "secret database error",
		},
		{
			name: "invalid JSON",
			code: http.StatusOK,
			want: entities.ErrUpstreamUnavailable,
			body: "invalid JSON",
		},
		{
			name: "multiple JSON values",
			code: http.StatusOK,
			want: entities.ErrUpstreamUnavailable,
			body: `{"status":"BRONZE"}{"status":"GOLD"}`,
		},
		{
			name: "trailing data",
			code: http.StatusOK,
			want: entities.ErrUpstreamUnavailable,
			body: `{"status":"BRONZE"} trailing`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			_, err := newLoyalty(t, server.URL, time.Second).Loyalty(t.Context(), "user")
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestTimeoutAndCancellation(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	c := newLoyalty(t, server.URL, 20*time.Millisecond)
	_, err := c.Loyalty(t.Context(), "user")
	if !errors.Is(err, entities.ErrUpstreamTimeout) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = c.Loyalty(ctx, "user")
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestNoRedirect(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/other", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := newLoyalty(t, server.URL, time.Second).Loyalty(t.Context(), "user")
	if !errors.Is(err, entities.ErrUpstreamUnavailable) {
		t.Fatal(oops.Wrap(err))
	}
}

func newLoyalty(t *testing.T, address string, timeout time.Duration) *loyaltyrepo.Repository {
	t.Helper()
	client := httpclient.New(httpclient.Config{
		URL: address, Timeout: timeout,
		MaxInFlight: 2, MaxIdleConnections: 4, MaxIdleConnectionsPerHost: 2, IdleConnectionTimeout: time.Second,
	})
	t.Cleanup(client.CloseIdleConnections)
	breaker := httpclient.NewBreaker(httpclient.BreakerConfig{FailureThreshold: 5, ResetTimeout: time.Second})
	repo, err := loyaltyrepo.New(client, address, breaker)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestGlobalLimitAndWaitingCancellation(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"status":"BRONZE","discount":5,"reservationCount":0}`)); err != nil {
				t.Error(err)
			}
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)
	deadline, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	client := newLoyalty(t, server.URL+"/", time.Second)
	finished := make(chan error, 2)
	for range 2 {
		go func() { _, err := client.Loyalty(t.Context(), "user"); finished <- err }()
	}
	for range 2 {
		select {
		case <-entered:
		case <-deadline.Done():
			t.Fatal(deadline.Err())
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	waiting := make(chan error, 1)
	go func() { _, err := client.Loyalty(ctx, "user"); waiting <- err }()
	cancel()
	select {
	case err := <-waiting:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-deadline.Done():
		t.Fatal(deadline.Err())
	}
	select {
	case <-entered:
		t.Fatal("request exceeded global limit")
	default:
	}
	for range 2 {
		select {
		case release <- struct{}{}:
		case <-deadline.Done():
			t.Fatal(deadline.Err())
		}
	}
	for range 2 {
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
		case <-deadline.Done():
			t.Fatal(deadline.Err())
		}
	}
}

func TestTruncatedResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "256")
		if _, err := w.Write([]byte(`{"discount":5}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := newLoyalty(t, server.URL, time.Second)
	_, err := client.Loyalty(t.Context(), "user")
	if !errors.Is(err, entities.ErrUpstreamUnavailable) {
		t.Fatal(err)
	}
}

func TestGlobalLimitAcrossRequests(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	entered := make(chan struct{}, 24)
	release := make(chan struct{})
	var active, peak, calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		running := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); running > old; old = peak.Load() {
			if peak.CompareAndSwap(old, running) {
				break
			}
		}
		calls.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"BRONZE","discount":5,"reservationCount":0}`))
	}))
	defer server.Close()
	defer close(release)
	client := newLoyalty(t, server.URL, time.Second)
	finished := make(chan error, 24)
	for range 24 {
		go func() { finished <- client.ChangeLoyalty(ctx, "user", -1, "") }()
	}
	for range 2 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	for range 2 {
		select {
		case release <- struct{}{}:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	// The initial barrier proves both permits are used; release later requests immediately.
	for range 22 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		select {
		case release <- struct{}{}:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	for range 24 {
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if calls.Load() != 24 || peak.Load() != 2 || active.Load() != 0 {
		t.Fatalf("calls %d peak %d active %d", calls.Load(), peak.Load(), active.Load())
	}
}

func TestBreakerRecoveryAndOperationKeys(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var healthy atomic.Bool
	var keysMu sync.Mutex
	keys := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			keysMu.Lock()
			keys = append(keys, r.Header.Get("Idempotency-Key"))
			keysMu.Unlock()
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if !healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"status":"BRONZE","discount":5,"reservationCount":0}`))
	}))
	defer server.Close()
	client := httpclient.New(httpclient.Config{
		Timeout: time.Second, MaxInFlight: 4,
		MaxIdleConnections: 4, MaxIdleConnectionsPerHost: 4, IdleConnectionTimeout: time.Second,
	})
	defer client.CloseIdleConnections()
	breaker := httpclient.NewBreaker(httpclient.BreakerConfig{FailureThreshold: 5, ResetTimeout: time.Second})
	repo, err := loyaltyrepo.New(client, server.URL, breaker)
	if err != nil {
		t.Fatal(err)
	}
	for range 6 {
		_, _ = repo.Loyalty(t.Context(), "user")
	}
	if calls.Load() != 5 {
		t.Fatalf("reads reached upstream %d times", calls.Load())
	}
	healthy.Store(true)
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err = repo.Loyalty(t.Context(), "user")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("breaker failed to recover", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for range 2 {
		if err = repo.ChangeLoyalty(t.Context(), "user", -1, "booking:cancel"); err != nil {
			t.Fatal(err)
		}
	}
	keysMu.Lock()
	defer keysMu.Unlock()
	if len(keys) != 2 || keys[0] != "booking:cancel" || keys[1] != keys[0] {
		t.Fatal(keys)
	}
}
