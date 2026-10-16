package dbifc

import (
	"context"
	"uuid"

	"lab2/reservation/internal/models/entities"
)

type Bookings interface {
	List(context.Context, string) ([]entities.Reservation, error)
	Get(context.Context, uuid.UUID, string) (entities.Reservation, error)
	Create(context.Context, entities.Booking) (entities.Reservation, error)
	Cancel(context.Context, uuid.UUID, string) error
	CancelKnown(context.Context, uuid.UUID, string) error
}

//go:generate go tool mockgen -source=bookings.go -destination=../../core/usecases/bookings/repository_mock_test.go -package=bookings Bookings
