package entities

import "uuid"

type Status string

const (
	StatusPaid     Status = "PAID"
	StatusCanceled Status = "CANCELED"
)

type Payment struct {
	PaymentUID uuid.UUID
	Status     Status
	Price      int
}
