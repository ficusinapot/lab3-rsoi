package repos

import (
	"context"
	"database/sql"
	"errors"

	"lab2/loyalty/ent"
	"lab2/loyalty/ent/loyalty"
	"lab2/loyalty/ent/operation"
	"lab2/loyalty/internal/models/entities"
	"lab2/loyalty/internal/postgres/converters"
	"lab2/platform/observability/metrics"

	"github.com/samber/oops"
)

type Repository struct{ client *ent.Client }

func New(client *ent.Client) *Repository { return &Repository{client: client} }

func (r *Repository) ensure(ctx context.Context, name string) error {
	return oops.Wrapf(r.client.Loyalty.Create().
		SetUsername(name).
		SetDiscount(5).
		OnConflictColumns(loyalty.FieldUsername).
		Ignore().
		Exec(ctx), "initialize loyalty")
}

func (r *Repository) Get(ctx context.Context, name string) (_ entities.Loyalty, err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.Get), &err)
	if err = r.ensure(ctx, name); err != nil {
		return entities.Loyalty{}, oops.Wrap(err)
	}
	row, err := r.client.Loyalty.Query().
		Where(loyalty.UsernameEQ(name)).
		Only(ctx)
	if err != nil {
		return entities.Loyalty{}, oops.Wrapf(err, "get loyalty")
	}
	return converters.Loyalty(row), nil
}

func (r *Repository) Change(
	ctx context.Context, command entities.LoyaltyChange,
) (_ entities.Loyalty, err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.Change), &err)
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return entities.Loyalty{}, oops.Wrapf(err, "begin loyalty change")
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, oops.Wrapf(rollbackErr, "rollback loyalty change"))
		}
	}()
	if err = tx.Loyalty.Create().SetUsername(command.User).SetDiscount(5).
		OnConflictColumns(loyalty.FieldUsername).Ignore().Exec(ctx); err != nil {
		return entities.Loyalty{}, oops.Wrapf(err, "initialize loyalty")
	}
	row, err := tx.Loyalty.Query().
		Where(loyalty.UsernameEQ(command.User)).
		ForUpdate().
		Only(ctx)
	if err != nil {
		return entities.Loyalty{}, oops.Wrapf(err, "lock loyalty")
	}

	var related *entities.ChangeRecord
	if key := command.RelatedKey(); key != "" {
		record, queryErr := tx.Operation.Query().Where(operation.OperationIDEQ(key)).Only(ctx)
		if queryErr != nil && !ent.IsNotFound(queryErr) {
			return entities.Loyalty{}, oops.Wrapf(queryErr, "read related loyalty change")
		}
		related = converters.ChangeRecord(record)
	}
	var applied *entities.ChangeRecord
	if command.Key != "" {
		// A unique insert serializes concurrent claims, even across different users.
		_, claimErr := tx.Operation.Create().SetOperationID(command.Key).SetUsername(command.User).SetDelta(command.Delta).
			OnConflictColumns(operation.FieldOperationID).DoNothing().ID(ctx)
		if claimErr != nil && !errors.Is(claimErr, sql.ErrNoRows) {
			return entities.Loyalty{}, oops.Wrapf(claimErr, "claim loyalty operation")
		}
		if errors.Is(claimErr, sql.ErrNoRows) {
			record, queryErr := tx.Operation.Query().Where(operation.OperationIDEQ(command.Key)).Only(ctx)
			if queryErr != nil {
				return entities.Loyalty{}, oops.Wrapf(queryErr, "get loyalty operation")
			}
			applied = converters.ChangeRecord(record)
		}
	}

	item, err := command.Decide(converters.Loyalty(row), applied, related)
	if err != nil {
		return entities.Loyalty{}, oops.Wrap(err)
	}
	if applied != nil {
		if err = tx.Commit(); err != nil {
			return entities.Loyalty{}, oops.Wrapf(err, "commit repeated operation")
		}
		return item, nil
	}

	err = tx.Loyalty.UpdateOneID(row.ID).
		SetReservationCount(item.ReservationCount).
		SetStatus(loyalty.Status(item.Status)).
		SetDiscount(item.Discount).
		Exec(ctx)
	if err != nil {
		return entities.Loyalty{}, oops.Wrapf(err, "update loyalty")
	}
	if err = tx.Commit(); err != nil {
		return entities.Loyalty{}, oops.Wrapf(err, "commit loyalty change")
	}
	return item, nil
}

// ResolveCreation serializes with Change: a committed increment wins, otherwise late creation is rejected.
func (r *Repository) ResolveCreation(
	ctx context.Context,
	command entities.CreationResolution,
) (committed bool, err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.ResolveCreation), &err)
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return false, oops.Wrapf(err, "begin creation resolution")
	}
	defer func() {
		if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
			err = oops.Wrap(errors.Join(err, e))
		}
	}()
	if err = tx.Loyalty.Create().SetUsername(command.User).SetDiscount(5).
		OnConflictColumns(loyalty.FieldUsername).Ignore().Exec(ctx); err != nil {
		return false, oops.Wrapf(err, "initialize loyalty")
	}
	if _, err = tx.Loyalty.Query().Where(loyalty.UsernameEQ(command.User)).ForUpdate().Only(ctx); err != nil {
		return false, oops.Wrapf(err, "lock creation resolution")
	}
	applied, queryErr := tx.Operation.Query().Where(operation.OperationIDEQ(command.CreationKey())).Only(ctx)
	if queryErr != nil && !ent.IsNotFound(queryErr) {
		return false, oops.Wrapf(queryErr, "find creation decision")
	}
	committed, err = command.Decide(converters.ChangeRecord(applied))
	if err != nil {
		return false, oops.Wrap(err)
	}
	if !committed {
		_, err = tx.Operation.Create().SetOperationID(command.RollbackKey()).SetUsername(command.User).SetDelta(-1).
			OnConflictColumns(operation.FieldOperationID).DoNothing().ID(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, oops.Wrapf(err, "abort late creation")
		}
		guard, queryErr := tx.Operation.Query().Where(operation.OperationIDEQ(command.RollbackKey())).Only(ctx)
		if queryErr != nil {
			return false, oops.Wrapf(queryErr, "read creation abort")
		}
		if err = converters.ChangeRecord(guard).Validate(command.User, -1); err != nil {
			return false, oops.Wrap(err)
		}
	}

	if err = tx.Commit(); err != nil {
		return false, oops.Wrapf(err, "commit creation decision")
	}
	return committed, nil
}
