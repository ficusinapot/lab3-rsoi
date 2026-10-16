package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/samber/oops"
)

func TestErrorResponse(t *testing.T) {
	t.Parallel()
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusServiceUnavailable,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			cause := errors.New("private database failure")
			ctx := oops.WithBuilder(t.Context(), oops.With("request_id", "request-123"))
			err := ErrorResponse(ctx, logger, status, "public failure", cause)
			var statusError huma.StatusError
			if !errors.As(err, &statusError) || statusError.GetStatus() != status {
				t.Fatalf("status error = %v", err)
			}
			var response *huma.ErrorModel
			if !errors.As(err, &response) || response.Detail != "public failure" {
				t.Fatalf("public response = %v", err)
			}
			if strings.Contains(err.Error(), cause.Error()) || errors.Is(err, cause) {
				t.Fatalf("technical cause exposed: %v", err)
			}
			var contextual oops.OopsError
			if !errors.As(err, &contextual) || contextual.Context()["request_id"] != "request-123" {
				t.Fatalf("request context missing: %v", err)
			}
			if status < http.StatusInternalServerError {
				if logs.Len() != 0 {
					t.Fatalf("unexpected client error log: %s", &logs)
				}
				return
			}
			var record struct {
				Level   string `json:"level"`
				Message string `json:"msg"`
				Error   string `json:"error"`
			}
			decoder := json.NewDecoder(&logs)
			if err := decoder.Decode(&record); err != nil {
				t.Fatal(err)
			}
			if record.Level != "ERROR" || record.Message != "request failed" || record.Error != cause.Error() {
				t.Fatalf("server error log = %+v", record)
			}
			if decoder.More() {
				t.Fatal("server failure logged more than once")
			}
		})
	}
}
