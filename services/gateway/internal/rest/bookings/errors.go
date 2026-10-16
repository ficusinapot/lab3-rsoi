package bookings

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/rest/bookings/api"
)

func serviceError(ctx context.Context, logger *slog.Logger, err error) (int, api.ErrorResponse) {
	status := http.StatusInternalServerError
	message := "internal server error"
	switch {
	case errors.Is(err, entities.ErrOperationPanic):
		status, message = http.StatusInternalServerError, "internal server error"
	case errors.Is(err, entities.ErrInvalidInput):
		status, message = http.StatusBadRequest, "invalid input"
	case errors.Is(err, entities.ErrNotFound):
		status, message = http.StatusNotFound, http.StatusText(http.StatusNotFound)
	case errors.Is(err, entities.ErrLoyaltyUnavailable):
		status, message = http.StatusServiceUnavailable, "Loyalty Service unavailable"
	case errors.Is(err, entities.ErrReservationUnavailable):
		status, message = http.StatusInternalServerError, "Reservation Service unavailable"
	case errors.Is(err, entities.ErrQueueUnavailable), errors.Is(err, entities.ErrOperationBusy):
		status, message = http.StatusServiceUnavailable, "operation service unavailable"
	case errors.Is(err, entities.ErrQueueFull):
		status, message = http.StatusServiceUnavailable, "retry queue full"
	case errors.Is(err, entities.ErrUpstreamTimeout):
		status, message = http.StatusGatewayTimeout, http.StatusText(http.StatusGatewayTimeout)
	case errors.Is(err, entities.ErrUpstreamUnavailable):
		status, message = http.StatusBadGateway, http.StatusText(http.StatusBadGateway)
	}
	if status >= 500 {
		logger.ErrorContext(ctx, "request failed", "error", err)
	}
	return status, api.ErrorResponse{Message: new(message)}
}

func (h *handler) writeValidationError(w http.ResponseWriter, r *http.Request, message string, err error) {
	response := api.ValidationErrorResponse{Message: new(message)}
	if err != nil {
		errorMessage := err.Error()
		details := []api.ErrorDescription{{Error: new(errorMessage)}}
		response.Errors = new(details)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.DebugContext(r.Context(), "write validation response", "error", err)
	}
}

func (h *handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	status, response := serviceError(r.Context(), h.logger, err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.DebugContext(r.Context(), "write service error response", "error", err)
	}
}
