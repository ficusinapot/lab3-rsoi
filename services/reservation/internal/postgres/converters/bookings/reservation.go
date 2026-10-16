package bookings

import (
	"context"

	"github.com/samber/oops"
	"lab2/reservation/ent"
	"lab2/reservation/internal/models/entities"
)

func Reservation(ctx context.Context, r *ent.Reservation) (entities.Reservation, error) {
	h, err := r.Edges.HotelOrErr()
	if err != nil {
		return entities.Reservation{}, oops.FromContext(ctx).
			In("bookings.postgres").
			With("operation", "load_hotel").
			Wrapf(err, "operation failed")
	}
	if r.StartDate == nil || r.EndDate == nil {
		return entities.Reservation{}, oops.In("bookings.postgres").
			Code("invalid_stored_booking").
			Errorf("reservation dates are missing")
	}
	return entities.Reservation{
		ReservationUID: r.ReservationUID, HotelUID: h.HotelUID,
		PaymentUID: r.PaymentUID, Status: entities.Status(r.Status),
		StartDate: r.StartDate.UTC(), EndDate: r.EndDate.UTC(),
	}, nil
}
