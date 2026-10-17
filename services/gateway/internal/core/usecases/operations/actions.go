package operations

import (
	"context"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/loyaltyifc"
	"lab2/gateway/internal/models/paymentifc"
	"lab2/gateway/internal/models/reservationifc"
)

type ActionFactory struct {
	reservations reservationifc.Canceler
	payments     paymentifc.Canceler
	loyalty      loyaltyifc.Updater
}

var _ entities.ActionBinder = (*ActionFactory)(nil)

func NewActionFactory(
	reservations reservationifc.Canceler, payments paymentifc.Canceler, loyalty loyaltyifc.Updater,
) *ActionFactory {
	return &ActionFactory{reservations: reservations, payments: payments, loyalty: loyalty}
}

func (p *ActionFactory) BindCancelPayment(payload entities.CancelPaymentOperation) entities.Action {
	return bindAction(payload, p.visitCancelPayment)
}

func (p *ActionFactory) BindCancelReservation(payload entities.CancelReservationOperation) entities.Action {
	return bindAction(payload, p.visitCancelReservation)
}

func (p *ActionFactory) BindRollbackLoyalty(payload entities.RollbackLoyaltyOperation) entities.Action {
	return bindAction(payload, p.visitRollbackLoyalty)
}

func (p *ActionFactory) BindDecreaseLoyalty(payload entities.DecreaseLoyaltyOperation) entities.Action {
	return bindAction(payload, p.visitDecreaseLoyalty)
}

func (p *ActionFactory) BindResolveCreation(payload entities.ResolveCreationOperation) entities.Action {
	return bindAction(payload, p.visitResolveCreation)
}

func bindAction[T entities.OperationPayload](
	payload T, visit entities.ActionVisitor[T],
) entities.OperationAction[T] {
	return entities.OperationAction[T]{Payload: payload, Visitor: visit}
}

func (p *ActionFactory) visitCancelPayment(
	ctx context.Context, operation entities.CancelPaymentOperation,
) (bool, error) {
	return false, oops.Wrap(p.payments.CancelPayment(ctx, operation.PaymentUID))
}

func (p *ActionFactory) visitCancelReservation(
	ctx context.Context, operation entities.CancelReservationOperation,
) (bool, error) {
	return false, oops.Wrap(p.reservations.CancelReservation(ctx, operation.ReservationUID, operation.User))
}

func (p *ActionFactory) visitRollbackLoyalty(
	ctx context.Context, operation entities.RollbackLoyaltyOperation,
) (bool, error) {
	return false, oops.Wrap(p.loyalty.ChangeLoyalty(ctx, operation.User, -1, operation.OperationID))
}

func (p *ActionFactory) visitDecreaseLoyalty(
	ctx context.Context, operation entities.DecreaseLoyaltyOperation,
) (bool, error) {
	return false, oops.Wrap(p.loyalty.ChangeLoyalty(ctx, operation.User, -1, operation.OperationID))
}

func (p *ActionFactory) visitResolveCreation(
	ctx context.Context, operation entities.ResolveCreationOperation,
) (bool, error) {
	committed, err := p.loyalty.ResolveCreation(ctx, operation.User, operation.ReservationUID)
	return committed, oops.Wrap(err)
}
