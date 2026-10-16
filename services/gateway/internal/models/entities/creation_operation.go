package entities

import "uuid"

// CreationOperation contains the identities needed to compensate booking creation.
// It has no execution dependencies and is not a persisted queue envelope.
type CreationOperation struct {
	key            uuid.UUID
	User           string
	ReservationUID uuid.UUID
	PaymentUID     uuid.UUID
}

func NewCreationOperation(user string, reservationID, paymentID uuid.UUID) CreationOperation {
	return CreationOperation{
		key:  OperationKey(OperationCompensation, reservationID),
		User: user, ReservationUID: reservationID, PaymentUID: paymentID,
	}
}

func (o CreationOperation) Group() Operation {
	return Operation{Kind: OperationCompensation, Key: o.key}
}

func (o CreationOperation) Compensation() Operation {
	group := o.Group()
	group.Steps = []Action{
		NewOperationAction(CancelReservationOperation{User: o.User, ReservationUID: o.ReservationUID}),
		NewOperationAction(CancelPaymentOperation{PaymentUID: o.PaymentUID}),
		NewOperationAction(RollbackLoyaltyOperation{User: o.User, OperationID: o.ReservationUID.String() + ":rollback"}),
	}
	return group
}

// ResolveCompensation checks whether loyalty committed before undoing known writes.
func (o CreationOperation) ResolveCompensation() Operation {
	group := o.Compensation()
	group.Steps = append([]Action{
		NewOperationAction(ResolveCreationOperation{User: o.User, ReservationUID: o.ReservationUID}),
	}, group.Steps...)
	return group
}
