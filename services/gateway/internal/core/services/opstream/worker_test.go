package opstream

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"lab2/gateway/internal/models/entities"
)

type executorFunc func(context.Context, entities.Operation) entities.OperationResult

func (f executorFunc) Execute(
	ctx context.Context,
	task entities.Operation,
	_ entities.ExecutionMode,
) entities.OperationResult {
	return f(ctx, task)
}

func TestWorkerBoundsConcurrencyAndCompletesIndependentTasks(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cfg := queueConfig()
		cfg.Capacity, cfg.Concurrency, cfg.CompensationWorkers = 6, 2, 2
		q := newQueue(cfg)
		for index := range cfg.Capacity {
			key := strconv.Itoa(index)
			_, _ = q.Reserve(testKey(key))
			deferOperation(t, q, entities.Operation{
				Key: testKey(key),
				Steps: []entities.Action{
					entities.NewOperationAction(entities.CancelPaymentOperation{}),
				},
			})
		}
		var active, peak, calls atomic.Int32
		release := make(chan struct{})
		w := newWorker(memoryStore{q}, executorFunc(
			func(_ context.Context, task entities.Operation) entities.OperationResult {
				current := active.Add(1)
				for previous := peak.Load(); current > previous; previous = peak.Load() {
					if peak.CompareAndSwap(previous, current) {
						break
					}
				}
				calls.Add(1)
				<-release
				active.Add(-1)
				task.Steps = nil
				return entities.NewOperationResult(task, nil)
			}), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
		time.Sleep(cfg.RetryInterval)
		done := make(chan struct{})
		go func() { defer close(done); w.retry(t.Context()) }()
		synctest.Wait()
		if active.Load() != 2 || calls.Load() != 2 {
			t.Fatal("worker did not reach bounded concurrency", active.Load(), calls.Load())
		}
		close(release)
		<-done
		if peak.Load() != 2 || calls.Load() != 6 || len(q.tasks) != 0 {
			t.Fatal("work lost or concurrency exceeded", peak.Load(), calls.Load(), len(q.tasks))
		}
	})
}

func TestWorkerWaitsIntervalAfterAttemptFinishes(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cfg := queueConfig()
		cfg.AttemptTimeout = 5 * time.Second
		q := newQueue(cfg)
		_, _ = q.Reserve(testKey("a"))
		deferOperation(t, q, entities.Operation{
			Key: testKey("a"),
			Steps: []entities.Action{
				entities.NewOperationAction(entities.CancelPaymentOperation{}),
			},
		})
		var calls atomic.Int32
		w := newWorker(memoryStore{q}, executorFunc(
			func(ctx context.Context, task entities.Operation) entities.OperationResult {
				if calls.Add(1) == 1 {
					<-ctx.Done()
					return entities.NewOperationResult(task, ctx.Err())
				}
				task.Steps = nil
				return entities.NewOperationResult(task, nil)
			}), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { defer close(done); w.Run(ctx) }()
		synctest.Wait()
		time.Sleep(20 * time.Second)
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatal("attempt retried before ten seconds after completion", calls.Load())
		}
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if calls.Load() != 2 || len(q.tasks) != 0 {
			t.Fatal("due retry did not complete", calls.Load(), len(q.tasks))
		}
		cancel()
		<-done
	})
}

func testWorker(q *queue, execute executorFunc) *worker {
	return newWorker(memoryStore{q}, execute, queueConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestWorkerRetriesOnlyRemainingWork(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		q := newQueue(queueConfig())
		_, _ = q.Reserve(testKey("a"))
		deferOperation(t, q, entities.Operation{
			Kind: entities.OperationCancellation,
			Key:  testKey("a"),
			Steps: []entities.Action{
				entities.NewOperationAction(entities.CancelReservationOperation{User: operationUser}),
				entities.NewOperationAction(entities.DecreaseLoyaltyOperation{
					User: operationUser, OperationID: loyaltyOperationID,
				}),
			},
		})
		var calls atomic.Int32
		w := testWorker(q, func(_ context.Context, task entities.Operation) entities.OperationResult {
			data, err := entities.OperationData[entities.DecreaseLoyaltyOperation](task.Steps[len(task.Steps)-1])
			if err != nil || data.User != operationUser || data.OperationID != loyaltyOperationID {
				t.Error("retry lost stable payload", task)
			}
			if calls.Add(1) == 1 {
				task.Steps = task.Steps[len(task.Steps)-1:]
				return entities.NewOperationResult(task, entities.ErrUpstreamUnavailable)
			}
			if entities.HasAction[entities.CancelReservationOperation](task) ||
				!entities.HasAction[entities.DecreaseLoyaltyOperation](task) {
				t.Error("worker replayed successful work", task)
			}
			task.Steps = nil
			return entities.NewOperationResult(task, nil)
		})
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { defer close(done); w.Run(ctx) }()
		synctest.Wait()
		time.Sleep(9 * time.Second)
		synctest.Wait()
		if calls.Load() != 0 {
			t.Fatal("retry happened before interval")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if task, ok := q.Pending(testKey("a")); !ok || entities.HasAction[entities.CancelReservationOperation](task) ||
			!entities.HasAction[entities.DecreaseLoyaltyOperation](task) || calls.Load() != 1 {
			t.Fatal("first result was not finalized", task, ok, calls.Load())
		}
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if _, exists := q.Pending(testKey("a")); exists || calls.Load() != 2 {
			t.Fatal("completed task remains queued", exists, calls.Load())
		}
		cancel()
		<-done
	})
}

func TestWorkerTimeoutRetainsTask(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		q := newQueue(queueConfig())
		_, _ = q.Reserve(testKey("a"))
		deferOperation(t, q, entities.Operation{
			Key: testKey("a"),
			Steps: []entities.Action{
				entities.NewOperationAction(entities.CancelPaymentOperation{}),
			},
		})
		finished := make(chan struct{})
		w := testWorker(q, func(ctx context.Context, task entities.Operation) entities.OperationResult {
			<-ctx.Done()
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Error("attempt lacks timeout", ctx.Err())
			}
			close(finished)
			return entities.NewOperationResult(task, ctx.Err())
		})
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { defer close(done); w.Run(ctx) }()
		<-finished
		synctest.Wait()
		if task, ok := q.Pending(testKey("a")); !ok || !entities.HasAction[entities.CancelPaymentOperation](task) {
			t.Fatal("timed out work lost", task, ok)
		}
		cancel()
		<-done
	})
}

func TestWorkerShutdownCancelsAttempt(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		q := newQueue(queueConfig())
		_, _ = q.Reserve(testKey("a"))
		deferOperation(t, q, entities.Operation{
			Key: testKey("a"),
			Steps: []entities.Action{
				entities.NewOperationAction(entities.CancelPaymentOperation{}),
			},
		})
		started := make(chan struct{})
		w := testWorker(q, func(ctx context.Context, task entities.Operation) entities.OperationResult {
			close(started)
			<-ctx.Done()
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Error("attempt detached from application", ctx.Err())
			}
			return entities.NewOperationResult(task, ctx.Err())
		})
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { defer close(done); w.Run(ctx) }()
		<-started
		cancel()
		<-done
		if task, ok := q.Pending(testKey("a")); !ok || !entities.HasAction[entities.CancelPaymentOperation](task) {
			t.Fatal("canceled attempt lost outstanding step", task, ok)
		}
		if item := q.tasks[testKey("a")]; item.claim != 0 || !item.ready {
			t.Fatal("shutdown left a locked claim", item)
		}
	})
}
