package entities

import (
	"time"
	"uuid"
)

type Status string

const (
	StatusPaid     Status = "PAID"
	StatusCanceled Status = "CANCELED"
)

type Reservation struct {
	ReservationUID uuid.UUID
	HotelUID       uuid.UUID
	PaymentUID     uuid.UUID
	Status         Status
	StartDate      time.Time
	EndDate        time.Time
}

type Booking struct {
	ReservationUID uuid.UUID
	HotelUID       uuid.UUID
	PaymentUID     uuid.UUID
	Username       string
	StartDate      time.Time
	EndDate        time.Time
}
