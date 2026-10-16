package opstreamifc

import (
	"context"
	"uuid"

	"lab2/gateway/internal/models/entities"
)

type Stream interface {
	ReserveOperation(context.Context, uuid.UUID, entities.OperationKind) (bool, error)
	ReleaseOperation(context.Context, uuid.UUID) error
	PendingOperation(context.Context, uuid.UUID) (entities.Operation, bool, error)
	Prepare(context.Context, entities.Operation) error
	Execute(context.Context, entities.Operation) entities.OperationResult
}
