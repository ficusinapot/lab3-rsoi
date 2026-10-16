package entities

import (
	"context"
	"errors"
)

func Unavailable(ctx context.Context, err error) bool {
	return ctx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrOperationPanic) &&
		(errors.Is(err, ErrUpstreamUnavailable) ||
			errors.Is(err, ErrUpstreamTimeout))
}
