package operations

import (
	"context"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/loyaltyifc"
	"lab2/gateway/internal/models/paymentifc"
	"lab2/gateway/internal/models/reservationifc"
)

type actions struct {
	reservations reservationifc.Canceler
	payments     paymentifc.Canceler
	loyalty      loyaltyifc.Updater
}

func NewActions(
	reservations reservationifc.Canceler, payments paymentifc.Canceler, loyalty loyaltyifc.Updater,
) entities.ActionVisitors {
	handlers := &actions{reservations: reservations, payments: payments, loyalty: loyalty}
	return entities.ActionVisitors{
		CancelPayment:     entities.ActionVisitorFunc[entities.CancelPaymentOperation](handlers.VisitCancelPayment),
		CancelReservation: entities.ActionVisitorFunc[entities.CancelReservationOperation](handlers.VisitCancelReservation),
		RollbackLoyalty:   entities.ActionVisitorFunc[entities.RollbackLoyaltyOperation](handlers.VisitRollbackLoyalty),
		DecreaseLoyalty:   entities.ActionVisitorFunc[entities.DecreaseLoyaltyOperation](handlers.VisitDecreaseLoyalty),
		ResolveCreation:   entities.ActionVisitorFunc[entities.ResolveCreationOperation](handlers.VisitResolveCreation),
	}
}

func (p *actions) VisitCancelPayment(ctx context.Context, operation entities.CancelPaymentOperation) (bool, error) {
	return false, oops.Wrap(p.payments.CancelPayment(ctx, operation.PaymentUID))
}

func (p *actions) VisitCancelReservation(
	ctx context.Context, operation entities.CancelReservationOperation,
) (bool, error) {
	return false, oops.Wrap(p.reservations.CancelReservation(ctx, operation.ReservationUID, operation.User))
}

func (p *actions) VisitRollbackLoyalty(ctx context.Context, operation entities.RollbackLoyaltyOperation) (bool, error) {
	return false, oops.Wrap(p.loyalty.ChangeLoyalty(ctx, operation.User, -1, operation.OperationID))
}

func (p *actions) VisitDecreaseLoyalty(ctx context.Context, operation entities.DecreaseLoyaltyOperation) (bool, error) {
	return false, oops.Wrap(p.loyalty.ChangeLoyalty(ctx, operation.User, -1, operation.OperationID))
}

func (p *actions) VisitResolveCreation(ctx context.Context, operation entities.ResolveCreationOperation) (bool, error) {
	committed, err := p.loyalty.ResolveCreation(ctx, operation.User, operation.ReservationUID)
	return committed, oops.Wrap(err)
}
