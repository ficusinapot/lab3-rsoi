//go:build integration

package fixtures

import (
	"context"
	"testing"
	"uuid"

	"lab2/reservation/ent"
)

const TestHotelUID = "049161bb-badd-4fa8-9d90-87c9a82b0668"

func SeedHotel(t *testing.T, ctx context.Context, client *ent.Client) *ent.Hotel {
	t.Helper()
	hotelUID := uuid.MustParse(TestHotelUID)
	hotel, err := client.Hotel.Create().
		SetHotelUID(hotelUID).
		SetName("Integration hotel").
		SetCountry("Russia").
		SetCity("Moscow").
		SetAddress("Test address").
		SetPrice(1000).
		Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return hotel
}
