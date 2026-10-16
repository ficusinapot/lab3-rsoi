package entities

import "testing"

func TestTierBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		count int
		want  Loyalty
	}{
		{
			name: "negative count is preserved", count: -1,
			want: Loyalty{Status: StatusBronze, Discount: 5, ReservationCount: -1},
		},
		{name: "new user", count: 0, want: Loyalty{Status: StatusBronze, Discount: 5, ReservationCount: 0}},
		{name: "last bronze", count: 9, want: Loyalty{Status: StatusBronze, Discount: 5, ReservationCount: 9}},
		{name: "first silver", count: 10, want: Loyalty{Status: StatusSilver, Discount: 7, ReservationCount: 10}},
		{name: "last silver", count: 19, want: Loyalty{Status: StatusSilver, Discount: 7, ReservationCount: 19}},
		{name: "first gold", count: 20, want: Loyalty{Status: StatusGold, Discount: 10, ReservationCount: 20}},
		{name: "above gold boundary", count: 25, want: Loyalty{Status: StatusGold, Discount: 10, ReservationCount: 25}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ForCount(tc.count); got != tc.want {
				t.Errorf("loyalty = %+v, want %+v", got, tc.want)
			}
		})
	}
}
