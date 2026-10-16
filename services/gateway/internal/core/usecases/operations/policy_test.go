package operations

import (
	"errors"
	"testing"

	"lab2/gateway/internal/models/entities"
)

func TestOperationRetryPolicy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		mode  entities.ExecutionMode
		cause error
		want  entities.OperationStatus
	}{
		{
			"initial payment failure releases slot", entities.ExecutionInitial, entities.ErrPaymentUnavailable,
			entities.OperationRejected,
		},
		{
			"recovered payment failure is retried", entities.ExecutionBackground, entities.ErrPaymentUnavailable,
			entities.OperationDeferred,
		},
		{
			"invalid retry is retained for operator", entities.ExecutionBackground, entities.ErrInvalidInput,
			entities.OperationFailed,
		},
		{
			"missing record is retained for operator", entities.ExecutionBackground, entities.ErrNotFound,
			entities.OperationFailed,
		},
		{
			"panic remains retryable", entities.ExecutionBackground,
			entities.NewOperationPanic(entities.ErrInvalidInput, nil), entities.OperationDeferred,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			task := cancellationTask()
			var calls []string
			processor := testProcessor(t, task, &calls, map[string]error{paymentStep: tc.cause})
			result := processor.Execute(t.Context(), task, tc.mode)
			if result.Status != tc.want || !errors.Is(result.Err, tc.cause) || result.Task.Done() {
				t.Fatalf("result = %+v, want status %v and cause %v", result, tc.want, tc.cause)
			}
		})
	}
}
