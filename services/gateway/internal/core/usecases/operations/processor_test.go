package operations

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"uuid"

	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/loyaltyifc"
	"lab2/gateway/internal/models/paymentifc"
	"lab2/gateway/internal/models/reservationifc"
)

const (
	operationUser   = "user"
	paymentStep     = "payment"
	reservationStep = "reservation"
	loyaltyStep     = "loyalty"
)

type paymentStub struct {
	paymentifc.Payments
	cancel func(context.Context, uuid.UUID) error
}

func (p paymentStub) CancelPayment(ctx context.Context, id uuid.UUID) error { return p.cancel(ctx, id) }

type reservationStub struct {
	reservationifc.Bookings
	cancel func(context.Context, uuid.UUID, string) error
}

func (r reservationStub) CancelReservation(ctx context.Context, id uuid.UUID, user string) error {
	return r.cancel(ctx, id, user)
}

type loyaltyStub struct {
	loyaltyifc.Client
	change func(context.Context, string, int, string) error
}

func (l loyaltyStub) ChangeLoyalty(ctx context.Context, user string, delta int, key string) error {
	return l.change(ctx, user, delta, key)
}

func cancellationTask() entities.Operation {
	return entities.Operation{
		Kind: entities.OperationCancellation,
		Key:  testKey("cancel"),
		Steps: []entities.Action{
			entities.NewOperationAction(entities.CancelPaymentOperation{PaymentUID: uuid.New()}),
			entities.NewOperationAction(entities.CancelReservationOperation{User: operationUser, ReservationUID: uuid.New()}),
			entities.NewOperationAction(entities.DecreaseLoyaltyOperation{User: operationUser, OperationID: "stable"}),
		},
	}
}

func testProcessor(t *testing.T, task entities.Operation, calls *[]string, failures map[string]error) *Processor {
	t.Helper()
	return newTestProcessor(
		reservationStub{cancel: func(_ context.Context, id uuid.UUID, user string) error {
			want := operationPayload[entities.CancelReservationOperation](t, task)
			if id != want.ReservationUID || user != want.User {
				t.Fatalf("reservation arguments = %v/%s", id, user)
			}
			*calls = append(*calls, reservationStep)
			return failures[reservationStep]
		}},
		paymentStub{cancel: func(_ context.Context, id uuid.UUID) error {
			if id != operationPayload[entities.CancelPaymentOperation](t, task).PaymentUID {
				t.Fatalf("payment argument = %v", id)
			}
			*calls = append(*calls, paymentStep)
			return failures[paymentStep]
		}},
		loyaltyStub{change: func(_ context.Context, user string, delta int, key string) error {
			want := operationPayload[entities.DecreaseLoyaltyOperation](t, task)
			if user != want.User || delta != -1 || key != want.OperationID {
				t.Fatalf("loyalty arguments = %s/%d/%s", user, delta, key)
			}
			*calls = append(*calls, loyaltyStep)
			return failures[loyaltyStep]
		}},
	)
}

func TestCancellationStopsAndResumesAtFailedStep(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		step      string
		calls     []string
		remaining []string
	}{
		{paymentStep, []string{paymentStep}, []string{paymentStep, reservationStep, loyaltyStep}},
		{reservationStep, []string{paymentStep, reservationStep}, []string{reservationStep, loyaltyStep}},
		{loyaltyStep, []string{paymentStep, reservationStep, loyaltyStep}, []string{loyaltyStep}},
	} {
		t.Run(tc.step, func(t *testing.T) {
			t.Parallel()
			cause := errors.New("dependency failed")
			failures := map[string]error{tc.step: cause}
			task := cancellationTask()
			original := task
			original.Steps = append([]entities.Action(nil), task.Steps...)
			var calls []string
			processor := testProcessor(t, task, &calls, failures)
			result := processor.Execute(t.Context(), task, entities.ExecutionBackground)
			if result.Status != entities.OperationDeferred || !errors.Is(result.Err, cause) {
				t.Fatalf("failed result = %+v", result)
			}
			if !reflect.DeepEqual(task, original) || !reflect.DeepEqual(calls, tc.calls) {
				t.Fatalf("input = %+v, calls = %v", task, calls)
			}
			want := operationPayload[entities.DecreaseLoyaltyOperation](t, task)
			got := operationPayload[entities.DecreaseLoyaltyOperation](t, result.Task)
			if result.Task.Key != task.Key || got != want {
				t.Fatalf("retry identifiers changed: %+v", result.Task)
			}
			delete(failures, tc.step)
			calls = nil
			result = processor.Execute(t.Context(), result.Task, entities.ExecutionBackground)
			if result.Status != entities.OperationCompleted || result.Err != nil || !result.Task.Done() {
				t.Fatalf("resumed result = %+v", result)
			}
			if !reflect.DeepEqual(calls, tc.remaining) {
				t.Fatalf("resumed calls = %v, want %v", calls, tc.remaining)
			}
		})
	}
}

func TestCompensationAttemptsIndependentSteps(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name             string
		reservationFails bool
		paymentFails     bool
	}{
		{"success", false, false},
		{"reservation fails", true, false},
		{"payment fails", false, true},
		{"both fail", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			task := cancellationTask()
			task.Kind = entities.OperationCompensation
			task.Steps = []entities.Action{task.Steps[1], task.Steps[0]}
			reservationCause, paymentCause := errors.New("reservation failed"), errors.New("payment failed")
			failures := map[string]error{}
			if tc.reservationFails {
				failures[reservationStep] = reservationCause
			}
			if tc.paymentFails {
				failures[paymentStep] = paymentCause
			}
			var calls []string
			processor := testProcessor(t, task, &calls, failures)
			result := processor.Execute(t.Context(), task, entities.ExecutionBackground)
			if !reflect.DeepEqual(calls, []string{reservationStep, paymentStep}) {
				t.Fatalf("calls = %v", calls)
			}
			if entities.HasAction[entities.CancelReservationOperation](result.Task) != tc.reservationFails ||
				entities.HasAction[entities.CancelPaymentOperation](result.Task) != tc.paymentFails {
				t.Fatalf("outstanding steps = %+v", result.Task)
			}
			if errors.Is(result.Err, reservationCause) != tc.reservationFails ||
				errors.Is(result.Err, paymentCause) != tc.paymentFails {
				t.Fatalf("original causes lost: %v", result.Err)
			}
			clear(failures)
			calls = nil
			result = processor.Execute(t.Context(), result.Task, entities.ExecutionBackground)
			var remaining []string
			if tc.reservationFails {
				remaining = append(remaining, reservationStep)
			}
			if tc.paymentFails {
				remaining = append(remaining, paymentStep)
			}
			if !reflect.DeepEqual(calls, remaining) || result.Status != entities.OperationCompleted || result.Err != nil {
				t.Fatalf("retry calls = %v, result = %+v", calls, result)
			}
		})
	}
}

func TestProcessorRejectsInvalidTaskBeforeCalls(t *testing.T) {
	t.Parallel()
	task := cancellationTask()
	task.Steps[len(task.Steps)-1] = entities.NewOperationAction(entities.DecreaseLoyaltyOperation{User: operationUser})
	result := newTestProcessor(nil, nil, nil).Execute(t.Context(), task, entities.ExecutionBackground)
	if result.Status != entities.OperationFailed || !errors.Is(result.Err, entities.ErrInvalidInput) ||
		!reflect.DeepEqual(result.Task, task) {
		t.Fatalf("invalid task result = %+v", result)
	}
}

func TestProcessorCanceledContextMakesNoCalls(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	task := cancellationTask()
	result := newTestProcessor(nil, nil, nil).Execute(ctx, task, entities.ExecutionBackground)
	if result.Status != entities.OperationDeferred || !errors.Is(result.Err, context.Canceled) ||
		!reflect.DeepEqual(result.Task, task) {
		t.Fatalf("canceled task result = %+v", result)
	}
}

func TestProcessorCompletesWithoutOutstandingSteps(t *testing.T) {
	t.Parallel()
	task := entities.Operation{
		Kind: entities.OperationCancellation,
		Key:  testKey("done"),
	}
	result := newTestProcessor(nil, nil, nil).Execute(t.Context(), task, entities.ExecutionBackground)
	if result.Status != entities.OperationCompleted || result.Err != nil || !reflect.DeepEqual(result.Task, task) {
		t.Fatalf("completed task result = %+v", result)
	}
}

func newTestProcessor(r reservationifc.Canceler, p paymentifc.Canceler, l loyaltyifc.Updater) *Processor {
	return New(NewExecutor(NewActions(r, p, l)))
}

func operationPayload[T entities.OperationPayload](t *testing.T, task entities.Operation) T {
	t.Helper()
	for _, action := range task.Steps {
		if data, err := entities.OperationData[T](action); err == nil {
			return data
		}
	}
	t.Fatalf("operation lacks expected payload: %+v", task)
	var zero T
	return zero
}
