package operations

import (
	"context"
	"runtime/debug"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/operationifc"
)

type Executor struct{ binder entities.ActionBinder }

var _ operationifc.ActionExecutor = (*Executor)(nil)

func NewExecutor(binder entities.ActionBinder) *Executor {
	return &Executor{binder: binder}
}

// Execute recovers at each action boundary so confirmed steps stay confirmed.
func (e *Executor) Execute(ctx context.Context, action entities.Action) (stop bool, err error) {
	defer func() {
		if value := recover(); value != nil {
			err = entities.NewOperationPanic(value, debug.Stack())
		}
	}()
	if action == nil {
		return false, oops.Wrapf(entities.ErrInvalidInput, "missing operation action")
	}
	if err := action.Validate(); err != nil {
		return false, oops.Wrap(err)
	}
	bound := action.Bind(e.binder)
	stop, err = bound.Accept(ctx)
	return stop, oops.Wrap(err)
}
