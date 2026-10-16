package coreifc

import (
	"context"
	"uuid"

	"lab2/loyalty/internal/models/entities"
)

type Loyalties interface {
	ResolveCreation(context.Context, string, uuid.UUID) (bool, error)
	Get(context.Context, string) (entities.Loyalty, error)
	Change(context.Context, string, int, string) (entities.Loyalty, error)
}
