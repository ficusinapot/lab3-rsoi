package entities

import (
	"time"
	"uuid"
)

type CreateInput struct {
	ReservationUID uuid.UUID
	HotelUID       uuid.UUID
	PaymentUID     uuid.UUID
	StartDate      time.Time
	EndDate        time.Time
}
