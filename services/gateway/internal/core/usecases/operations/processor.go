package operations

import (
	"context"
	"errors"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/operationifc"
	"lab2/platform/observability/metrics"
)

type Processor struct{ handlers operationifc.ActionExecutor }

var _ operationifc.Executor = (*Processor)(nil)

func New(handlers operationifc.ActionExecutor) *Processor { return &Processor{handlers: handlers} }

func (p *Processor) Execute(
	ctx context.Context,
	task entities.Operation,
	mode entities.ExecutionMode,
) (result entities.OperationResult) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, p.Execute), &result.Err)
	if mode != entities.ExecutionInitial && mode != entities.ExecutionBackground {
		return entities.OperationResult{
			Task: task, Status: entities.OperationRejected, Err: oops.Wrap(entities.ErrInvalidInput),
		}
	}
	if err := task.Validate(); err != nil {
		result = entities.NewOperationResult(task, err)
	} else if err := ctx.Err(); err != nil {
		result = entities.NewOperationResult(task, oops.Wrap(err))
	} else if task.Kind == entities.OperationCompensation {
		result = p.compensate(ctx, task)
	} else {
		result = p.cancel(ctx, task)
	}

	if result.Err != nil {
		if mode == entities.ExecutionInitial && task.Kind == entities.OperationCancellation &&
			entities.HasAction[entities.CancelPaymentOperation](result.Task) {
			result.Status = entities.OperationRejected
		} else if mode == entities.ExecutionBackground && !errors.Is(result.Err, entities.ErrOperationPanic) &&
			(errors.Is(result.Err, entities.ErrInvalidInput) || errors.Is(result.Err, entities.ErrNotFound)) {
			result.Status = entities.OperationFailed
		}
	}
	return result
}
