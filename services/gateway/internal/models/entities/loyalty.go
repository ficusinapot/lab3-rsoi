package entities

type LoyaltyStatus string

const (
	LoyaltyBronze LoyaltyStatus = "BRONZE"
	LoyaltySilver LoyaltyStatus = "SILVER"
	LoyaltyGold   LoyaltyStatus = "GOLD"
)

type Loyalty struct {
	Status           LoyaltyStatus
	Discount         int
	ReservationCount int
}
