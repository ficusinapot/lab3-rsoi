package operationifc

import (
	"context"
	"uuid"

	"lab2/gateway/internal/models/entities"
)

// Groups owns the synchronous lifecycle of a reserved operation group.
type Groups interface {
	Create(context.Context, string, func(Creation) error) error
	Cancel(context.Context, string, uuid.UUID) error
}

// Creation performs writes while maintaining their compensation plan.
type Creation interface {
	CreatePayment(context.Context, int) (entities.InternalPayment, error)
	CreateReservation(context.Context, entities.CreateBooking) (entities.InternalReservation, error)
	IncreaseLoyalty(context.Context) error
}
