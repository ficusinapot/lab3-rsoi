package bookings

import (
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"

	model "lab2/gateway/internal/models/entities"

	"go.uber.org/mock/gomock"
)

const testUser = "user"

func TestCreate(t *testing.T) {
	t.Parallel()
	hotelID, paymentID, reservationID := uuid.New(), uuid.New(), uuid.New()
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 3)
	input := model.CreateBooking{HotelUID: hotelID, StartDate: start, EndDate: end}
	payment := model.InternalPayment{
		PaymentUID: paymentID,
		Status:     model.PaymentPaid, Price: 281,
	}
	reservation := model.InternalReservation{
		ReservationUID: reservationID,
		HotelUID:       hotelID,
		PaymentUID:     paymentID,
		StartDate:      start,
		EndDate:        end,
		Status:         model.ReservationPaid,
	}
	ctrl := gomock.NewController(t)
	reservations := NewMockReservations(ctrl)
	payments := NewMockPayments(ctrl)
	loyalty := NewMockLoyalty(ctrl)
	gomock.InOrder(
		reservations.EXPECT().Hotel(gomock.Any(), hotelID).Return(model.Hotel{Price: 101}, nil),
		loyalty.EXPECT().Loyalty(gomock.Any(), testUser).Return(model.Loyalty{Discount: 7}, nil),
		payments.EXPECT().CreatePayment(gomock.Any(), paymentCreateMatcher(281)).Return(payment, nil),
		reservations.EXPECT().CreateReservation(gomock.Any(), testUser, reservationInputMatcher(model.InternalCreate{
			HotelUID:   hotelID,
			PaymentUID: paymentID,
			StartDate:  start,
			EndDate:    end,
		})).Return(reservation, nil),
		loyalty.EXPECT().ChangeLoyalty(gomock.Any(), testUser, 1, gomock.Any()).Return(nil),
	)
	got, err := newTestCommands(reservations, payments, loyalty).Create(t.Context(), model.CreateBooking{
		User: testUser, HotelUID: input.HotelUID, StartDate: input.StartDate, EndDate: input.EndDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := model.Created{
		ReservationUID: reservationID,
		HotelUID:       hotelID,
		StartDate:      start,
		EndDate:        end,
		Status:         model.ReservationPaid,
		Discount:       7,
		Payment:        payment.Payment,
	}
	if got != want {
		t.Fatalf("created reservation = %+v, want %+v", got, want)
	}
}

func TestCreateStopsAtFailure(t *testing.T) {
	t.Parallel()
	hotelID, paymentID, reservationID := uuid.New(), uuid.New(), uuid.New()
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 3)
	cause := errors.New("dependency unavailable")
	for _, tc := range []struct {
		name   string
		expect func(*MockReservations, *MockPayments, *MockLoyalty)
	}{
		{
			name: "hotel unavailable",
			expect: func(reservations *MockReservations, payments *MockPayments, loyalty *MockLoyalty) {
				gomock.InOrder(
					reservations.EXPECT().Hotel(gomock.Any(), hotelID).Return(model.Hotel{Price: 10000}, cause),
				)
			},
		},
		{
			name: "loyalty unavailable",
			expect: func(reservations *MockReservations, payments *MockPayments, loyalty *MockLoyalty) {
				gomock.InOrder(
					reservations.EXPECT().Hotel(gomock.Any(), hotelID).Return(model.Hotel{Price: 10000}, nil),
					loyalty.EXPECT().Loyalty(gomock.Any(), testUser).Return(model.Loyalty{Discount: 5}, cause),
				)
			},
		},
		{
			name: "payment unavailable",
			expect: func(reservations *MockReservations, payments *MockPayments, loyalty *MockLoyalty) {
				gomock.InOrder(
					reservations.EXPECT().Hotel(gomock.Any(), hotelID).Return(model.Hotel{Price: 10000}, nil),
					loyalty.EXPECT().Loyalty(gomock.Any(), testUser).Return(model.Loyalty{Discount: 5}, nil),
					payments.EXPECT().CreatePayment(gomock.Any(), paymentCreateMatcher(28500)).
						Return(model.InternalPayment{PaymentUID: paymentID}, cause),
				)
			},
		},
		{
			name: "reservation unavailable",
			expect: func(reservations *MockReservations, payments *MockPayments, loyalty *MockLoyalty) {
				gomock.InOrder(
					reservations.EXPECT().Hotel(gomock.Any(), hotelID).Return(model.Hotel{Price: 10000}, nil),
					loyalty.EXPECT().Loyalty(gomock.Any(), testUser).Return(model.Loyalty{Discount: 5}, nil),
					payments.EXPECT().CreatePayment(gomock.Any(), paymentCreateMatcher(28500)).
						Return(model.InternalPayment{PaymentUID: paymentID}, nil),
					reservations.EXPECT().CreateReservation(gomock.Any(), testUser, reservationInputMatcher(model.InternalCreate{
						HotelUID: hotelID, PaymentUID: paymentID, StartDate: start, EndDate: end,
					})).Return(model.InternalReservation{}, cause),
					payments.EXPECT().CancelPayment(gomock.Any(), paymentID).Return(nil),
				)
			},
		},
		{
			name: "loyalty increment fails",
			expect: func(reservations *MockReservations, payments *MockPayments, loyalty *MockLoyalty) {
				gomock.InOrder(
					reservations.EXPECT().Hotel(gomock.Any(), hotelID).Return(model.Hotel{Price: 10000}, nil),
					loyalty.EXPECT().Loyalty(gomock.Any(), testUser).Return(model.Loyalty{Discount: 5}, nil),
					payments.EXPECT().CreatePayment(gomock.Any(), paymentCreateMatcher(28500)).
						Return(model.InternalPayment{PaymentUID: paymentID}, nil),
					reservations.EXPECT().CreateReservation(gomock.Any(), testUser, reservationInputMatcher(model.InternalCreate{
						HotelUID: hotelID, PaymentUID: paymentID, StartDate: start, EndDate: end,
					})).Return(model.InternalReservation{ReservationUID: reservationID}, nil),
					loyalty.EXPECT().ChangeLoyalty(gomock.Any(), testUser, 1, reservationID.String()+":create").Return(cause),
					reservations.EXPECT().CancelReservation(gomock.Any(), reservationID, testUser).Return(cause),
					payments.EXPECT().CancelPayment(gomock.Any(), paymentID).Return(nil),
				)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			reservations := NewMockReservations(ctrl)
			payments := NewMockPayments(ctrl)
			loyalty := NewMockLoyalty(ctrl)
			tc.expect(reservations, payments, loyalty)
			// Compensation now also guards UUIDs whose POST responses were never received.
			reservations.EXPECT().CancelReservation(gomock.Any(), gomock.Any(), testUser).AnyTimes().Return(nil)
			payments.EXPECT().CancelPayment(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
			loyalty.EXPECT().ChangeLoyalty(gomock.Any(), testUser, -1, gomock.Any()).AnyTimes().Return(nil)
			got, err := newTestCommands(reservations, payments, loyalty).Create(t.Context(), model.CreateBooking{
				User:     testUser,
				HotelUID: hotelID, StartDate: start, EndDate: end,
			})
			if !errors.Is(err, cause) {
				t.Fatalf("error = %v, want cause %v", err, cause)
			}
			if got != (model.Created{}) {
				t.Fatalf("created reservation = %+v, want zero value", got)
			}
		})
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		user   string
		change func(*model.CreateBooking)
	}{
		{name: "blank user", user: " "},
		{name: "long user", user: strings.Repeat("a", 81)},
		{name: "invalid UTF-8 user", user: "\xff"},
		{name: "missing hotel", user: testUser, change: func(input *model.CreateBooking) {
			input.HotelUID = uuid.Nil()
		}},
		{name: "missing start", user: testUser, change: func(input *model.CreateBooking) {
			input.StartDate = time.Time{}
		}},
		{name: "missing end", user: testUser, change: func(input *model.CreateBooking) {
			input.EndDate = time.Time{}
		}},
		{name: "equal dates", user: testUser, change: func(input *model.CreateBooking) {
			input.EndDate = input.StartDate
		}},
		{name: "reversed dates", user: testUser, change: func(input *model.CreateBooking) {
			input.EndDate = input.StartDate.AddDate(0, 0, -1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
			input := model.CreateBooking{HotelUID: uuid.New(), StartDate: start, EndDate: start.AddDate(0, 0, 3)}
			if tc.change != nil {
				tc.change(&input)
			}
			service := newTestCommands(NewMockReservations(ctrl), NewMockPayments(ctrl), NewMockLoyalty(ctrl))
			_, err := service.Create(t.Context(), model.CreateBooking{
				User: tc.user, HotelUID: input.HotelUID, StartDate: input.StartDate, EndDate: input.EndDate,
			})
			if !errors.Is(err, model.ErrInvalidInput) {
				t.Fatalf("error = %v, want invalid input", err)
			}
		})
	}
}

func TestFullQueueStopsCreationBeforeUpstreamCalls(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	service := newTestCommands(NewMockReservations(ctrl), NewMockPayments(ctrl), NewMockLoyalty(ctrl))
	for range 256 {
		reserved, err := service.stream.ReserveOperation(t.Context(), uuid.New(), model.OperationCompensation)
		if err != nil || !reserved {
			t.Fatalf("reserve: reserved=%t, error=%v", reserved, err)
		}
	}
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	_, err := service.Create(t.Context(), model.CreateBooking{
		User:     testUser,
		HotelUID: uuid.New(), StartDate: start, EndDate: start.AddDate(0, 0, 1),
	})
	if !errors.Is(err, model.ErrQueueFull) {
		t.Fatal(err)
	}
}

type reservationInputMatcher model.InternalCreate

func (m reservationInputMatcher) Matches(value any) bool {
	input, ok := value.(model.InternalCreate)
	if !ok || input.ReservationUID == uuid.Nil() {
		return false
	}
	input.ReservationUID = uuid.Nil()
	return input == model.InternalCreate(m)
}
func (reservationInputMatcher) String() string { return "reservation input with a supplied UUID" }

func paymentCreateMatcher(price int) gomock.Matcher {
	return gomock.Cond(func(input model.PaymentCreate) bool {
		return input.Price == price && input.PaymentUID != uuid.Nil()
	})
}
