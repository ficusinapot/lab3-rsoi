package operations

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/samber/oops"

	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/loyaltyifc"
	"lab2/gateway/internal/models/operationifc"
	"lab2/gateway/internal/models/opstreamifc"
	"lab2/gateway/internal/models/paymentifc"
	"lab2/gateway/internal/models/reservationifc"
)

type Groups struct {
	reservations    reservationifc.BookingSource
	payments        paymentifc.Creator
	loyalty         loyaltyifc.BookingSource
	stream          opstreamifc.Stream
	rollbackTimeout time.Duration
}

var _ operationifc.Groups = (*Groups)(nil)

func NewGroups(
	stream opstreamifc.Stream,
	rollbackTimeout time.Duration,
	reservations reservationifc.BookingSource,
	payments paymentifc.Creator,
	loyalty loyaltyifc.BookingSource,
) *Groups {
	return &Groups{
		stream: stream, rollbackTimeout: rollbackTimeout,
		reservations: reservations, payments: payments, loyalty: loyalty,
	}
}

func (g *Groups) reserve(ctx context.Context, task entities.Operation) error {
	reserved, err := g.stream.ReserveOperation(ctx, task.Key, task.Kind)
	if err != nil {
		return oops.Wrap(err)
	}
	if !reserved {
		return oops.Wrap(entities.ErrOperationBusy)
	}
	return nil
}

// prepare arms compensation only after the first plan is stored. Later failures
// retain the new plan: side effects may already require its updated identities.
func (g *Groups) prepare(ctx context.Context, current *entities.Operation, next entities.Operation) error {
	if current.Key != next.Key || current.Kind != next.Kind {
		return oops.Wrapf(entities.ErrInvalidInput, "operation group identity changed")
	}
	err := g.stream.Prepare(ctx, next)
	if err == nil || !current.Done() {
		*current = next
	}
	return oops.Wrap(err)
}

func (g *Groups) finish(ctx context.Context, task *entities.Operation, err *error) {
	if *err == nil || task.Done() {
		*err = oops.Wrap(errors.Join(*err, g.stream.ReleaseOperation(context.WithoutCancel(ctx), task.Key)))
		return
	}
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), g.rollbackTimeout)
	defer cancel()
	result := g.stream.Execute(rollbackCtx, *task)
	*err = oops.Wrap(errors.Join(*err, result.Err))
}

func (g *Groups) execute(ctx context.Context, task entities.Operation) entities.OperationResult {
	if err := g.reserve(ctx, task); err != nil {
		return entities.OperationResult{Task: task, Status: entities.OperationRejected, Err: err}
	}
	return g.stream.Execute(ctx, task)
}

func (g *Groups) pending(ctx context.Context, key uuid.UUID) (entities.Operation, bool, error) {
	task, pending, err := g.stream.PendingOperation(ctx, key)
	return task, pending, oops.Wrap(err)
}
