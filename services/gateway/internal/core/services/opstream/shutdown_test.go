package opstream

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"lab2/gateway/internal/models/entities"
)

func TestStreamExecutorPanicReleasesClaimAndWorkerContinues(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		task := streamTask()
		task.Kind = entities.OperationCompensation
		task.Steps = []entities.Action{task.Steps[1], task.Steps[0]}
		var calls atomic.Int32
		stream := testStream(func(_ context.Context, operation entities.Operation) entities.OperationResult {
			if calls.Add(1) <= 2 {
				panic("handler panic")
			}
			operation.Steps = nil
			return entities.NewOperationResult(operation, nil)
		})
		_, _ = stream.Reserve(testKey(task.Key))
		result := stream.Execute(t.Context(), task)
		if !errors.Is(result.Err, entities.ErrOperationPanic) || result.Status != entities.OperationDeferred {
			t.Fatal("initial panic was not deferred", result)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { defer close(done); stream.Run(ctx) }()
		synctest.Wait()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		stream.store.(memoryStore).queue.mu.Lock()
		item := stream.store.(memoryStore).queue.tasks[task.Key]
		stream.store.(memoryStore).queue.mu.Unlock()
		if item.claim != 0 || !item.ready || calls.Load() != 2 {
			t.Fatal("background panic left a claimed operation", item, calls.Load())
		}
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if _, pending := stream.Pending(testKey(task.Key)); pending || calls.Load() != 3 {
			t.Fatal("worker did not survive panic", pending, calls.Load())
		}
		cancel()
		<-done
	})
}

func TestStreamDrainFinishesDeferredWorkAfterWorkerStops(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		task := streamTask()
		var calls atomic.Int32
		stream := testStream(func(_ context.Context, operation entities.Operation) entities.OperationResult {
			operation.Steps = operation.Steps[len(operation.Steps)-1:]
			if calls.Add(1) == 1 {
				return entities.NewOperationResult(operation, entities.ErrLoyaltyUnavailable)
			}
			operation.Steps = nil
			return entities.NewOperationResult(operation, nil)
		})
		ctx, stop := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { defer close(done); stream.Run(ctx) }()
		synctest.Wait()
		// A request finishing during HTTP shutdown can still enqueue its remaining step.
		_, _ = stream.Reserve(testKey(task.Key))
		stream.Execute(t.Context(), task)
		stop()
		<-done
		started := time.Now()
		drainCtx, cancel := context.WithTimeout(t.Context(), stream.cfg.ShutdownTimeout)
		defer cancel()
		if err := stream.Drain(drainCtx); err != nil {
			t.Fatal(err)
		}
		if stream.store.(memoryStore).queue.remaining() != 0 || calls.Load() != 2 ||
			time.Since(started) < stream.cfg.RetryInterval {
			t.Fatal("drain lost work or skipped retry interval", calls.Load(), time.Since(started))
		}
	})
}

func TestStreamDrainDeadlineRetainsRemainingWork(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		task := streamTask()
		var calls atomic.Int32
		stream := testStream(func(ctx context.Context, operation entities.Operation) entities.OperationResult {
			operation.Steps = operation.Steps[len(operation.Steps)-1:]
			if calls.Add(1) > 1 {
				<-ctx.Done()
				return entities.NewOperationResult(operation, ctx.Err())
			}
			return entities.NewOperationResult(operation, entities.ErrLoyaltyUnavailable)
		})
		_, _ = stream.Reserve(testKey(task.Key))
		stream.Execute(t.Context(), task)
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		if err := stream.Drain(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("drain did not honor shutdown deadline", err)
		}
		item := stream.store.(memoryStore).queue.tasks[task.Key]
		if item.claim != 0 || !item.ready || !entities.HasAction[entities.DecreaseLoyaltyOperation](item.task) ||
			entities.HasAction[entities.CancelPaymentOperation](item.task) {
			t.Fatal("drain lost outstanding work or retained claim", item)
		}
	})
}
