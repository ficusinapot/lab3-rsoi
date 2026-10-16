package hotels

import (
	"context"
	"database/sql"
	"errors"
	"uuid"

	"lab2/platform/observability/metrics"

	"github.com/samber/oops"
	"lab2/reservation/ent"
	"lab2/reservation/ent/hotel"
	"lab2/reservation/internal/models/entities"
	hotelconverter "lab2/reservation/internal/postgres/converters/hotels"
)

type Repository struct{ client *ent.Client }

func New(client *ent.Client) *Repository { return &Repository{client: client} }

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (_ entities.Hotel, err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.Get), &err)
	h, err := r.client.Hotel.Query().
		Where(hotel.HotelUIDEQ(id)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return entities.Hotel{}, oops.In("repository").
			Code("not_found").
			With("entity", "hotel").
			Public("not found").
			Wrap(entities.ErrNotFound)
	}
	if err != nil {
		return entities.Hotel{}, oops.FromContext(ctx).
			In("hotels.postgres").
			With("operation", "get").
			Wrapf(err, "operation failed")
	}
	return hotelconverter.Hotel(h), nil
}

func (r *Repository) List(ctx context.Context, page, size int) (_ entities.Page, err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.List), &err)
	tx, err := r.client.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return entities.Page{}, oops.FromContext(ctx).
			In("hotels.postgres").
			With("operation", "begin_snapshot").
			Wrapf(err, "operation failed")
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, oops.FromContext(ctx).
				In("hotels.postgres").
				With("operation", "rollback_snapshot").
				Wrapf(rollbackErr, "operation failed"))
		}
	}()
	result, queryErr := list(ctx, tx, page, size)
	if queryErr != nil {
		return entities.Page{}, oops.In("hotels.postgres").
			With("operation", "list").
			Wrap(queryErr)
	}
	if err := tx.Commit(); err != nil {
		return entities.Page{}, oops.FromContext(ctx).
			In("hotels.postgres").
			With("operation", "commit_snapshot").
			Wrapf(err, "operation failed")
	}
	return result, nil
}

func list(ctx context.Context, tx *ent.Tx, page, size int) (entities.Page, error) {
	count, err := tx.Hotel.Query().Count(ctx)
	if err != nil {
		return entities.Page{}, oops.FromContext(ctx).
			In("hotels.postgres").
			With("operation", "count").
			Wrapf(err, "operation failed")
	}
	rows, err := tx.Hotel.Query().
		Order(ent.Asc(hotel.FieldID)).
		Offset((page - 1) * size).
		Limit(size).
		All(ctx)
	if err != nil {
		return entities.Page{}, oops.FromContext(ctx).
			In("hotels.postgres").
			With("operation", "list").
			Wrapf(err, "operation failed")
	}
	items := make([]entities.Hotel, 0, len(rows))
	for _, h := range rows {
		items = append(items, hotelconverter.Hotel(h))
	}
	return entities.Page{Page: page, PageSize: size, TotalElements: count, Items: items}, nil
}
