package entities

import (
	"time"
	"uuid"
)

type CreateBooking struct {
	User      string
	HotelUID  uuid.UUID
	StartDate time.Time
	EndDate   time.Time
}

type InternalCreate struct {
	ReservationUID uuid.UUID
	HotelUID       uuid.UUID
	StartDate      time.Time
	EndDate        time.Time
	PaymentUID     uuid.UUID
}

type Created struct {
	ReservationUID uuid.UUID
	HotelUID       uuid.UUID
	StartDate      time.Time
	EndDate        time.Time
	Discount       int
	Status         ReservationStatus
	Payment        Payment
}

type UserInfo struct {
	Reservations []Reservation
	Loyalty      Loyalty
}

type CancelBooking struct {
	User           string
	ReservationUID uuid.UUID
}
