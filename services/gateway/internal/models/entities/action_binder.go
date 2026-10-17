package entities

// ActionBinder attaches a typed visitor to each operation payload.
type ActionBinder interface {
	BindCancelPayment(CancelPaymentOperation) Action
	BindCancelReservation(CancelReservationOperation) Action
	BindRollbackLoyalty(RollbackLoyaltyOperation) Action
	BindDecreaseLoyalty(DecreaseLoyaltyOperation) Action
	BindResolveCreation(ResolveCreationOperation) Action
}

func (o CancelPaymentOperation) Bind(binder ActionBinder) Action {
	return binder.BindCancelPayment(o)
}

func (o CancelReservationOperation) Bind(binder ActionBinder) Action {
	return binder.BindCancelReservation(o)
}

func (o RollbackLoyaltyOperation) Bind(binder ActionBinder) Action {
	return binder.BindRollbackLoyalty(o)
}

func (o DecreaseLoyaltyOperation) Bind(binder ActionBinder) Action {
	return binder.BindDecreaseLoyalty(o)
}

func (o ResolveCreationOperation) Bind(binder ActionBinder) Action {
	return binder.BindResolveCreation(o)
}
