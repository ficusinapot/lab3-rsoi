package bookings

import (
	"context"
	"uuid"

	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/operationifc"
	"lab2/platform/observability/metrics"

	"github.com/samber/oops"
)

func (s *Commands) Create(
	ctx context.Context,
	input entities.CreateBooking,
) (_ entities.Created, err error) {
	user := input.User
	defer metrics.Instrument(metrics.PreInstrument(ctx, s.Create), &err)
	if err = validateUser(user); err != nil {
		return entities.Created{}, err
	}
	if input.HotelUID == uuid.Nil() {
		return entities.Created{}, oops.Wrap(entities.ErrInvalidInput)
	}
	nights, err := bookingNights(input)
	if err != nil {
		return entities.Created{}, err
	}
	var created entities.Created
	err = s.groups.Create(ctx, user, func(creation operationifc.Creation) error {
		hotel, err := s.reservations.Hotel(ctx, input.HotelUID)
		if err != nil {
			return oops.Wrapf(err, "get hotel")
		}
		loyalty, err := s.loyalty.Loyalty(ctx, user)
		if err != nil {
			return oops.Wrapf(err, "get loyalty")
		}
		price, err := cost(nights, hotel.Price, loyalty.Discount)
		if err != nil {
			return oops.Wrap(err)
		}
		payment, err := creation.CreatePayment(ctx, price)
		if err != nil {
			return oops.Wrap(err)
		}
		row, err := creation.CreateReservation(ctx, input)
		if err != nil {
			return oops.Wrap(err)
		}
		if err := creation.IncreaseLoyalty(ctx); err != nil {
			return oops.Wrap(err)
		}
		created = entities.Created{
			ReservationUID: row.ReservationUID,
			HotelUID:       row.HotelUID,
			StartDate:      row.StartDate,
			EndDate:        row.EndDate,
			Status:         row.Status,
			Discount:       loyalty.Discount,
			Payment:        payment.Payment,
		}
		return nil
	})
	return created, oops.Wrap(err)
}
