package httpclient

import (
	"context"
	"errors"
	"time"

	"github.com/samber/oops"
	"github.com/sony/gobreaker/v2"
	"lab2/gateway/internal/models/entities"
)

type BreakerConfig struct {
	FailureThreshold int           `mapstructure:"failure_threshold"`
	ResetTimeout     time.Duration `mapstructure:"reset_timeout"`
}

func NewBreaker(cfg BreakerConfig) *gobreaker.TwoStepCircuitBreaker[struct{}] {
	return gobreaker.NewTwoStepCircuitBreaker[struct{}](gobreaker.Settings{
		MaxRequests: 1,
		Timeout:     cfg.ResetTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return int64(counts.ConsecutiveFailures) >= int64(cfg.FailureThreshold)
		},
		IsExcluded: func(err error) bool {
			return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
		},
	})
}

func Read[T any](
	ctx context.Context, b *gobreaker.TwoStepCircuitBreaker[struct{}], service error, call func() (T, error),
) (result T, err error) {
	if err = ctx.Err(); err != nil {
		return result, oops.Wrap(err)
	}
	done, err := b.Allow()
	if err != nil {
		return result, ServiceError(ctx, service, entities.ErrUpstreamUnavailable)
	}
	defer func() {
		switch {
		case ctx.Err() != nil:
			done(ctx.Err())
		case entities.Unavailable(ctx, err):
			done(entities.ErrUpstreamUnavailable)
		default:
			done(nil)
		}
	}()
	result, err = call()
	return result, ServiceError(ctx, service, err)
}

func ServiceError(ctx context.Context, service, err error) error {
	if entities.Unavailable(ctx, err) {
		return errors.Join(service, err)
	}
	return err
}
