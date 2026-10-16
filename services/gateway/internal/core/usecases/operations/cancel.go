package operations

import (
	"context"
	"fmt"

	"github.com/samber/oops"

	"lab2/gateway/internal/models/entities"
)

func (p *Processor) cancel(ctx context.Context, task entities.Operation) entities.OperationResult {
	for len(task.Steps) > 0 {
		action := task.Steps[0]
		if _, err := p.handlers.Execute(ctx, action); err != nil {
			return entities.NewOperationResult(task,
				oops.With("action", fmt.Sprintf("%T", action.Data())).Wrapf(err, "execute operation step"))
		}
		task.Steps = task.Steps[1:]
	}
	return entities.NewOperationResult(task, nil)
}
