//go:build e2e

package fixtures

import (
	"context"
	"database/sql"
	"testing"
	"uuid"

	_ "github.com/jackc/pgx/v5/stdlib"
	"lab2/reservation/ent"
	"lab2/reservation/internal/postgres"
)

const TestHotelUID = "049161bb-badd-4fa8-9d90-87c9a82b0668"

type Hotel struct {
	UID     uuid.UUID
	Name    string
	Country string
	City    string
	Address string
	Stars   int
	Price   int
}

var TestHotel = Hotel{
	UID:     uuid.MustParse(TestHotelUID),
	Name:    "Ararat Park Hyatt Moscow",
	Country: "Россия",
	City:    "Москва",
	Address: "Неглинная ул., 4",
	Stars:   5,
	Price:   10000,
}

func SeedHotel(t *testing.T, databaseURL string) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	client := postgres.NewClient(db)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	createHotel(t, ctx, client, TestHotel)
}

func createHotel(t *testing.T, ctx context.Context, client *ent.Client, hotel Hotel) {
	t.Helper()
	_, err := client.Hotel.Create().
		SetHotelUID(hotel.UID).
		SetName(hotel.Name).
		SetCountry(hotel.Country).
		SetCity(hotel.City).
		SetAddress(hotel.Address).
		SetStars(hotel.Stars).
		SetPrice(hotel.Price).
		Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
}
