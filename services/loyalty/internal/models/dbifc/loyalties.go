package dbifc

import (
	"context"

	"lab2/loyalty/internal/models/entities"
)

type Loyalties interface {
	ResolveCreation(context.Context, entities.CreationResolution) (bool, error)
	Get(context.Context, string) (entities.Loyalty, error)
	Change(context.Context, entities.LoyaltyChange) (entities.Loyalty, error)
}
