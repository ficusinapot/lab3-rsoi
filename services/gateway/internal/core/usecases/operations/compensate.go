package operations

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
)

func (p *Processor) compensate(ctx context.Context, task entities.Operation) entities.OperationResult {
	if entities.HasAction[entities.ResolveCreationOperation](task) {
		committed, err := p.handlers.Execute(ctx, task.Steps[0])
		if err != nil {
			return entities.NewOperationResult(task, oops.Wrapf(err, "resolve creation before rollback"))
		}
		task.Steps = task.Steps[1:]
		if committed {
			task.Steps = nil
			return entities.NewOperationResult(task, nil)
		}
	}
	var failures []error
	remaining := make([]entities.Action, 0, len(task.Steps))
	for _, step := range task.Steps {
		if _, err := p.handlers.Execute(ctx, step); err != nil {
			remaining = append(remaining, step)
			failures = append(failures,
				oops.With("action", fmt.Sprintf("%T", step.Data())).Wrapf(err, "execute operation step"))
		}
	}
	task.Steps = remaining
	return entities.NewOperationResult(task, oops.Wrap(errors.Join(failures...)))
}
