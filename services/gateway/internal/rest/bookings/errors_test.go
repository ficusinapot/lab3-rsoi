package bookings

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"lab2/gateway/internal/models/entities"
)

func TestOperationPanicReturnsInternalError(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, cause := range []error{entities.ErrInvalidInput, entities.ErrLoyaltyUnavailable} {
		status, response := serviceError(t.Context(), logger, entities.NewOperationPanic(cause, nil))
		if status != http.StatusInternalServerError || response.Message == nil ||
			*response.Message != "internal server error" {
			t.Fatalf("panic response: %d %+v", status, response)
		}
	}
}
