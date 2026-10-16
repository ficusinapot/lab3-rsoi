package operations

import (
	"context"
	"uuid"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
)

func (g *Groups) Cancel(ctx context.Context, user string, id uuid.UUID) error {
	// A visible reservation may still belong to an unfinished creation.
	// Read the current row only after creation/compensation has finished.
	creationKey := entities.OperationKey(entities.OperationCompensation, id)
	if _, pending, err := g.stream.PendingOperation(ctx, creationKey); err != nil {
		return oops.Wrap(err)
	} else if pending {
		return oops.Wrap(entities.ErrOperationBusy)
	}
	row, err := g.reservations.Reservation(ctx, id, user)
	if err != nil {
		return oops.Wrapf(err, "get reservation")
	}
	if row.Status == entities.ReservationCanceled {
		return nil
	}
	key := entities.OperationKey(entities.OperationCancellation, id)
	task, pending, err := g.pending(ctx, key)
	if err != nil {
		return oops.Wrap(err)
	}
	if pending {
		if entities.HasAction[entities.CancelPaymentOperation](task) ||
			entities.HasAction[entities.CancelReservationOperation](task) {
			return oops.Wrap(entities.ErrReservationUnavailable)
		}
		return nil
	}

	task = entities.Operation{
		Kind: entities.OperationCancellation, Key: key,
		Steps: []entities.Action{
			entities.NewOperationAction(entities.CancelPaymentOperation{PaymentUID: row.PaymentUID}),
			entities.NewOperationAction(entities.CancelReservationOperation{User: user, ReservationUID: id}),
			entities.NewOperationAction(entities.DecreaseLoyaltyOperation{User: user, OperationID: id.String() + ":cancel"}),
		},
	}

	result := g.execute(ctx, task)
	task = result.Task
	if entities.HasAction[entities.CancelPaymentOperation](task) {
		return result.Err
	}
	if !entities.HasAction[entities.CancelReservationOperation](task) &&
		entities.HasAction[entities.DecreaseLoyaltyOperation](task) &&
		entities.Unavailable(ctx, result.Err) {
		return nil
	}
	return result.Err
}
