package opstream

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
	"uuid"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/operationifc"
	"lab2/gateway/internal/models/opstreamifc"
)

type worker struct {
	store    opstreamifc.Store
	executor operationifc.Executor
	cfg      Config
	logger   *slog.Logger
}

func newWorker(store opstreamifc.Store, executor operationifc.Executor, cfg Config, logger *slog.Logger) *worker {
	return &worker{store: store, executor: executor, cfg: cfg, logger: logger}
}

type workerPool struct {
	lane  string
	count int
}

func (w *worker) pools() [3]workerPool {
	return [3]workerPool{
		{"compensation", w.cfg.CompensationWorkers},
		{"cancellation", w.cfg.CancellationWorkers},
		{"loyalty", w.cfg.LoyaltyWorkers},
	}
}

func (w *worker) Run(ctx context.Context) {
	var workers sync.WaitGroup
	for _, pool := range w.pools() {
		for range pool.count {
			workers.Go(func() { w.runLane(ctx, pool.lane) })
		}
	}
	workers.Wait()
}

func (w *worker) runLane(ctx context.Context, lane string) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		w.runReady(ctx, lane)
	}
}

func (w *worker) runReady(ctx context.Context, lane string) {
	keys, err := w.store.ReadyKeys(ctx, lane)
	if err != nil {
		w.logger.WarnContext(ctx, "read ready operations", "error", err)
		return
	}
	for _, key := range keys {
		if ctx.Err() != nil {
			return
		}
		w.execute(ctx, key)
	}
}

func (w *worker) retry(ctx context.Context) {
	var workers sync.WaitGroup
	for _, pool := range w.pools() {
		for range pool.count {
			workers.Go(func() { w.runReady(ctx, pool.lane) })
		}
	}
	workers.Wait()
}

func (w *worker) execute(ctx context.Context, key uuid.UUID) {
	owned, ok, err := w.store.Take(ctx, key)
	if err != nil {
		w.logger.WarnContext(ctx, "take operation", "error", err)
		return
	}
	if !ok {
		return
	}
	attemptCtx, cancel := context.WithTimeout(ctx, w.cfg.AttemptTimeout)
	defer cancel()
	w.process(attemptCtx, owned, entities.ExecutionBackground)
}

func (w *worker) process(
	ctx context.Context,
	owned opstreamifc.Attempt,
	mode entities.ExecutionMode,
) entities.OperationResult {
	logger := w.logger.With("operation", owned.Task.Key, "kind", owned.Task.Kind, "attempt", owned.Number)
	result := w.invoke(ctx, owned.Task, mode)
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.cfg.StorageTimeout)
	defer cancel()
	accepted, err := w.store.Finish(finishCtx, owned, result)
	if err != nil {
		result.Err = oops.Wrap(errors.Join(result.Err, err))
		return result
	}
	if !accepted {
		result.Err = oops.Wrap(errors.Join(result.Err, entities.ErrOperationBusy))
		logger.WarnContext(ctx, "operation result superseded")
		return result
	}
	if result.Err != nil {
		logger.WarnContext(ctx, "operation attempt failed", "error", result.Err, "status", result.Status)
		return result
	}
	logger.DebugContext(ctx, "operation attempt finalized", "status", result.Status)
	return result
}

func (w *worker) invoke(
	ctx context.Context,
	task entities.Operation,
	mode entities.ExecutionMode,
) (result entities.OperationResult) {
	defer func() {
		if value := recover(); value != nil {
			result = entities.NewOperationResult(task, entities.NewOperationPanic(value, debug.Stack()))
		}
	}()
	return w.executor.Execute(ctx, task, mode)
}
