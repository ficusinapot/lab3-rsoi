package converters

import (
	"lab2/loyalty/ent"
	"lab2/loyalty/internal/models/entities"
)

func Loyalty(row *ent.Loyalty) entities.Loyalty {
	return entities.Loyalty{
		Status: entities.Status(row.Status), Discount: row.Discount, ReservationCount: row.ReservationCount,
	}
}

func ChangeRecord(row *ent.Operation) *entities.ChangeRecord {
	if row == nil {
		return nil
	}
	return &entities.ChangeRecord{User: row.Username, Delta: row.Delta}
}
