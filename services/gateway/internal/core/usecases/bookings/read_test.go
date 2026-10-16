package bookings

import (
	"errors"
	"reflect"
	"testing"
	"time"
	"uuid"

	"go.uber.org/mock/gomock"
	model "lab2/gateway/internal/models/entities"
)

const (
	listOperation    = "list"
	loyaltyOperation = "loyalty"
	paymentOperation = "payment"
)

func TestGet(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		failure string
	}{
		{name: "reservation with hotel and payment"},
		{name: "reservation unavailable", failure: "reservation"},
		{name: "hotel unavailable", failure: "hotel"},
		{name: "payment unavailable", failure: paymentOperation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			reservations := NewMockReservationReader(ctrl)
			payments := NewMockPaymentReader(ctrl)
			cause := model.ErrUpstreamUnavailable
			row := readRows(1)[0]
			row.StartDate = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
			row.EndDate = row.StartDate.AddDate(0, 0, 3)
			hotel := model.Hotel{
				HotelUID: row.HotelUID, Name: "Hotel", Country: "Россия", City: "Москва",
				Address: "Неглинная ул., 4", Stars: new(5), Price: 10000,
			}
			payment := model.Payment{Status: model.PaymentPaid, Price: 28500}
			var reservationErr, hotelErr, paymentErr error
			switch tc.failure {
			case "reservation":
				reservationErr = cause
			case "hotel":
				hotelErr = cause
			case paymentOperation:
				paymentErr = cause
			}
			reservations.EXPECT().Reservation(gomock.Any(), row.ReservationUID, testUser).Return(row, reservationErr)
			if reservationErr == nil {
				reservations.EXPECT().Hotel(gomock.Any(), row.HotelUID).Return(hotel, hotelErr)
				if hotelErr == nil {
					payments.EXPECT().Payment(gomock.Any(), row.PaymentUID).Return(payment, paymentErr)
				}
			}
			got, err := NewQueries(reservations, payments, nil, 4).Get(t.Context(), row.ReservationUID, testUser)
			if tc.failure != "" && tc.failure != paymentOperation {
				if !errors.Is(err, cause) {
					t.Fatalf("error = %v, want cause %v", err, cause)
				}
				if !reflect.DeepEqual(got, model.Reservation{}) {
					t.Errorf("reservation = %+v, want zero value", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := model.Reservation{
				ReservationUID: row.ReservationUID,
				StartDate:      row.StartDate,
				EndDate:        row.EndDate,
				Status:         row.Status,
				Hotel: model.HotelInfo{
					HotelUID: row.HotelUID, Name: hotel.Name, Stars: new(5),
					FullAddress: "Россия, Москва, Неглинная ул., 4",
				},
				Payment: payment,
			}
			if tc.failure == paymentOperation {
				want.Payment = model.Payment{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("reservation = %+v, want %+v", got, want)
			}
		})
	}
}

func TestMe(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		failure string
	}{
		{name: "new user"},
		{name: "reservation list unavailable", failure: listOperation},
		{name: "loyalty unavailable", failure: loyaltyOperation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			reservations := NewMockReservationReader(ctrl)
			loyalty := NewMockLoyaltyReader(ctrl)
			cause := model.ErrUpstreamUnavailable
			var listErr, loyaltyErr error
			switch tc.failure {
			case listOperation:
				listErr = cause
			case loyaltyOperation:
				loyaltyErr = cause
			}
			listing := reservations.EXPECT().Reservations(gomock.Any(), testUser).Return(nil, listErr)
			wantLoyalty := model.Loyalty{Status: model.LoyaltyBronze, Discount: 5}
			if listErr == nil {
				loyalty.EXPECT().Loyalty(gomock.Any(), testUser).Return(wantLoyalty, loyaltyErr).After(listing)
			}
			got, err := NewQueries(reservations, NewMockPaymentReader(ctrl), loyalty, 4).Me(t.Context(), testUser)
			if tc.failure != "" && tc.failure != loyaltyOperation {
				if !errors.Is(err, cause) {
					t.Fatalf("error = %v, want cause %v", err, cause)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Reservations == nil || len(got.Reservations) != 0 {
				t.Errorf("reservations = %v, want nonnil empty slice", got.Reservations)
			}
			if tc.failure == loyaltyOperation {
				wantLoyalty = model.Loyalty{}
			}
			if got.Loyalty != wantLoyalty {
				t.Errorf("loyalty = %+v, want %+v", got.Loyalty, wantLoyalty)
			}
		})
	}
}

func TestReadRejectsBlankUser(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		call func(*Queries) error
	}{
		{name: listOperation, call: func(service *Queries) error { _, err := service.List(t.Context(), " "); return err }},
		{name: "get", call: func(service *Queries) error {
			_, err := service.Get(t.Context(), uuid.New(), " ")
			return err
		}},
		{name: "me", call: func(service *Queries) error { _, err := service.Me(t.Context(), " "); return err }},
		{name: loyaltyOperation, call: func(service *Queries) error {
			_, err := service.Loyalty(t.Context(), " ")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			service := NewQueries(NewMockReservationReader(ctrl), NewMockPaymentReader(ctrl), NewMockLoyaltyReader(ctrl), 4)
			if err := tc.call(service); !errors.Is(err, model.ErrInvalidInput) {
				t.Fatalf("error = %v, want invalid input", err)
			}
		})
	}
}
