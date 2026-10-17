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
	Accept(context.Context, ActionVisitors) (bool, error)
}

// Action is the common view of typed actions in a heterogeneous operation group.
type Action interface {
	Data() OperationPayload
	Validate() error
	Accept(context.Context, ActionVisitors) (bool, error)
}

// ActionVisitor handles one operation payload type.
type ActionVisitor[T OperationPayload] interface {
	Visit(context.Context, T) (bool, error)
}

type ActionVisitorFunc[T OperationPayload] func(context.Context, T) (bool, error)

func (f ActionVisitorFunc[T]) Visit(ctx context.Context, payload T) (bool, error) {
	stop, err := f(ctx, payload)
	return stop, oops.Wrap(err)
}

// ActionVisitors supplies an independent visitor for each operation.
type ActionVisitors struct {
	CancelPayment     ActionVisitor[CancelPaymentOperation]
	CancelReservation ActionVisitor[CancelReservationOperation]
	RollbackLoyalty   ActionVisitor[RollbackLoyaltyOperation]
	DecreaseLoyalty   ActionVisitor[DecreaseLoyaltyOperation]
	ResolveCreation   ActionVisitor[ResolveCreationOperation]
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

func (a OperationAction[T]) Accept(ctx context.Context, visitor ActionVisitors) (bool, error) {
	stop, err := a.Payload.Accept(ctx, visitor)
	return stop, oops.Wrap(err)
}

func (o CancelPaymentOperation) Accept(ctx context.Context, visitor ActionVisitors) (bool, error) {
	stop, err := visitor.CancelPayment.Visit(ctx, o)
	return stop, oops.Wrap(err)
}

func (o CancelReservationOperation) Accept(ctx context.Context, visitor ActionVisitors) (bool, error) {
	stop, err := visitor.CancelReservation.Visit(ctx, o)
	return stop, oops.Wrap(err)
}

func (o RollbackLoyaltyOperation) Accept(ctx context.Context, visitor ActionVisitors) (bool, error) {
	stop, err := visitor.RollbackLoyalty.Visit(ctx, o)
	return stop, oops.Wrap(err)
}

func (o DecreaseLoyaltyOperation) Accept(ctx context.Context, visitor ActionVisitors) (bool, error) {
	stop, err := visitor.DecreaseLoyalty.Visit(ctx, o)
	return stop, oops.Wrap(err)
}

func (o ResolveCreationOperation) Accept(ctx context.Context, visitor ActionVisitors) (bool, error) {
	stop, err := visitor.ResolveCreation.Visit(ctx, o)
	return stop, oops.Wrap(err)
}
