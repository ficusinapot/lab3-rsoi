package repos

import (
	"context"
	"database/sql"
	"errors"
	"uuid"

	"github.com/samber/oops"
	"lab2/payment/ent"
	"lab2/payment/ent/payment"
	"lab2/payment/ent/paymentrequest"
	"lab2/payment/internal/models/entities"
	"lab2/payment/internal/postgres/converters"
	"lab2/platform/observability/metrics"
)

type Repository struct{ client *ent.Client }

func New(client *ent.Client) *Repository { return &Repository{client: client} }

func (r *Repository) Create(ctx context.Context, id uuid.UUID, price int) (_ entities.Payment, err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.Create), &err)
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return entities.Payment{}, oops.Wrapf(err, "begin payment creation")
	}
	defer func() {
		if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
			err = oops.Wrap(errors.Join(err, e))
		}
	}()
	if err = tx.PaymentRequest.Create().SetOperationID(id.String()).OnConflictColumns(paymentrequest.FieldOperationID).
		DoNothing().Exec(ctx); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return entities.Payment{}, oops.Wrapf(err, "claim payment request")
	}
	guard, err := tx.PaymentRequest.Query().Where(paymentrequest.OperationIDEQ(id.String())).ForUpdate().Only(ctx)
	if err != nil {
		return entities.Payment{}, oops.Wrapf(err, "lock payment request")
	}
	if guard.Canceled {
		return entities.Payment{}, oops.Wrap(entities.ErrInvalidInput)
	}
	row, err := tx.Payment.Query().Where(payment.PaymentUIDEQ(id)).Only(ctx)
	if ent.IsNotFound(err) {
		row, err = tx.Payment.Create().SetPaymentUID(id).SetStatus(payment.StatusPAID).SetPrice(price).Save(ctx)
	} else if err == nil && (row.Price != price || row.Status != payment.StatusPAID) {
		return entities.Payment{}, oops.Wrap(entities.ErrInvalidInput)
	}
	if err != nil {
		return entities.Payment{}, oops.Wrapf(err, "create payment")
	}
	if err = tx.Commit(); err != nil {
		return entities.Payment{}, oops.Wrapf(err, "commit payment creation")
	}
	return converters.Payment(row), nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (_ entities.Payment, err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.Get), &err)
	row, err := r.client.Payment.Query().Where(payment.PaymentUIDEQ(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return entities.Payment{}, oops.Wrap(entities.ErrNotFound)
	}
	if err != nil {
		return entities.Payment{}, oops.Wrapf(err, "get payment")
	}
	return converters.Payment(row), nil
}

func (r *Repository) Cancel(ctx context.Context, id uuid.UUID) (err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.Cancel), &err)
	// Legacy DELETE still reports 404. The guarded transaction handles a concurrent POST.
	if _, err = r.Get(ctx, id); err != nil {
		return err
	}
	return r.CancelKnown(ctx, id)
}

func (r *Repository) CancelKnown(ctx context.Context, id uuid.UUID) (err error) {
	defer metrics.Instrument(metrics.PreInstrument(ctx, r.CancelKnown), &err)
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return oops.Wrapf(err, "begin payment cancellation")
	}
	defer func() {
		if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
			err = oops.Wrap(errors.Join(err, e))
		}
	}()
	err = tx.PaymentRequest.Create().SetOperationID(id.String()).SetCanceled(true).
		OnConflictColumns(paymentrequest.FieldOperationID).DoNothing().Exec(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return oops.Wrapf(err, "claim payment cancellation")
	}
	guard, err := tx.PaymentRequest.Query().Where(paymentrequest.OperationIDEQ(id.String())).ForUpdate().Only(ctx)
	if err != nil {
		return oops.Wrapf(err, "lock payment cancellation")
	}
	if err = tx.PaymentRequest.UpdateOneID(guard.ID).SetCanceled(true).Exec(ctx); err != nil {
		return oops.Wrapf(err, "cancel payment request")
	}
	_, err = tx.Payment.Update().Where(payment.PaymentUIDEQ(id)).SetStatus(payment.StatusCANCELED).Save(ctx)
	if err != nil {
		return oops.Wrapf(err, "cancel payment")
	}
	return oops.Wrapf(tx.Commit(), "commit payment cancellation")
}
