package operations

import (
	"context"
	"runtime/debug"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/loyaltyifc"
	"lab2/gateway/internal/models/operationifc"
	"lab2/gateway/internal/models/paymentifc"
	"lab2/gateway/internal/models/reservationifc"
)

type Actions struct {
	reservations reservationifc.Canceler
	payments     paymentifc.Canceler
	loyalty      loyaltyifc.Updater
}

var _ operationifc.ActionExecutor = (*Actions)(nil)

func NewActions(
	reservations reservationifc.Canceler, payments paymentifc.Canceler, loyalty loyaltyifc.Updater,
) *Actions {
	return &Actions{reservations: reservations, payments: payments, loyalty: loyalty}
}

// Execute recovers at each action boundary so confirmed steps stay confirmed.
func (p *Actions) Execute(ctx context.Context, action entities.Action) (stop bool, err error) {
	defer func() {
		if value := recover(); value != nil {
			err = entities.NewOperationPanic(value, debug.Stack())
		}
	}()
	if action == nil {
		return false, oops.Wrapf(entities.ErrInvalidInput, "missing operation action")
	}
	if err := action.Validate(); err != nil {
		return false, oops.Wrap(err)
	}
	switch typed := action.(type) {
	case entities.OperationAction[entities.CancelPaymentOperation]:
		return false, oops.Wrap(p.payments.CancelPayment(ctx, typed.Payload.PaymentUID))
	case entities.OperationAction[entities.CancelReservationOperation]:
		return false, oops.Wrap(p.reservations.CancelReservation(ctx, typed.Payload.ReservationUID, typed.Payload.User))
	case entities.OperationAction[entities.RollbackLoyaltyOperation]:
		return false, oops.Wrap(p.loyalty.ChangeLoyalty(ctx, typed.Payload.User, -1, typed.Payload.OperationID))
	case entities.OperationAction[entities.DecreaseLoyaltyOperation]:
		return false, oops.Wrap(p.loyalty.ChangeLoyalty(ctx, typed.Payload.User, -1, typed.Payload.OperationID))
	case entities.OperationAction[entities.ResolveCreationOperation]:
		committed, err := p.loyalty.ResolveCreation(ctx, typed.Payload.User, typed.Payload.ReservationUID)
		return committed, oops.Wrap(err)
	}
	return false, oops.Wrapf(entities.ErrInvalidInput, "unknown operation handler")
}
