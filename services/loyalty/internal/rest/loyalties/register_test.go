package loyalties

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/samber/oops"

	"go.uber.org/mock/gomock"
	"lab2/loyalty/internal/models/entities"
	"lab2/platform/rest"
)

const (
	loyaltyAPIPrefix = "/api/v1"
	loyaltyAPITitle  = "Loyalty"
)

func TestLoyaltyHTTPContract(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, method, path string
		delta, count       int
	}{
		{name: "get", method: http.MethodGet, path: "/api/v1/loyalty", count: 25},
		{name: "increase", method: http.MethodPost, path: "/api/v1/loyalty/reservations", delta: 1, count: 10},
		{name: "decrease", method: http.MethodDelete, path: "/api/v1/loyalty/reservations", delta: -1, count: 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			service := NewMockService(gomock.NewController(t))
			want := entities.ForCount(tc.count)
			if tc.method == http.MethodGet {
				service.EXPECT().Get(gomock.Any(), "Test Max").Return(want, nil)
			} else {
				service.EXPECT().Change(gomock.Any(), "Test Max", tc.delta, "").Return(want, nil)
			}
			cfg := rest.Config{APIPrefix: loyaltyAPIPrefix}
			router, api := rest.New(cfg, rest.OpenAPIConfig{Title: loyaltyAPITitle, Version: "1"})
			Register(api, service, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("X-User-Name", "Test Max")
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			if out.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", out.Code, out.Body)
			}
			var got struct {
				Status           entities.Status `json:"status"`
				Discount         int             `json:"discount"`
				ReservationCount int             `json:"reservationCount"`
			}
			decoder := json.NewDecoder(out.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&got); err != nil {
				t.Fatalf("decode loyalty response: %v", err)
			}
			if got.Status != want.Status {
				t.Errorf("status = %q, want %q", got.Status, want.Status)
			}
			if got.Discount != want.Discount {
				t.Errorf("discount = %d, want %d", got.Discount, want.Discount)
			}
			if got.ReservationCount != want.ReservationCount {
				t.Errorf("reservationCount = %d, want %d", got.ReservationCount, want.ReservationCount)
			}
		})
	}
}

func TestServiceErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{name: "invalid input", err: entities.ErrInvalidInput, status: http.StatusBadRequest},
		{name: "unexpected error", err: oops.Errorf("storage failed"), status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			service := NewMockService(gomock.NewController(t))
			service.EXPECT().Get(gomock.Any(), "Test Max").Return(entities.Loyalty{}, tc.err)
			cfg := rest.Config{APIPrefix: loyaltyAPIPrefix}
			router, api := rest.New(cfg, rest.OpenAPIConfig{Title: loyaltyAPITitle, Version: "1"})
			Register(api, service, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/loyalty", nil)
			req.Header.Set("X-User-Name", "Test Max")
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			if out.Code != tc.status {
				t.Fatalf("status = %d, want %d; body: %s", out.Code, tc.status, out.Body)
			}
			var got struct {
				Detail string `json:"detail"`
			}
			if err := json.NewDecoder(out.Body).Decode(&got); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if got.Detail == "" {
				t.Error("error response has no detail")
			}
			if strings.Contains(got.Detail, "storage failed") {
				t.Errorf("internal error exposed: %q", got.Detail)
			}
		})
	}
}

func TestOperationHeader(t *testing.T) {
	t.Parallel()
	service := NewMockService(gomock.NewController(t))
	service.EXPECT().Change(gomock.Any(), "Test Max", -1, "booking:cancel").Return(entities.ForCount(0), nil)
	cfg := rest.Config{APIPrefix: loyaltyAPIPrefix}
	router, api := rest.New(cfg, rest.OpenAPIConfig{Title: loyaltyAPITitle, Version: "1"})
	Register(api, service, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/loyalty/reservations", nil)
	req.Header.Set("X-User-Name", "Test Max")
	req.Header.Set("Idempotency-Key", "booking:cancel")
	out := httptest.NewRecorder()
	router.ServeHTTP(out, req)
	if out.Code != http.StatusOK {
		t.Fatal(out.Code, out.Body)
	}
}
