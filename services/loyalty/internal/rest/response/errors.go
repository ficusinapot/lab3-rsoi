package response

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/samber/oops"
	"lab2/loyalty/internal/models/entities"
	"lab2/platform/rest"
)

func Error(ctx context.Context, logger *slog.Logger, err error) error {
	status := http.StatusInternalServerError
	message := "internal server error"
	if errors.Is(err, entities.ErrInvalidInput) {
		status, message = http.StatusBadRequest, oops.GetPublic(err, "invalid input")
	}
	return oops.Wrap(rest.ErrorResponse(ctx, logger, status, message, err))
}
