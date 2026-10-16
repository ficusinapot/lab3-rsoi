package rest

import (
	"context"
	"log/slog"
	"net/http"
	"time"
	"uuid"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/samber/oops"
)

type HTTPMetrics interface {
	ObserveHTTPRequest(string, string, int, time.Duration)
}

func Observability(logger *slog.Logger, metrics HTTPMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := requestID(r)
			w.Header().Set("X-Request-ID", id)
			ctx := oops.WithBuilder(r.Context(), oops.In("http").Trace(id))
			r = r.WithContext(ctx)
			response := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			started := time.Now()
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.ErrorContext(ctx, "recovering from panic", "error", oops.
						With("panic", recovered).
						Errorf("handler panicked"))
					if response.Status() == 0 {
						http.Error(response, "internal server error", http.StatusInternalServerError)
					}
				}
				route := chi.RouteContext(ctx).RoutePattern()
				if route == "" {
					route = "unmatched"
				}
				status := response.Status()
				if status == 0 {
					status = http.StatusOK
				}
				observe(
					ctx,
					logger,
					metrics,
					observation{Method: r.Method, Route: route, Status: status, Duration: time.Since(started), RequestID: id},
				)
			}()
			next.ServeHTTP(response, r)
		})
	}
}

type observation struct {
	Method    string
	Route     string
	Status    int
	Duration  time.Duration
	RequestID string
}

func observe(ctx context.Context, logger *slog.Logger, metrics HTTPMetrics, request observation) {
	if metrics != nil {
		method := request.Method
		switch method {
		case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
			http.MethodDelete, http.MethodHead, http.MethodOptions:
		default:
			method = "OTHER"
		}
		metrics.ObserveHTTPRequest(
			method,
			request.Route,
			request.Status,
			request.Duration,
		)
	}
	level := slog.LevelDebug
	message := "REQUEST"
	if request.Status >= http.StatusInternalServerError {
		level = slog.LevelError
		message = "REQUEST_ERROR"
	}
	logger.LogAttrs(
		ctx,
		level,
		message,
		slog.String("method", request.Method),
		slog.String("route", request.Route),
		slog.Int("status", request.Status),
		slog.Duration("duration", request.Duration),
		slog.String("x-request-id", request.RequestID),
	)
}

func requestID(r *http.Request) string {
	if id, err := uuid.Parse(r.Header.Get("X-Request-ID")); err == nil {
		return id.String()
	}
	return uuid.New().String()
}
