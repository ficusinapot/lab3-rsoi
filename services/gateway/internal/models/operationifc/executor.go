package operationifc

import (
	"context"

	"lab2/gateway/internal/models/entities"
)

type Executor interface {
	Execute(context.Context, entities.Operation, entities.ExecutionMode) entities.OperationResult
}
