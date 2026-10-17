package entities

import (
	"context"
	"strings"
	"uuid"

	"github.com/samber/oops"
)

// OperationPayload contains validated action data. No serialized data enters a usecase.
type OperationPayload interface {
	Validate() error
	Bind(ActionBinder) Action
}

// Action is the common view of typed actions in a heterogeneous operation group.
type Action interface {
	OperationPayload
	Data() OperationPayload
	Accept(context.Context) (bool, error)
}

// ActionVisitor handles one operation payload type.
type ActionVisitor[T OperationPayload] func(context.Context, T) (bool, error)

type OperationAction[T OperationPayload] struct {
	Payload T
	Visitor ActionVisitor[T]
}

func (a OperationAction[T]) Data() OperationPayload { return a.Payload }

type (
	CancelPaymentOperation     struct{ PaymentUID uuid.UUID }
	CancelReservationOperation struct {
		User           string
		ReservationUID uuid.UUID
	}
	ResolveCreationOperation struct {
		User           string
		ReservationUID uuid.UUID
	}
	RollbackLoyaltyOperation struct {
		User        string
		OperationID string
	}
)

type DecreaseLoyaltyOperation struct {
	User        string
	OperationID string
}

func NewOperationAction[T OperationPayload](data T) OperationAction[T] {
	return OperationAction[T]{Payload: data}
}

func OperationData[T OperationPayload](action Action) (T, error) {
	typed, ok := action.(OperationAction[T])
	if !ok {
		var zero T
		return zero, oops.Wrapf(ErrInvalidInput, "invalid operation payload type")
	}
	return typed.Payload, nil
}

func (a OperationAction[T]) Validate() error {
	return oops.Wrap(a.Payload.Validate())
}

func (o CancelPaymentOperation) Validate() error {
	if o.PaymentUID == uuid.Nil() {
		return oops.Wrapf(ErrInvalidInput, "invalid cancel payment payload")
	}
	return nil
}

func (o CancelReservationOperation) Validate() error {
	if strings.TrimSpace(o.User) == "" || o.ReservationUID == uuid.Nil() {
		return oops.Wrapf(ErrInvalidInput, "invalid cancel reservation payload")
	}
	return nil
}

func (o ResolveCreationOperation) Validate() error {
	if strings.TrimSpace(o.User) == "" || o.ReservationUID == uuid.Nil() {
		return oops.Wrapf(ErrInvalidInput, "invalid resolve creation payload")
	}
	return nil
}

func (o RollbackLoyaltyOperation) Validate() error {
	if strings.TrimSpace(o.User) == "" || strings.TrimSpace(o.OperationID) == "" {
		return oops.Wrapf(ErrInvalidInput, "invalid rollback loyalty payload")
	}
	return nil
}

func (o DecreaseLoyaltyOperation) Validate() error {
	if strings.TrimSpace(o.User) == "" || strings.TrimSpace(o.OperationID) == "" {
		return oops.Wrapf(ErrInvalidInput, "invalid decrease loyalty payload")
	}
	return nil
}

func (a OperationAction[T]) Accept(ctx context.Context) (bool, error) {
	if a.Visitor == nil {
		return false, oops.Wrapf(ErrInvalidInput, "missing operation visitor")
	}
	stop, err := a.Visitor(ctx, a.Payload)
	return stop, oops.Wrap(err)
}

func (a OperationAction[T]) Bind(binder ActionBinder) Action {
	return a.Payload.Bind(binder)
}
