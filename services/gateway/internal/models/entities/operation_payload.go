package entities

import (
	"strings"
	"uuid"

	"github.com/samber/oops"
)

// OperationPayload contains validated action data. No serialized data enters a usecase.
type OperationPayload interface {
	Validate() error
}

// Action is the common view of typed actions in a heterogeneous operation group.
type Action interface {
	Data() OperationPayload
	Validate() error
}

type OperationAction[T OperationPayload] struct {
	Payload T
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
