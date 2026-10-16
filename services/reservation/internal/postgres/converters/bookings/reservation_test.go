package bookings

import (
	"testing"
	"time"
	"uuid"

	"lab2/reservation/ent"
	entreservation "lab2/reservation/ent/reservation"
	"lab2/reservation/internal/models/entities"
)

func TestReservation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		invalidate func(*ent.Reservation)
	}{
		{name: "complete reservation"},
		{name: "missing start date", invalidate: func(row *ent.Reservation) { row.StartDate = nil }},
		{name: "missing end date", invalidate: func(row *ent.Reservation) { row.EndDate = nil }},
		{name: "missing hotel", invalidate: func(row *ent.Reservation) { row.Edges.Hotel = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start := time.Date(2030, 1, 1, 3, 0, 0, 0, time.FixedZone("Moscow", 3*60*60))
			end := start.AddDate(0, 0, 2)
			row := &ent.Reservation{
				ReservationUID: uuid.New(),
				PaymentUID:     uuid.New(),
				Status:         entreservation.StatusPAID,
				StartDate:      &start,
				EndDate:        &end,
				Edges:          ent.ReservationEdges{Hotel: &ent.Hotel{HotelUID: uuid.New()}},
			}
			want := entities.Reservation{
				ReservationUID: row.ReservationUID,
				HotelUID:       row.Edges.Hotel.HotelUID,
				PaymentUID:     row.PaymentUID,
				Status:         entities.StatusPaid,
				StartDate:      start.UTC(),
				EndDate:        end.UTC(),
			}
			if tc.invalidate != nil {
				tc.invalidate(row)
			}
			got, err := Reservation(t.Context(), row)
			if tc.invalidate != nil {
				if err == nil {
					t.Fatal("Reservation accepted an incomplete row")
				}
				if got != (entities.Reservation{}) {
					t.Fatalf("Reservation = %+v, want zero value on error", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("Reservation = %+v, want %+v", got, want)
			}
		})
	}
}
