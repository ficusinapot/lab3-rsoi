package entities

import (
	"time"
	"uuid"
)

type ReservationStatus string

const (
	ReservationPaid     ReservationStatus = "PAID"
	ReservationCanceled ReservationStatus = "CANCELED"
)

type InternalReservation struct {
	ReservationUID uuid.UUID
	HotelUID       uuid.UUID
	PaymentUID     uuid.UUID
	StartDate      time.Time
	EndDate        time.Time
	Status         ReservationStatus
}
type Reservation struct {
	ReservationUID uuid.UUID
	Hotel          HotelInfo
	StartDate      time.Time
	EndDate        time.Time
	Status         ReservationStatus
	Payment        Payment
}
