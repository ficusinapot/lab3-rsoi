package bookings

import (
	"context"
	"strings"
	"uuid"

	"lab2/gateway/internal/models/entities"

	"github.com/samber/oops"
)

func (s *Queries) Hotels(ctx context.Context, query entities.PageQuery) (entities.Page, error) {
	zeroPage := false
	if query.Page != nil {
		if *query.Page < 0 {
			return entities.Page{}, oops.Wrap(entities.ErrInvalidInput)
		}
		zeroPage = *query.Page == 0
		if zeroPage {
			page := 1
			query.Page = &page
		}
	}
	if query.Size != nil && (*query.Size < 1 || *query.Size > 100) {
		return entities.Page{}, oops.Wrap(entities.ErrInvalidInput)
	}
	item, err := s.reservations.Hotels(ctx, query)
	if err != nil {
		return entities.Page{}, oops.Wrapf(err, "list hotels")
	}
	if zeroPage {
		item.Page = 0
	}
	return item, nil
}

func (s *Queries) Loyalty(ctx context.Context, user string) (entities.Loyalty, error) {
	if err := validateUser(user); err != nil {
		return entities.Loyalty{}, err
	}
	item, err := s.loyalty.Loyalty(ctx, user)
	return item, oops.Wrapf(err, "get loyalty")
}

func (s *Queries) reservationWithHotel(
	ctx context.Context, item entities.InternalReservation, hotel entities.Hotel,
) (entities.Reservation, error) {
	payment, err := s.payments.Payment(ctx, item.PaymentUID)
	if err != nil && !entities.Unavailable(ctx, err) {
		return entities.Reservation{}, oops.Wrapf(err, "get payment")
	}
	if err != nil {
		payment = entities.Payment{}
	}
	return entities.Reservation{
		ReservationUID: item.ReservationUID,
		StartDate:      item.StartDate,
		EndDate:        item.EndDate,
		Status:         item.Status,
		Hotel: entities.HotelInfo{
			HotelUID:    hotel.HotelUID,
			Name:        hotel.Name,
			Stars:       hotel.Stars,
			FullAddress: strings.Join([]string{hotel.Country, hotel.City, hotel.Address}, ", "),
		},
		Payment: payment,
	}, nil
}

func (s *Queries) Get(ctx context.Context, id uuid.UUID, user string) (entities.Reservation, error) {
	if err := validateUser(user); err != nil {
		return entities.Reservation{}, err
	}
	row, err := s.reservations.Reservation(ctx, id, user)
	if err != nil {
		return entities.Reservation{}, oops.Wrapf(err, "get reservation")
	}
	hotel, err := s.reservations.Hotel(ctx, row.HotelUID)
	if err != nil {
		return entities.Reservation{}, oops.Wrapf(err, "get hotel")
	}
	return s.reservationWithHotel(ctx, row, hotel)
}

func (s *Queries) Me(ctx context.Context, user string) (entities.UserInfo, error) {
	items, err := s.List(ctx, user)
	if err != nil {
		return entities.UserInfo{}, err
	}
	loyalty, err := s.Loyalty(ctx, user)
	if err != nil && !entities.Unavailable(ctx, err) {
		return entities.UserInfo{}, err
	}
	if err != nil {
		loyalty = entities.Loyalty{}
	}
	return entities.UserInfo{Reservations: items, Loyalty: loyalty}, nil
}
