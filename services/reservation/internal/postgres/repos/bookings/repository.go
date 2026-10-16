package bookings

import (
	"context"
	"database/sql"
	"errors"
	"uuid"

	"lab2/reservation/ent/bookingrequest"

	"github.com/samber/oops"

	"lab2/platform/observability/metrics"

	"lab2/reservation/ent"
	"lab2/reservation/ent/reservation"
	"lab2/reservation/internal/models/entities"
	bookingconverter "lab2/reservation/internal/postgres/converters/bookings"
)

type Repository struct{ client *ent.Client }

func New(client *ent.Client) *Repository { return &Repository{client: client} }

func (r *Repository) List(ctx context.Context, name string) (_ []entities.Reservation, err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.List), &err)
	rows, err := r.client.Reservation.Query().
		Where(reservation.UsernameEQ(name)).
		WithHotel().
		Order(ent.Asc(reservation.FieldID)).
		All(ctx)
	if err != nil {
		return nil, oops.FromContext(ctx).
			In("bookings.postgres").
			With("operation", "list").
			Wrapf(err, "operation failed")
	}
	items := make([]entities.Reservation, 0, len(rows))
	for _, row := range rows {
		item, err := bookingconverter.Reservation(ctx, row)
		if err != nil {
			return nil, oops.Wrapf(err, "map reservation")
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID, name string) (_ entities.Reservation, err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.Get), &err)
	row, err := r.client.Reservation.Query().
		Where(reservation.ReservationUIDEQ(id), reservation.UsernameEQ(name)).
		WithHotel().
		Only(ctx)
	if ent.IsNotFound(err) {
		return entities.Reservation{}, oops.In("repository").
			Code("not_found").
			With("entity", "reservation").
			Public("not found").
			Wrap(entities.ErrNotFound)
	}
	if err != nil {
		return entities.Reservation{}, oops.FromContext(ctx).
			In("bookings.postgres").
			With("operation", "get").
			Wrapf(err, "operation failed")
	}
	item, err := bookingconverter.Reservation(ctx, row)
	return item, oops.Wrapf(err, "map reservation")
}

func (r *Repository) Cancel(ctx context.Context, id uuid.UUID, name string) (err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.Cancel), &err)
	if _, err = r.Get(ctx, id, name); err != nil {
		return err
	}
	return r.CancelKnown(ctx, id, name)
}

func (r *Repository) CancelKnown(ctx context.Context, id uuid.UUID, name string) (err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.CancelKnown), &err)
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return oops.Wrapf(err, "begin reservation cancellation")
	}
	defer func() {
		if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
			err = oops.Wrap(errors.Join(err, e))
		}
	}()
	err = tx.BookingRequest.Create().SetOperationID(id.String()).SetUsername(name).SetCanceled(true).
		OnConflictColumns(bookingrequest.FieldOperationID).DoNothing().Exec(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return oops.Wrapf(err, "claim reservation cancellation")
	}
	guard, err := tx.BookingRequest.Query().Where(bookingrequest.OperationIDEQ(id.String())).ForUpdate().Only(ctx)
	if err != nil {
		return oops.Wrapf(err, "lock reservation cancellation")
	}
	if guard.Username != name {
		return oops.Wrap(entities.ErrInvalidInput)
	}
	// Existing lab2 rows might not yet have a guard: check ownership before committing it.
	row, err := tx.Reservation.Query().Where(reservation.ReservationUIDEQ(id)).Only(ctx)
	if err == nil && row.Username != name {
		return oops.Wrap(entities.ErrNotFound)
	}
	if err != nil && !ent.IsNotFound(err) {
		return oops.Wrapf(err, "get reservation for cancellation")
	}
	if err = tx.BookingRequest.UpdateOneID(guard.ID).SetCanceled(true).Exec(ctx); err != nil {
		return oops.Wrapf(err, "cancel reservation request")
	}
	if _, err = tx.Reservation.Update().Where(reservation.ReservationUIDEQ(id), reservation.UsernameEQ(name)).
		SetStatus(reservation.StatusCANCELED).Save(ctx); err != nil {
		return oops.Wrapf(err, "cancel reservation")
	}
	return oops.Wrapf(tx.Commit(), "commit reservation cancellation")
}
