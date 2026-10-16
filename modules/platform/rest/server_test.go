package rest

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/samber/oops"
)

type shutdownListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (l *shutdownListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() { close(l.closed) })
	return oops.Wrapf(err, "close test listener")
}

func TestServeShutdown(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		forced bool
	}{
		{name: "graceful"},
		{name: "deadline", forced: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			observed := &shutdownListener{Listener: listener, closed: make(chan struct{})}
			ctx, cancel := context.WithCancel(t.Context())
			entered := make(chan struct{})
			release := make(chan struct{})
			handled := make(chan struct{})
			var releaseOnce sync.Once
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(handled)
				close(entered)
				select {
				case <-release:
					w.WriteHeader(http.StatusNoContent)
				case <-r.Context().Done():
				}
			})
			cfg := Config{
				ReadHeaderTimeout: time.Second, ReadTimeout: time.Second,
				WriteTimeout: time.Second, IdleTimeout: time.Second, ShutdownTimeout: 50 * time.Millisecond,
			}
			stopped := make(chan struct{})
			var shutdownErr error
			go func() {
				shutdownErr = serve(ctx, cfg, observed, handler)
				close(stopped)
			}()
			client := &http.Client{Timeout: time.Second}
			response := make(chan struct{})
			var requestErr error
			var status int
			go func() {
				res, err := client.Get("http://" + listener.Addr().String() + "/active")
				if err == nil {
					status = res.StatusCode
					_, readErr := io.Copy(io.Discard, res.Body)
					err = errors.Join(readErr, res.Body.Close())
				}
				requestErr = err
				close(response)
			}()
			t.Cleanup(func() {
				cancel()
				releaseOnce.Do(func() { close(release) })
				for _, done := range []<-chan struct{}{stopped, response} {
					select {
					case <-done:
					case <-time.After(2 * time.Second):
						t.Error("shutdown cleanup timed out")
					}
				}
				_ = listener.Close()
				client.CloseIdleConnections()
			})
			wait := func(done <-chan struct{}, message string) {
				t.Helper()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Fatal(message)
				}
			}
			wait(entered, "active request did not reach handler")
			cancel()
			wait(observed.closed, "shutdown did not close listener")
			if !tc.forced {
				releaseOnce.Do(func() { close(release) })
			}
			wait(stopped, "server did not finish shutdown")
			wait(response, "active request did not finish")
			wait(handled, "handler did not finish")
			if tc.forced {
				if !errors.Is(shutdownErr, context.DeadlineExceeded) || requestErr == nil {
					t.Fatalf("shutdown %v request %v", shutdownErr, requestErr)
				}
			} else if shutdownErr != nil || requestErr != nil || status != http.StatusNoContent {
				t.Fatalf("shutdown %v request %v status %d", shutdownErr, requestErr, status)
			}
		})
	}
}
