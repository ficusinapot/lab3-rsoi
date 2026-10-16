package entities

type Status string

const (
	StatusBronze Status = "BRONZE"
	StatusSilver Status = "SILVER"
	StatusGold   Status = "GOLD"
)

type Loyalty struct {
	Status           Status
	Discount         int
	ReservationCount int
}

func ForCount(count int) Loyalty {
	switch {
	case count >= 20:
		return Loyalty{Status: StatusGold, Discount: 10, ReservationCount: count}
	case count >= 10:
		return Loyalty{Status: StatusSilver, Discount: 7, ReservationCount: count}
	default:
		return Loyalty{Status: StatusBronze, Discount: 5, ReservationCount: count}
	}
}
