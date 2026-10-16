package operations

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"uuid"

	"lab2/gateway/internal/models/entities"
)

func TestCancellationPanicPreservesConfirmedSteps(t *testing.T) {
	t.Parallel()
	task := cancellationTask()
	cause := errors.New("reservation adapter panicked")
	calls := []string{}
	panics := true
	processor := newTestProcessor(
		reservationStub{cancel: func(context.Context, uuid.UUID, string) error {
			calls = append(calls, reservationStep)
			if panics {
				panic(cause)
			}
			return nil
		}},
		paymentStub{cancel: func(context.Context, uuid.UUID) error {
			calls = append(calls, paymentStep)
			return nil
		}},
		loyaltyStub{change: func(_ context.Context, _ string, _ int, key string) error {
			calls = append(calls, loyaltyStep)
			if key != operationPayload[entities.DecreaseLoyaltyOperation](t, task).OperationID {
				t.Error("idempotency key changed", key)
			}
			return nil
		}},
	)
	result := processor.Execute(t.Context(), task, entities.ExecutionBackground)
	if !errors.Is(result.Err, entities.ErrOperationPanic) || !errors.Is(result.Err, cause) {
		t.Fatal("panic cause lost", result)
	}
	if entities.HasAction[entities.CancelPaymentOperation](result.Task) ||
		!entities.HasAction[entities.CancelReservationOperation](result.Task) ||
		!entities.HasAction[entities.DecreaseLoyaltyOperation](result.Task) {
		t.Fatal("confirmed progress lost", result.Task)
	}
	if !reflect.DeepEqual(calls, []string{paymentStep, reservationStep}) {
		t.Fatal("cancellation continued after panic", calls)
	}
	panics, calls = false, []string{}
	result = processor.Execute(t.Context(), result.Task, entities.ExecutionBackground)
	if result.Err != nil || !result.Task.Done() || !reflect.DeepEqual(calls, []string{reservationStep, loyaltyStep}) {
		t.Fatal("retry replayed confirmed Payment", result, calls)
	}
}

func TestCompensationPanicDoesNotSkipIndependentPayment(t *testing.T) {
	t.Parallel()
	task := cancellationTask()
	task.Kind = entities.OperationCompensation
	task.Steps = []entities.Action{task.Steps[1], task.Steps[0]}
	paid := false
	processor := newTestProcessor(
		reservationStub{cancel: func(context.Context, uuid.UUID, string) error { panic("reservation panic") }},
		paymentStub{cancel: func(context.Context, uuid.UUID) error { paid = true; return nil }},
		nil,
	)
	result := processor.Execute(t.Context(), task, entities.ExecutionBackground)
	if !paid || !entities.HasAction[entities.CancelReservationOperation](result.Task) ||
		entities.HasAction[entities.CancelPaymentOperation](result.Task) ||
		!errors.Is(result.Err, entities.ErrOperationPanic) {
		t.Fatal("panic prevented independent compensation", paid, result)
	}
}
