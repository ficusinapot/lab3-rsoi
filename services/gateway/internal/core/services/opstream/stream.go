package opstream

import (
	"context"
	"errors"
	"log/slog"
	"time"
	"uuid"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/operationifc"
	"lab2/gateway/internal/models/opstreamifc"
)

type Stream struct {
	store  opstreamifc.Store
	worker *worker
	cfg    Config
}

var _ opstreamifc.Stream = (*Stream)(nil)

// NewMemory builds an isolated stream for tests without Redis.
func NewMemory(cfg Config, executor operationifc.Executor, logger *slog.Logger) *Stream {
	return New(cfg, memoryStore{newQueue(cfg)}, executor, logger)
}

func New(cfg Config, store opstreamifc.Store, executor operationifc.Executor, logger *slog.Logger) *Stream {
	return &Stream{store: store, worker: newWorker(store, executor, cfg, logger), cfg: cfg}
}

func (s *Stream) ReserveOperation(ctx context.Context, key uuid.UUID, kind entities.OperationKind) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.StorageTimeout)
	defer cancel()

	ok, err := s.store.Reserve(ctx, key, kind)
	return ok, oops.Wrap(err)
}

func (s *Stream) ReleaseOperation(ctx context.Context, key uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.StorageTimeout)
	defer cancel()
	return oops.Wrap(s.store.Release(ctx, key))
}

func (s *Stream) PendingOperation(ctx context.Context, key uuid.UUID) (entities.Operation, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.StorageTimeout)
	defer cancel()

	task, ok, err := s.store.Pending(ctx, key)
	return task, ok, oops.Wrap(err)
}

func (s *Stream) Prepare(ctx context.Context, task entities.Operation) error {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.StorageTimeout)
	defer cancel()
	return oops.Wrap(s.store.Prepare(ctx, task))
}

// Run blocks until cancellation and waits for all in-flight background attempts.
func (s *Stream) Run(ctx context.Context) { s.worker.Run(ctx) }

// Drain runs after HTTP requests and the background worker have stopped.
func (s *Stream) Drain(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.RetryInterval)
	defer ticker.Stop()
	for {
		remaining, err := s.store.Remaining(ctx)
		if err != nil {
			return oops.Wrap(err)
		}
		if remaining == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return oops.With("remaining_operations", remaining).Wrapf(err, "drain operations")
		}
		s.worker.retry(ctx)
		remaining, err = s.store.Remaining(ctx)
		if err != nil {
			return oops.Wrap(err)
		}
		if remaining == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return oops.With("remaining_operations", remaining).Wrapf(ctx.Err(), "drain operations")
		case <-ticker.C:
		}
	}
}

func (s *Stream) Execute(ctx context.Context, task entities.Operation) entities.OperationResult {
	owned, ok, err := s.store.Start(ctx, task)
	if err != nil {
		return entities.OperationResult{Task: task, Status: entities.OperationRejected, Err: err}
	}
	if !ok {
		return entities.OperationResult{
			Task: task, Status: entities.OperationRejected,
			Err: oops.Wrapf(entities.ErrInvalidInput, "operation is not reserved"),
		}
	}
	if err := task.Validate(); err != nil {
		result := entities.NewOperationResult(task, err)
		result.Status = entities.OperationRejected
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cfg.StorageTimeout)
		defer cancel()
		accepted, finishErr := s.store.Finish(finishCtx, owned, result)
		result.Err = oops.Wrap(errors.Join(result.Err, finishErr))
		if finishErr == nil && !accepted {
			result.Err = oops.Wrap(errors.Join(result.Err, entities.ErrOperationBusy))
		}
		return result
	}

	return s.worker.process(ctx, owned, entities.ExecutionInitial)
}
