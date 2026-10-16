package operationifc

import (
	"context"

	"lab2/gateway/internal/models/entities"
)

type ActionExecutor interface {
	Execute(context.Context, entities.Action) (bool, error)
}
