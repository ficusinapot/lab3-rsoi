package bookings

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
	"uuid"

	"github.com/samber/oops"

	"lab2/reservation/internal/models/dbifc"
	"lab2/reservation/internal/models/entities"
)

type Service struct{ repo dbifc.Bookings }

func New(repo dbifc.Bookings) *Service { return &Service{repo: repo} }

func validateUser(name string) error {
	if strings.TrimSpace(name) == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 80 {
		return oops.In("validation").
			Code("invalid_input").
			Public("valid username is required").
			Wrap(errors.Join(entities.ErrInvalidInput, entities.ErrInvalidUsername))
	}
	return nil
}

func (s *Service) List(ctx context.Context, name string) ([]entities.Reservation, error) {
	if err := validateUser(name); err != nil {
		return nil, err
	}
	result, err := s.repo.List(ctx, name)
	return result, oops.FromContext(ctx).
		In("bookings.usecase").
		With("operation", "list").
		Wrapf(err, "operation failed")
}

func (s *Service) Get(ctx context.Context, id uuid.UUID, name string) (entities.Reservation, error) {
	if err := validateUser(name); err != nil {
		return entities.Reservation{}, err
	}
	result, err := s.repo.Get(ctx, id, name)
	return result, oops.FromContext(ctx).
		In("bookings.usecase").
		With("operation", "get").
		Wrapf(err, "operation failed")
}

func (s *Service) Create(ctx context.Context, name string, input entities.CreateInput) (entities.Reservation, error) {
	if err := validateUser(name); err != nil {
		return entities.Reservation{}, err
	}
	if input.HotelUID == uuid.Nil() || input.PaymentUID == uuid.Nil() {
		return entities.Reservation{}, oops.In("validation").
			Code("invalid_input").
			Public("valid UUIDs are required").
			Wrap(entities.ErrInvalidInput)
	}
	start, end := input.StartDate, input.EndDate
	if start.IsZero() || !end.After(start) {
		return entities.Reservation{}, oops.In("validation").
			Code("invalid_input").
			Public("valid increasing dates are required").
			Wrap(entities.ErrInvalidInput)
	}
	id := input.ReservationUID
	if id == uuid.Nil() {
		id = uuid.New()
	}
	result, err := s.repo.Create(ctx, entities.Booking{
		ReservationUID: id, HotelUID: input.HotelUID,
		PaymentUID: input.PaymentUID, Username: name, StartDate: start, EndDate: end,
	})
	return result, oops.FromContext(ctx).
		In("bookings.usecase").
		With("operation", "create").
		Wrapf(err, "operation failed")
}

func (s *Service) Cancel(ctx context.Context, id uuid.UUID, name string) error {
	if err := validateUser(name); err != nil {
		return err
	}
	return oops.FromContext(ctx).
		In("bookings.usecase").
		With("operation", "cancel").
		Wrapf(s.repo.Cancel(ctx, id, name), "operation failed")
}

func (s *Service) CancelKnown(ctx context.Context, id uuid.UUID, name string) error {
	if err := validateUser(name); err != nil {
		return err
	}
	if id == uuid.Nil() {
		return oops.Wrap(entities.ErrInvalidInput)
	}
	return oops.Wrapf(s.repo.CancelKnown(ctx, id, name), "cancel known reservation")
}
