package bookings

import (
	"context"
	"database/sql"
	"errors"

	"lab2/reservation/ent/bookingrequest"

	"github.com/samber/oops"

	"lab2/platform/observability/metrics"

	"lab2/reservation/ent"
	"lab2/reservation/ent/hotel"
	"lab2/reservation/ent/reservation"
	"lab2/reservation/internal/models/entities"
	bookingconverter "lab2/reservation/internal/postgres/converters/bookings"
)

func (r *Repository) Create(ctx context.Context, booking entities.Booking) (_ entities.Reservation, err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.Create), &err)
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return entities.Reservation{}, oops.Wrapf(err, "begin reservation creation")
	}
	defer func() {
		if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
			err = oops.Wrap(errors.Join(err, e))
		}
	}()
	err = tx.BookingRequest.Create().SetOperationID(booking.ReservationUID.String()).SetUsername(booking.Username).
		OnConflictColumns(bookingrequest.FieldOperationID).DoNothing().Exec(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return entities.Reservation{}, oops.Wrapf(err, "claim reservation request")
	}
	guard, err := tx.BookingRequest.Query().
		Where(bookingrequest.OperationIDEQ(booking.ReservationUID.String())).ForUpdate().Only(ctx)
	if err != nil {
		return entities.Reservation{}, oops.Wrapf(err, "lock reservation request")
	}
	if guard.Canceled || guard.Username != booking.Username {
		return entities.Reservation{}, oops.Wrap(entities.ErrInvalidInput)
	}
	existing, err := tx.Reservation.Query().
		Where(reservation.ReservationUIDEQ(booking.ReservationUID)).WithHotel().Only(ctx)
	if err == nil {
		if existing.Username != booking.Username || existing.PaymentUID != booking.PaymentUID ||
			existing.Edges.Hotel.HotelUID != booking.HotelUID || !existing.StartDate.Equal(booking.StartDate) ||
			!existing.EndDate.Equal(booking.EndDate) || existing.Status != reservation.StatusPAID {
			return entities.Reservation{}, oops.Wrap(entities.ErrInvalidInput)
		}
		item, err := bookingconverter.Reservation(ctx, existing)
		if err != nil {
			return entities.Reservation{}, oops.Wrap(err)
		}
		if err = tx.Commit(); err != nil {
			return entities.Reservation{}, oops.Wrapf(err, "commit repeated reservation")
		}
		return item, nil
	}
	if !ent.IsNotFound(err) {
		return entities.Reservation{}, oops.Wrapf(err, "get existing reservation")
	}

	h, err := tx.Hotel.Query().
		Where(hotel.HotelUIDEQ(booking.HotelUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return entities.Reservation{}, oops.In("repository").
			Code("not_found").
			With("entity", "hotel").
			Public("not found").
			Wrap(entities.ErrNotFound)
	}
	if err != nil {
		return entities.Reservation{}, oops.FromContext(ctx).
			In("bookings.postgres").
			With("operation", "get_hotel").
			Wrapf(err, "operation failed")
	}
	row, err := tx.Reservation.Create().
		SetReservationUID(booking.ReservationUID).
		SetUsername(booking.Username).
		SetPaymentUID(booking.PaymentUID).
		SetHotelID(h.ID).
		SetStatus(reservation.Status(entities.StatusPaid)).
		SetStartDate(booking.StartDate).
		SetEndDate(booking.EndDate).
		Save(ctx)
	if err != nil {
		return entities.Reservation{}, oops.FromContext(ctx).
			In("bookings.postgres").
			With("operation", "create").
			Wrapf(err, "operation failed")
	}
	row.Edges.Hotel = h
	item, err := bookingconverter.Reservation(ctx, row)
	if err != nil {
		return entities.Reservation{}, oops.Wrapf(err, "map reservation")
	}
	if err = tx.Commit(); err != nil {
		return entities.Reservation{}, oops.Wrapf(err, "commit reservation creation")
	}
	return item, nil
}
