package resttransport

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"lab2/platform/rest"
	bookinghttp "lab2/reservation/internal/rest/bookings"
	hotelhttp "lab2/reservation/internal/rest/hotels"
)

const (
	testTitle   = "Reservation"
	testVersion = "test"
	docsPath    = "/docs"
	schemaPath  = "/openapi"
	apiPrefix   = "/testing"
)

func TestNewDocs(t *testing.T) {
	t.Parallel()
	router, api := rest.New(rest.Config{RequestTimeout: time.Second, APIPrefix: apiPrefix}, rest.OpenAPIConfig{
		Title: testTitle, Version: testVersion, DocsPath: docsPath, SchemaPath: schemaPath,
	})
	if api.OpenAPI().Info.Title != testTitle {
		t.Fatal("OpenAPI title was not configured")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, apiPrefix+docsPath, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "swagger-ui") {
		t.Fatalf("Swagger docs: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMalformedHTTPValues(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{name: "non-numeric page", method: http.MethodGet, path: "/api/v1/hotels?page=abc"},
		{name: "invalid hotel UUID", method: http.MethodGet, path: "/api/v1/hotels/bad"},
		{name: "invalid reservation UUID", method: http.MethodGet, path: "/api/v1/reservations/bad"},
		{
			name: "invalid start date", method: http.MethodPost, path: "/api/v1/reservations",
			body: `{"hotelUid":"049161bb-badd-4fa8-9d90-87c9a82b0668",` +
				`"paymentUid":"049161bb-badd-4fa8-9d90-87c9a82b0668",` +
				`"startDate":"bad","endDate":"2026-01-04"}`,
		},
		{
			name: "malformed JSON", method: http.MethodPost, path: "/api/v1/reservations",
			body: `{`, wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := rest.Config{APIPrefix: "/api/v1", MaxBodyBytes: 4096}
			router, api := rest.New(cfg, rest.OpenAPIConfig{Title: testTitle, Version: testVersion})
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			ctrl := gomock.NewController(t)
			hotelhttp.Register(api, NewMockHotels(ctrl), logger)
			bookinghttp.Register(api, NewMockBookings(ctrl), logger, cfg)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("X-User-Name", "user")
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			wantStatus := tc.wantStatus
			if wantStatus == 0 {
				wantStatus = http.StatusUnprocessableEntity
			}
			if response.Code != wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, wantStatus, response.Body)
			}
		})
	}
}
