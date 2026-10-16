//go:build integration

package hotels_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"lab2/reservation/ent/hotel"
	"lab2/reservation/internal/models/entities"
	hotelrepo "lab2/reservation/internal/postgres/repos/hotels"
	"lab2/reservation/tests/integration/infrastructure"
)

func TestHotelRepository(t *testing.T) {
	t.Parallel()
	db := infrastructure.NewDatabase(t)
	id := uuid.New()
	_, err := db.Client.Hotel.Create().
		SetHotelUID(id).SetName("Repository hotel").SetCountry("Russia").
		SetCity("Moscow").SetAddress("Street").SetPrice(1000).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := db.Client.Hotel.Delete().Where(hotel.HotelUIDEQ(id)).Exec(ctx); err != nil {
			t.Error(err)
		}
	})
	repo := hotelrepo.New(db.Client)
	got, err := repo.Get(t.Context(), id)
	if err != nil || got.HotelUID != id || got.Price != 1000 {
		t.Fatalf("get: %+v %v", got, err)
	}
	page, err := repo.List(t.Context(), 1, 100)
	if err != nil || page.TotalElements < 1 || page.Page != 1 {
		t.Fatalf("list: %+v %v", page, err)
	}
	found := false
	for _, item := range page.Items {
		found = found || item.HotelUID == id
	}
	if !found {
		t.Fatalf("hotel not found in page: %+v", page)
	}
	if _, err := repo.Get(t.Context(), uuid.New()); !errors.Is(err, entities.ErrNotFound) {
		t.Fatalf("missing hotel: %v", err)
	}
}

func TestListReleasesTransaction(t *testing.T) {
	t.Parallel()
	db := infrastructure.NewDatabase(t)
	db.SQL.SetMaxOpenConns(1)
	repo := hotelrepo.New(db.Client)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	if _, err := repo.List(ctx, 0, 10); err == nil {
		t.Fatal("List with negative offset succeeded")
	}
	if _, err := repo.List(ctx, 1, 10); err != nil {
		t.Fatalf("List after rollback: %v", err)
	}
	if inUse := db.SQL.Stats().InUse; inUse != 0 {
		t.Fatalf("connections in use after commit = %d, want 0", inUse)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := repo.List(canceled, 1, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("List error = %v, want context.Canceled", err)
	}
	if _, err := repo.List(ctx, 1, 10); err != nil {
		t.Fatalf("List after cancellation: %v", err)
	}
}
