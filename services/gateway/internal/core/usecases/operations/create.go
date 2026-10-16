package operations

import (
	"context"
	"runtime/debug"
	"uuid"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/operationifc"
)

// Create owns the reservation and compensation lifecycle around the booking steps.
func (g *Groups) Create(ctx context.Context, user string, run func(operationifc.Creation) error) (err error) {
	creation := &creation{
		groups:    g,
		operation: entities.NewCreationOperation(user, uuid.New(), uuid.New()),
	}
	creation.task = creation.operation.Group()
	if err = g.reserve(ctx, creation.task); err != nil {
		return oops.Wrap(err)
	}
	defer func() {
		if value := recover(); value != nil {
			err = entities.NewOperationPanic(value, debug.Stack())
		}
		g.finish(ctx, &creation.task, &err)
	}()
	return run(creation)
}

type creation struct {
	groups    *Groups
	operation entities.CreationOperation
	task      entities.Operation
}

func (c *creation) CreatePayment(ctx context.Context, price int) (entities.InternalPayment, error) {
	if err := c.groups.prepare(ctx, &c.task, c.operation.Compensation()); err != nil {
		return entities.InternalPayment{}, oops.Wrap(err)
	}
	payment, err := c.groups.payments.CreatePayment(ctx, entities.PaymentCreate{
		PaymentUID: c.operation.PaymentUID, Price: price,
	})
	if err != nil {
		return entities.InternalPayment{}, oops.Wrapf(err, "create payment")
	}
	c.operation.PaymentUID = payment.PaymentUID
	c.task = c.operation.Compensation()
	return payment, nil
}

func (c *creation) CreateReservation(
	ctx context.Context, input entities.CreateBooking,
) (entities.InternalReservation, error) {
	row, err := c.groups.reservations.CreateReservation(ctx, c.operation.User, entities.InternalCreate{
		ReservationUID: c.operation.ReservationUID,
		HotelUID:       input.HotelUID,
		StartDate:      input.StartDate,
		EndDate:        input.EndDate,
		PaymentUID:     c.operation.PaymentUID,
	})
	if err != nil {
		return entities.InternalReservation{}, oops.Wrapf(err, "create reservation")
	}
	c.operation.ReservationUID = row.ReservationUID
	if err := c.groups.prepare(ctx, &c.task, c.operation.ResolveCompensation()); err != nil {
		return entities.InternalReservation{}, oops.Wrap(err)
	}
	return row, nil
}

func (c *creation) IncreaseLoyalty(ctx context.Context) error {
	return oops.Wrapf(c.groups.loyalty.ChangeLoyalty(
		ctx, c.operation.User, 1, c.operation.ReservationUID.String()+":create",
	), "increase loyalty")
}
