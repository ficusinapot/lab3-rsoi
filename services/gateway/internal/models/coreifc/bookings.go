package coreifc

import (
	"context"
	"uuid"

	"lab2/gateway/internal/models/entities"
)

type BookingQueries interface {
	Hotels(context.Context, entities.PageQuery) (entities.Page, error)
	List(context.Context, string) ([]entities.Reservation, error)
	Get(context.Context, uuid.UUID, string) (entities.Reservation, error)
	Me(context.Context, string) (entities.UserInfo, error)
	Loyalty(context.Context, string) (entities.Loyalty, error)
}

type BookingCommands interface {
	Create(context.Context, entities.CreateBooking) (entities.Created, error)
	Cancel(context.Context, entities.CancelBooking) error
}
