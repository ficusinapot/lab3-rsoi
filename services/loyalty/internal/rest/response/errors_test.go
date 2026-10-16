package response

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/samber/oops"
	"lab2/loyalty/internal/models/entities"
	"lab2/platform/rest"
)

func TestError(t *testing.T) {
	t.Parallel()
	const internalMessage = "internal server error"
	const secret = "postgres://program:secret-password@localhost/reservation" // #nosec G101 -- fake credential.
	for _, tc := range []struct {
		name    string
		err     error
		status  int
		message string
		logs    int
	}{
		{
			name: "plain invalid", err: entities.ErrInvalidInput,
			status: http.StatusBadRequest, message: "invalid input",
		},
		{
			name: "wrapped invalid", err: oops.Wrapf(entities.ErrInvalidInput, "technical validation"),
			status: http.StatusBadRequest, message: "invalid input",
		},
		{
			name: "plain technical", err: errors.New(secret),
			status: http.StatusInternalServerError, message: internalMessage, logs: 1,
		},
		{
			name: "wrapped technical", err: oops.Public("private technical message").Wrap(errors.New(secret)),
			status: http.StatusInternalServerError, message: internalMessage, logs: 1,
		},
		{name: "invalid", err: oops.In("validation").Code("invalid_input").
			Public("valid increasing dates are required").
			Wrap(entities.ErrInvalidInput), status: http.StatusBadRequest, message: "valid increasing dates are required"},
		{name: "technical", err: oops.
			With("dsn", secret).
			Errorf("SQL failed: %s", secret), status: http.StatusInternalServerError, message: internalMessage, logs: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var logBuffer bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
			router, api := rest.New(rest.Config{RequestTimeout: time.Second, APIPrefix: "/testing"}, rest.OpenAPIConfig{
				Title: "test", Version: "test", DocsPath: "/docs", SchemaPath: "/openapi",
			})
			huma.Register(api, huma.Operation{OperationID: "failure", Method: http.MethodGet, Path: "/failure"},
				func(ctx context.Context, _ *struct{}) (*struct{}, error) {
					err := Error(ctx, logger, tc.err)
					var statusError huma.StatusError
					if !errors.As(err, &statusError) || statusError.GetStatus() != tc.status {
						t.Fatalf("status error=%v", err)
					}
					return nil, err
				})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/testing/failure", nil))
			var body struct {
				Detail string `json:"detail"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.status || body.Detail != tc.message || strings.Contains(response.Body.String(), secret) {
				t.Fatalf("response: status=%d body=%s", response.Code, response.Body.String())
			}
			lines := strings.Split(strings.TrimSpace(logBuffer.String()), "\n")
			if tc.logs == 0 {
				if logBuffer.Len() != 0 {
					t.Fatalf("unexpected logs: %s", &logBuffer)
				}
				return
			}
			if len(lines) != tc.logs {
				t.Fatalf("log count=%d: %s", len(lines), &logBuffer)
			}
			var entry struct {
				Level   string          `json:"level"`
				Message string          `json:"msg"`
				Error   json.RawMessage `json:"error"`
			}
			if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
				t.Fatal(err)
			}
			validLog := entry.Level == "ERROR" && entry.Message == "request failed"
			if !validLog || !strings.Contains(string(entry.Error), tc.err.Error()) {
				t.Fatalf("structured log=%+v", entry)
			}
		})
	}
}
