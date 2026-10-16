//go:build integration

package bookings_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"

	"lab2/reservation/ent/hotel"
	"lab2/reservation/ent/reservation"
	"lab2/reservation/internal/models/entities"
	bookingrepo "lab2/reservation/internal/postgres/repos/bookings"
	"lab2/reservation/tests/integration/infrastructure"
)

func TestBookingRepository(t *testing.T) {
	t.Parallel()
	db := infrastructure.NewDatabase(t)
	hotelUID := uuid.New()
	reservationUID := uuid.New()
	_, err := db.Client.Hotel.Create().
		SetHotelUID(hotelUID).SetName("Repository hotel").SetCountry("Russia").
		SetCity("Moscow").SetAddress("Street").SetPrice(1000).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := db.Client.Reservation.Delete().
			Where(reservation.ReservationUIDEQ(reservationUID)).Exec(ctx); err != nil {
			t.Error(err)
		}
		if _, err := db.Client.Hotel.Delete().Where(hotel.HotelUIDEQ(hotelUID)).Exec(ctx); err != nil {
			t.Error(err)
		}
	})

	username := strings.Repeat("я", 44) + reservationUID.String()
	repo := bookingrepo.New(db.Client)
	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	created, err := repo.Create(t.Context(), entities.Booking{
		ReservationUID: reservationUID, HotelUID: hotelUID, PaymentUID: uuid.New(),
		Username: username, StartDate: start, EndDate: start.AddDate(0, 0, 2),
	})
	if err != nil || created.ReservationUID != reservationUID || created.Status != entities.StatusPaid {
		t.Fatalf("create: %+v %v", created, err)
	}
	items, err := repo.List(t.Context(), username)
	if err != nil || len(items) != 1 || items[0] != created {
		t.Fatalf("list: %+v %v", items, err)
	}
	if _, err := repo.Get(t.Context(), reservationUID, "other-user"); !errors.Is(err, entities.ErrNotFound) {
		t.Fatalf("ownership: %v", err)
	}
	if err := repo.Cancel(t.Context(), reservationUID, username); err != nil {
		t.Fatal(err)
	}
	after, err := repo.Get(t.Context(), reservationUID, username)
	if err != nil || after.Status != entities.StatusCanceled {
		t.Fatalf("cancel: %+v %v", after, err)
	}
}
