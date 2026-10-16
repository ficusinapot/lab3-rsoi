package bookings

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"

	"go.uber.org/mock/gomock"
	bookings "lab2/reservation/internal/models/entities"
)

const (
	bookingID    = "049161bb-badd-4fa8-9d90-87c9a82b0668"
	bookingStart = "2021-10-08"
	bookingEnd   = "2021-10-11"
	testUser     = "user"
)

func TestServiceOwnership(t *testing.T) {
	t.Parallel()
	const username = " Test Max "
	cause := bookings.ErrNotFound
	ctrl := gomock.NewController(t)
	repo := NewMockBookings(ctrl)
	repo.EXPECT().List(gomock.Any(), username).
		Return([]bookings.Reservation{{ReservationUID: uuid.MustParse(bookingID)}}, nil)
	repo.EXPECT().Get(gomock.Any(), uuid.MustParse(bookingID), username).
		Return(bookings.Reservation{}, cause)
	repo.EXPECT().Cancel(gomock.Any(), uuid.MustParse(bookingID), username).
		Return(cause)
	s := New(repo)
	if result, err := s.List(t.Context(), username); err != nil || len(result) != 1 {
		t.Fatalf("list=%v err=%v", result, err)
	}
	if _, err := s.Get(t.Context(), uuid.MustParse(bookingID), username); !errors.Is(err, cause) {
		t.Fatalf("get error=%v", err)
	}
	if err := s.Cancel(t.Context(), uuid.MustParse(bookingID), username); !errors.Is(err, cause) {
		t.Fatalf("cancel error=%v", err)
	}
}

func TestServiceCreate(t *testing.T) {
	t.Parallel()
	cause := errors.New("write failed")
	for _, tc := range []struct {
		name          string
		repositoryErr error
	}{
		{name: "successful booking"},
		{name: "repository failure", repositoryErr: cause},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := bookings.CreateInput{
				HotelUID:   uuid.MustParse(bookingID),
				PaymentUID: uuid.New(),
				StartDate:  mustDate(bookingStart),
				EndDate:    mustDate(bookingEnd),
			}
			repo := NewMockBookings(gomock.NewController(t))
			var want bookings.Reservation
			repo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, booking bookings.Booking) (bookings.Reservation, error) {
					if booking.ReservationUID == uuid.Nil() {
						t.Error("reservation UID was not generated")
					}
					wantBooking := bookings.Booking{
						ReservationUID: booking.ReservationUID,
						HotelUID:       input.HotelUID,
						PaymentUID:     input.PaymentUID,
						Username:       "Test Max",
						StartDate:      input.StartDate,
						EndDate:        input.EndDate,
					}
					if booking != wantBooking {
						t.Errorf("booking = %+v, want %+v", booking, wantBooking)
					}
					if tc.repositoryErr != nil {
						return bookings.Reservation{}, tc.repositoryErr
					}
					want = bookings.Reservation{
						ReservationUID: booking.ReservationUID,
						HotelUID:       booking.HotelUID,
						PaymentUID:     booking.PaymentUID,
						Status:         bookings.StatusPaid,
						StartDate:      booking.StartDate,
						EndDate:        booking.EndDate,
					}
					return want, nil
				},
			)
			got, err := New(repo).Create(t.Context(), "Test Max", input)
			if !errors.Is(err, tc.repositoryErr) {
				t.Fatalf("Create error = %v, want %v", err, tc.repositoryErr)
			}
			if got != want {
				t.Fatalf("reservation = %+v, want %+v", got, want)
			}
		})
	}
}

func TestServiceValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		username   string
		invalidate func(*bookings.CreateInput)
	}{
		{name: "empty user", username: " "},
		{name: "long user", username: strings.Repeat("a", 81)},
		{name: "invalid UTF-8 user", username: "\xff"},
		{
			name: "invalid hotel", username: testUser,
			invalidate: func(input *bookings.CreateInput) { input.HotelUID = uuid.Nil() },
		},
		{
			name: "invalid payment", username: testUser,
			invalidate: func(input *bookings.CreateInput) { input.PaymentUID = uuid.Nil() },
		},
		{
			name: "equal dates", username: testUser,
			invalidate: func(input *bookings.CreateInput) { input.EndDate = input.StartDate },
		},
		{
			name: "end before start", username: testUser,
			invalidate: func(input *bookings.CreateInput) { input.EndDate = input.StartDate.AddDate(0, 0, -1) },
		},
		{
			name: "zero start", username: testUser,
			invalidate: func(input *bookings.CreateInput) { input.StartDate = time.Time{} },
		},
		{
			name: "zero end", username: testUser,
			invalidate: func(input *bookings.CreateInput) { input.EndDate = time.Time{} },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := bookings.CreateInput{
				HotelUID:   uuid.MustParse(bookingID),
				PaymentUID: uuid.MustParse(bookingID),
				StartDate:  mustDate(bookingStart),
				EndDate:    mustDate(bookingEnd),
			}
			if tc.invalidate != nil {
				tc.invalidate(&input)
			}
			repo := NewMockBookings(gomock.NewController(t))
			_, err := New(repo).Create(t.Context(), tc.username, input)
			if !errors.Is(err, bookings.ErrInvalidInput) {
				t.Fatalf("Create error = %v, want ErrInvalidInput", err)
			}
		})
	}
	ctrl := gomock.NewController(t)
	s := New(NewMockBookings(ctrl))
	if _, err := s.List(t.Context(), ""); !errors.Is(err, bookings.ErrInvalidUsername) {
		t.Fatalf("list validation=%v", err)
	}
}

func mustDate(value string) time.Time {
	date, err := time.Parse(time.DateOnly, value)
	if err != nil {
		panic(err)
	}
	return date
}
