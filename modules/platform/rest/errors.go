package rest

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/samber/oops"
)

// ErrorResponse builds a public HTTP error and logs server-side failures.
func ErrorResponse(ctx context.Context, logger *slog.Logger, status int, message string, cause error) error {
	if status >= http.StatusInternalServerError {
		logger.ErrorContext(ctx, "request failed", "error", cause)
	}
	return oops.FromContext(ctx).Wrap(huma.NewError(status, message))
}
