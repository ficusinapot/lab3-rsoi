package paymentifc

import (
	"context"
	"uuid"

	"lab2/gateway/internal/models/entities"
)

type Canceler interface {
	CancelPayment(context.Context, uuid.UUID) error
}

type Reader interface {
	Payment(context.Context, uuid.UUID) (entities.Payment, error)
}

type Creator interface {
	CreatePayment(context.Context, entities.PaymentCreate) (entities.InternalPayment, error)
}

type Payments interface {
	Reader
	Creator
	Canceler
}
