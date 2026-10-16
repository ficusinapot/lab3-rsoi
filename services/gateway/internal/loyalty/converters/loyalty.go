package converters

import (
	"lab2/gateway/internal/clients/contracts"
	"lab2/gateway/internal/models/entities"
)

func Entity(l contracts.Loyalty) entities.Loyalty {
	return entities.Loyalty{
		Status: entities.LoyaltyStatus(l.Status), Discount: l.Discount, ReservationCount: l.ReservationCount,
	}
}
