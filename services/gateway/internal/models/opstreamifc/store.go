package opstreamifc

import (
	"context"
	"uuid"

	"lab2/gateway/internal/models/entities"
)

type Attempt struct {
	Task   entities.Operation
	Token  string
	Number uint64
}

// Store owns atomic publication, leases and finalization; it never calls downstream services.
type Store interface {
	Reserve(context.Context, uuid.UUID, entities.OperationKind) (bool, error)
	Release(context.Context, uuid.UUID) error
	Pending(context.Context, uuid.UUID) (entities.Operation, bool, error)
	Prepare(context.Context, entities.Operation) error
	Start(context.Context, entities.Operation) (Attempt, bool, error)
	// ReadyKeys receives deliveries for this store; abandoned deliveries become claimable after the lease.
	ReadyKeys(context.Context, string) ([]uuid.UUID, error)
	Take(context.Context, uuid.UUID) (Attempt, bool, error)
	Finish(context.Context, Attempt, entities.OperationResult) (bool, error)
	Remaining(context.Context) (int, error)
}
