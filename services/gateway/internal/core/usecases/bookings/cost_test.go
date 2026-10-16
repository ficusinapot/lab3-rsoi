package bookings

import (
	"errors"
	"math"
	"testing"
	"time"

	model "lab2/gateway/internal/models/entities"
)

func TestCost(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		nights    int64
		price     int
		discount  int
		wantPrice int
		wantErr   error
	}{
		{name: "bronze", nights: 3, price: 10000, discount: 5, wantPrice: 28500},
		{name: "silver", nights: 3, price: 10000, discount: 7, wantPrice: 27900},
		{name: "gold", nights: 3, price: 10000, discount: 10, wantPrice: 27000},
		{name: "round down", nights: 3, price: 101, discount: 7, wantPrice: 281},
		{name: "free hotel", nights: 3, discount: 5},
		{name: "no discount", nights: 1, price: 100, wantPrice: 100},
		{name: "full discount", nights: 1, price: 100, discount: 100},
		{name: "zero nights", price: 100, discount: 5, wantErr: model.ErrInvalidInput},
		{name: "negative nights", nights: -1, price: 100, discount: 5, wantErr: model.ErrInvalidInput},
		{name: "negative price", nights: 1, price: -1, discount: 5, wantErr: model.ErrInvalidInput},
		{name: "negative discount", nights: 1, price: 100, discount: -1, wantErr: model.ErrInvalidInput},
		{name: "discount above 100", nights: 1, price: 100, discount: 101, wantErr: model.ErrInvalidInput},
		{name: "base overflows", nights: math.MaxInt64, price: 2, discount: 5, wantErr: model.ErrInvalidInput},
		{
			name: "discount product overflows", nights: math.MaxInt64/100 + 1, price: 1,
			wantErr: model.ErrInvalidInput,
		},
		{
			name: "storage price overflows", nights: 2, price: math.MaxInt32, discount: 5,
			wantErr: model.ErrInvalidInput,
		},
		{name: "maximum storage price", nights: 1, price: math.MaxInt32, wantPrice: math.MaxInt32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := cost(tc.nights, tc.price, tc.discount)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if got != tc.wantPrice {
				t.Fatalf("price = %d, want %d", got, tc.wantPrice)
			}
		})
	}
}

func TestBookingNights(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		start, end time.Time
		wantNights int64
		wantErr    error
	}{
		{name: "three nights", start: start, end: start.AddDate(0, 0, 3), wantNights: 3},
		{name: "zero start", end: start, wantErr: model.ErrInvalidInput},
		{name: "zero end", start: start, wantErr: model.ErrInvalidInput},
		{name: "equal dates", start: start, end: start, wantErr: model.ErrInvalidInput},
		{name: "reversed dates", start: start, end: start.AddDate(0, 0, -1), wantErr: model.ErrInvalidInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := bookingNights(model.CreateBooking{StartDate: tc.start, EndDate: tc.end})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if got != tc.wantNights {
				t.Fatalf("nights = %d, want %d", got, tc.wantNights)
			}
		})
	}
}
