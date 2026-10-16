package converters

import (
	"lab2/gateway/internal/clients/contracts"
	"lab2/gateway/internal/models/entities"
)

func Entity(p contracts.Payment) entities.Payment {
	return entities.Payment{Status: entities.PaymentStatus(p.Status), Price: p.Price}
}

func InternalEntity(p contracts.Payment) entities.InternalPayment {
	return entities.InternalPayment{Payment: Entity(p), PaymentUID: p.PaymentUid}
}
