package reservationifc

import (
	"context"
	"uuid"

	"lab2/gateway/internal/models/entities"
)

type Canceler interface {
	CancelReservation(context.Context, uuid.UUID, string) error
}

type Reader interface {
	Hotels(context.Context, entities.PageQuery) (entities.Page, error)
	Hotel(context.Context, uuid.UUID) (entities.Hotel, error)
	Reservations(context.Context, string) ([]entities.InternalReservation, error)
	Reservation(context.Context, uuid.UUID, string) (entities.InternalReservation, error)
}

type Bookings interface {
	Reader
	BookingSource
	Canceler
}

type BookingSource interface {
	Hotel(context.Context, uuid.UUID) (entities.Hotel, error)
	Reservation(context.Context, uuid.UUID, string) (entities.InternalReservation, error)
	CreateReservation(context.Context, string, entities.InternalCreate) (entities.InternalReservation, error)
}
