package opstream

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
	"uuid"

	"lab2/gateway/internal/models/entities"
)

const (
	operationUser      = "user"
	loyaltyOperationID = "stable"
)

func streamTask() entities.Operation {
	return entities.Operation{
		Kind: entities.OperationCancellation,
		Key:  testKey("cancel"),
		Steps: []entities.Action{
			entities.NewOperationAction(entities.CancelPaymentOperation{PaymentUID: uuid.New()}),
			entities.NewOperationAction(entities.CancelReservationOperation{User: operationUser, ReservationUID: uuid.New()}),
			entities.NewOperationAction(entities.DecreaseLoyaltyOperation{User: operationUser, OperationID: loyaltyOperationID}),
		},
	}
}

func testStream(execute executorFunc) *Stream {
	return NewMemory(queueConfig(), execute, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestStreamResumesInitialAttemptWithoutRepeatingSuccessfulSteps(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		task := streamTask()
		var calls atomic.Int32
		stream := testStream(func(_ context.Context, operation entities.Operation) entities.OperationResult {
			data, err := entities.OperationData[entities.DecreaseLoyaltyOperation](operation.Steps[len(operation.Steps)-1])
			if err != nil || operation.Key != task.Key || data.OperationID != loyaltyOperationID {
				t.Error("operation identifiers changed", operation)
			}
			if calls.Add(1) == 1 {
				operation.Steps = operation.Steps[len(operation.Steps)-1:]
				return entities.NewOperationResult(operation, entities.ErrLoyaltyUnavailable)
			}
			if entities.HasAction[entities.CancelPaymentOperation](operation) ||
				entities.HasAction[entities.CancelReservationOperation](operation) ||
				!entities.HasAction[entities.DecreaseLoyaltyOperation](operation) {
				t.Error("successful initial steps replayed", operation)
			}
			operation.Steps = nil
			return entities.NewOperationResult(operation, nil)
		})
		_, _ = stream.Reserve(testKey(task.Key))
		result := stream.Execute(t.Context(), task)
		if result.Status != entities.OperationDeferred || !errors.Is(result.Err, entities.ErrLoyaltyUnavailable) {
			t.Fatal("initial result lost", result)
		}
		if _, err := stream.Reserve(testKey("other")); !errors.Is(err, entities.ErrQueueFull) {
			t.Fatal("deferred operation lost capacity", err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { defer close(done); stream.Run(ctx) }()
		synctest.Wait()
		time.Sleep(queueConfig().RetryInterval)
		synctest.Wait()
		if calls.Load() != 2 {
			t.Fatal("background continuation did not execute", calls.Load())
		}
		if _, pending := stream.Pending(testKey(task.Key)); pending {
			t.Fatal("completed operation remains pending")
		}
		cancel()
		<-done
	})
}

func TestStreamPaymentFailureReleasesSlotWithoutBackgroundCancellation(t *testing.T) {
	t.Parallel()
	task := streamTask()
	stream := testStream(func(_ context.Context, operation entities.Operation) entities.OperationResult {
		result := entities.NewOperationResult(operation, entities.ErrUpstreamUnavailable)
		result.Status = entities.OperationRejected
		return result
	})
	_, _ = stream.Reserve(testKey(task.Key))
	result := stream.Execute(t.Context(), task)
	if !errors.Is(result.Err, entities.ErrUpstreamUnavailable) ||
		!entities.HasAction[entities.CancelPaymentOperation](result.Task) ||
		result.Status != entities.OperationRejected {
		t.Fatal("payment failure lost", result)
	}
	if _, pending := stream.Pending(testKey(task.Key)); pending {
		t.Fatal("failed Payment cancellation queued")
	}
	if ok, err := stream.Reserve(testKey("other")); !ok || err != nil {
		t.Fatal("failed synchronous operation retained capacity", ok, err)
	}
}

func TestStreamRejectsDuplicateInitialExecution(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		task := streamTask()
		release := make(chan struct{})
		var calls atomic.Int32
		stream := testStream(func(_ context.Context, operation entities.Operation) entities.OperationResult {
			calls.Add(1)
			<-release
			operation.Steps = nil
			return entities.NewOperationResult(operation, nil)
		})
		_, _ = stream.Reserve(testKey(task.Key))
		done := make(chan struct{})
		go func() { defer close(done); stream.Execute(t.Context(), task) }()
		synctest.Wait()
		duplicate := stream.Execute(t.Context(), task)
		if !errors.Is(duplicate.Err, entities.ErrInvalidInput) || calls.Load() != 1 {
			t.Fatal("duplicate initial operation executed", duplicate, calls.Load())
		}
		if _, claimed := stream.store.(memoryStore).queue.take(testKey(task.Key)); claimed {
			t.Fatal("worker claimed synchronous operation")
		}
		close(release)
		<-done
	})
}

func TestStreamCompensationUsesCallerContext(t *testing.T) {
	t.Parallel()
	task := streamTask()
	task.Kind = entities.OperationCompensation
	task.Steps = []entities.Action{task.Steps[1], task.Steps[0]}
	callerCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	stream := testStream(func(ctx context.Context, operation entities.Operation) entities.OperationResult {
		if ctx != callerCtx {
			t.Error("stream replaced the caller context")
		}
		operation.Steps = nil
		return entities.NewOperationResult(operation, nil)
	})
	_, _ = stream.Reserve(testKey(task.Key))
	if result := stream.Execute(callerCtx, task); result.Err != nil || !result.Task.Done() {
		t.Fatal("compensation failed", result)
	}
}

func TestStreamRequiresReservationAndRejectsInvalidPayload(t *testing.T) {
	t.Parallel()
	stream := testStream(func(_ context.Context, operation entities.Operation) entities.OperationResult {
		t.Error("invalid operation reached handler")
		return entities.NewOperationResult(operation, nil)
	})
	task := streamTask()
	if result := stream.Execute(t.Context(), task); !errors.Is(result.Err, entities.ErrInvalidInput) {
		t.Fatal("unreserved operation accepted", result)
	}
	_, _ = stream.Reserve(testKey(task.Key))
	task.Steps[len(task.Steps)-1] = entities.NewOperationAction(entities.DecreaseLoyaltyOperation{User: "user"})
	if result := stream.Execute(t.Context(), task); !errors.Is(result.Err, entities.ErrInvalidInput) {
		t.Fatal("invalid operation accepted", result)
	}
	if _, pending := stream.Pending(testKey(task.Key)); pending {
		t.Fatal("invalid operation retained capacity")
	}
}

func (s *Stream) Reserve(key uuid.UUID) (bool, error) {
	return s.store.(memoryStore).queue.Reserve(testKey(key))
}

func (s *Stream) Pending(key uuid.UUID) (entities.Operation, bool) {
	return s.store.(memoryStore).queue.Pending(testKey(key))
}
