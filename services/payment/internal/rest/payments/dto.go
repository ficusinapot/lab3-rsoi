package payments

import (
	"uuid"

	"lab2/payment/internal/models/entities"
)

type CreateRequest struct {
	PaymentUID *uuid.UUID `json:"paymentUid,omitempty" format:"uuid"`
	Price      int        `json:"price"`
}

type PaymentResponse struct {
	PaymentUID uuid.UUID `json:"paymentUid" format:"uuid"`
	Status     string    `json:"status" enum:"PAID,CANCELED"`
	Price      int       `json:"price"`
}

func paymentResponse(item entities.Payment) PaymentResponse {
	return PaymentResponse{PaymentUID: item.PaymentUID, Status: string(item.Status), Price: item.Price}
}
