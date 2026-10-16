package coreifc

import (
	"context"
	"uuid"

	"lab2/reservation/internal/models/entities"
)

//go:generate go tool mockgen -source=bookings.go -destination=../../rest/bookings_mock_test.go -package=resttransport Bookings

type Bookings interface {
	List(context.Context, string) ([]entities.Reservation, error)
	Get(context.Context, uuid.UUID, string) (entities.Reservation, error)
	Create(context.Context, string, entities.CreateInput) (entities.Reservation, error)
	Cancel(context.Context, uuid.UUID, string) error
	CancelKnown(context.Context, uuid.UUID, string) error
}
