package entities

import (
	"errors"
	"reflect"
	"testing"
	"uuid"
)

const operationUser = "user"

func validRetry() Operation {
	return Operation{
		Kind: OperationCancellation,
		Key:  testKey("cancel"),
		Steps: []Action{
			NewOperationAction(CancelPaymentOperation{PaymentUID: uuid.New()}),
			NewOperationAction(CancelReservationOperation{User: operationUser, ReservationUID: uuid.New()}),
			NewOperationAction(DecreaseLoyaltyOperation{User: operationUser, OperationID: "stable"}),
		},
	}
}

func TestRetryValidate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit func(*Operation)
	}{
		{"unknown kind", func(task *Operation) { task.Kind = OperationKindUnknown }},
		{"unsupported kind", func(task *Operation) { task.Kind = OperationKind(99) }},
		{"wrong step order", func(task *Operation) {
			task.Steps[0] = NewOperationAction(DecreaseLoyaltyOperation{User: operationUser, OperationID: "stable"})
		}},
		{"missing DTO", func(task *Operation) { task.Steps[0] = nil }},
		{"missing key", func(task *Operation) { task.Key = testKey("") }},
		{"missing user", func(task *Operation) {
			task.Steps[1] = NewOperationAction(CancelReservationOperation{ReservationUID: uuid.New()})
		}},
		{"missing payment", func(task *Operation) {
			task.Steps[0] = NewOperationAction(CancelPaymentOperation{})
		}},
		{"missing reservation", func(task *Operation) {
			task.Steps[1] = NewOperationAction(CancelReservationOperation{User: operationUser})
		}},
		{"missing operation", func(task *Operation) {
			task.Steps[2] = NewOperationAction(DecreaseLoyaltyOperation{User: operationUser})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			task := validRetry()
			tc.edit(&task)
			if !errors.Is(task.Validate(), ErrInvalidInput) {
				t.Fatal("invalid task accepted")
			}
		})
	}
}

func TestRetryValidateRequiresOnlyPendingSteps(t *testing.T) {
	t.Parallel()
	for _, kind := range []OperationKind{OperationCancellation, OperationCompensation} {
		task := Operation{
			Kind: kind,
			Key:  testKey("done"),
		}
		if err := task.Validate(); err != nil || !task.Done() {
			t.Fatalf("completed task = %+v, validation = %v", task, err)
		}
	}
	id := uuid.New()
	createKey := OperationKey(OperationCompensation, id)
	cancelKey := OperationKey(OperationCancellation, id)
	if createKey == uuid.Nil() || createKey == cancelKey || cancelKey != OperationKey(OperationCancellation, id) {
		t.Fatal("operation keys are not isolated and stable")
	}
	if err := validRetry().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestOperationResult(t *testing.T) {
	t.Parallel()
	cause := errors.New("operation failed")
	for _, tc := range []struct {
		name   string
		task   Operation
		err    error
		status OperationStatus
	}{
		{"done", Operation{}, nil, OperationCompleted},
		{"pending", validRetry(), nil, OperationDeferred},
		{"failed", Operation{}, cause, OperationDeferred},
		{"pending failed", validRetry(), cause, OperationDeferred},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := NewOperationResult(tc.task, tc.err)
			if result.Status != tc.status || !reflect.DeepEqual(result.Task, tc.task) || !errors.Is(result.Err, tc.err) {
				t.Fatalf("result = %+v, want status %v, task %+v, error %v", result, tc.status, tc.task, tc.err)
			}
		})
	}
}
