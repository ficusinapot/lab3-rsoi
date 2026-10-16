package entities

import "uuid"

type PaymentStatus string

const (
	PaymentPaid     PaymentStatus = "PAID"
	PaymentCanceled PaymentStatus = "CANCELED"
)

type Payment struct {
	Status PaymentStatus
	Price  int
}

type InternalPayment struct {
	Payment
	PaymentUID uuid.UUID
}

type PaymentCreate struct {
	PaymentUID uuid.UUID
	Price      int
}
