package response

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/samber/oops"
	"lab2/platform/rest"
	"lab2/reservation/internal/models/entities"
)

func Error(ctx context.Context, logger *slog.Logger, err error) error {
	status := http.StatusInternalServerError
	message := "internal server error"
	switch {
	case errors.Is(err, entities.ErrNotFound):
		status, message = http.StatusNotFound, "not found"
	case errors.Is(err, entities.ErrInvalidInput):
		status, message = http.StatusBadRequest, oops.GetPublic(err, "invalid input")
		if errors.Is(err, entities.ErrInvalidUsername) {
			message = "valid X-User-Name is required"
		}
	}
	return oops.Wrap(rest.ErrorResponse(ctx, logger, status, message, err))
}
