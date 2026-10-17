package entities

import (
	"context"
	"errors"
	"testing"
	"uuid"
)

func TestActionAcceptUsesTypedVisitor(t *testing.T) {
	t.Parallel()
	payload := CancelPaymentOperation{PaymentUID: uuid.New()}
	cause := errors.New("payment unavailable")
	called := false
	action := OperationAction[CancelPaymentOperation]{
		Payload: payload,
		Visitor: ActionVisitor[CancelPaymentOperation](func(
			ctx context.Context, got CancelPaymentOperation,
		) (bool, error) {
			called = true
			if ctx != t.Context() || got != payload {
				t.Fatal("visitor received different context or payload")
			}
			return true, cause
		}),
	}
	stop, err := action.Accept(t.Context())
	if !called || !stop || !errors.Is(err, cause) {
		t.Fatalf("visitor result: called=%t stop=%t err=%v", called, stop, err)
	}
	action.Visitor = nil
	if _, err := action.Accept(t.Context()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing visitor error = %v", err)
	}
}
