package loyalties

import "lab2/loyalty/internal/models/entities"

type LoyaltyResponse struct {
	Status           string `json:"status" enum:"BRONZE,SILVER,GOLD"`
	Discount         int    `json:"discount"`
	ReservationCount int    `json:"reservationCount"`
}

func loyaltyResponse(item entities.Loyalty) LoyaltyResponse {
	return LoyaltyResponse{
		Status: string(item.Status), Discount: item.Discount, ReservationCount: item.ReservationCount,
	}
}
