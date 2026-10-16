package coreifc

import (
	"context"
	"uuid"

	"lab2/payment/internal/models/entities"
)

type Payments interface {
	CreateWithID(context.Context, uuid.UUID, int) (entities.Payment, error)
	Create(context.Context, int) (entities.Payment, error)
	Get(context.Context, uuid.UUID) (entities.Payment, error)
	Cancel(context.Context, uuid.UUID) error
	CancelKnown(context.Context, uuid.UUID) error
}
