package converters

import (
	"lab2/payment/ent"
	"lab2/payment/internal/models/entities"
)

func Payment(row *ent.Payment) entities.Payment {
	return entities.Payment{
		PaymentUID: row.PaymentUID, Status: entities.Status(row.Status), Price: row.Price,
	}
}
