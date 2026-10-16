//go:build e2e

package e2e

import "uuid"

type createRequest struct {
	HotelUID  string `json:"hotelUid"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

type hotelPage struct {
	Items []struct {
		HotelUID uuid.UUID `json:"hotelUid"`
	} `json:"items"`
}

type paymentResult struct {
	Status string `json:"status"`
	Price  int    `json:"price"`
}

type createdResult struct {
	ReservationUID uuid.UUID     `json:"reservationUid"`
	Discount       int           `json:"discount"`
	Status         string        `json:"status"`
	Payment        paymentResult `json:"payment"`
}

type reservationResult struct {
	Status string `json:"status"`
	Hotel  struct {
		FullAddress string `json:"fullAddress"`
	} `json:"hotel"`
	Payment paymentResult `json:"payment"`
}

type loyaltyResult struct {
	Status           string `json:"status"`
	Discount         int    `json:"discount"`
	ReservationCount int    `json:"reservationCount"`
}

type userResult struct {
	Reservations []reservationResult `json:"reservations"`
	Loyalty      loyaltyResult       `json:"loyalty"`
}
