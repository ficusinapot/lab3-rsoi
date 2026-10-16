package payments

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"github.com/samber/oops"

	"go.uber.org/mock/gomock"
	"lab2/payment/internal/models/entities"
	"lab2/platform/rest"
)

func TestPaymentHTTPContract(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	payment := entities.Payment{PaymentUID: id, Status: entities.StatusPaid, Price: 27000}
	for _, tc := range []struct {
		name, method, path, body string
		status                   int
	}{
		{
			name: "create", method: http.MethodPost, path: "/api/v1/payments",
			body: `{"price":27000}`, status: http.StatusCreated,
		},
		{name: "get", method: http.MethodGet, path: "/api/v1/payments/" + id.String(), status: http.StatusOK},
		{name: "cancel", method: http.MethodDelete, path: "/api/v1/payments/" + id.String(), status: http.StatusNoContent},
		{name: "invalid uuid", method: http.MethodGet, path: "/api/v1/payments/bad", status: http.StatusUnprocessableEntity},
		{
			name: "malformed json", method: http.MethodPost, path: "/api/v1/payments",
			body: `{"price":`, status: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			service := NewMockService(gomock.NewController(t))
			switch tc.name {
			case "create":
				service.EXPECT().Create(gomock.Any(), 27000).Return(payment, nil)
			case "get":
				service.EXPECT().Get(gomock.Any(), id).Return(payment, nil)
			case "cancel":
				service.EXPECT().Cancel(gomock.Any(), id).Return(nil)
			}
			cfg := rest.Config{APIPrefix: "/api/v1", MaxBodyBytes: 4096}
			router, api := rest.New(cfg, rest.OpenAPIConfig{Title: "Payment", Version: "1"})
			Register(api, service, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			if out.Code != tc.status {
				t.Fatalf("status = %d, want %d; body: %s", out.Code, tc.status, out.Body)
			}
			if tc.status == http.StatusNoContent {
				if out.Body.Len() != 0 {
					t.Errorf("204 response body = %q, want empty", out.Body.String())
				}
				return
			}
			if tc.status != http.StatusOK && tc.status != http.StatusCreated {
				return
			}
			var got struct {
				PaymentUID uuid.UUID `json:"paymentUid"`
				Status     string    `json:"status"`
				Price      int       `json:"price"`
			}
			decoder := json.NewDecoder(out.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&got); err != nil {
				t.Fatalf("decode payment response: %v", err)
			}
			if got.PaymentUID != id {
				t.Errorf("paymentUid = %s, want %s", got.PaymentUID, id)
			}
			if got.Status != "PAID" {
				t.Errorf("status = %q, want PAID", got.Status)
			}
			if got.Price != 27000 {
				t.Errorf("price = %d, want 27000", got.Price)
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
		{name: "not found", err: entities.ErrNotFound, status: http.StatusNotFound},
		{name: "invalid input", err: entities.ErrInvalidInput, status: http.StatusBadRequest},
		{name: "unexpected error", err: oops.Errorf("storage failed"), status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id := uuid.New()
			service := NewMockService(gomock.NewController(t))
			service.EXPECT().Get(gomock.Any(), id).Return(entities.Payment{}, tc.err)
			cfg := rest.Config{APIPrefix: "/api/v1"}
			router, api := rest.New(cfg, rest.OpenAPIConfig{Title: "Payment", Version: "1"})
			Register(api, service, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/"+id.String()+"", nil)
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
