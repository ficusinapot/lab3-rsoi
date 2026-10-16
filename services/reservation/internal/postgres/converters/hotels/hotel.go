package hotels

import (
	"lab2/reservation/ent"
	"lab2/reservation/internal/models/entities"
)

func Hotel(row *ent.Hotel) entities.Hotel {
	return entities.Hotel{
		HotelUID: row.HotelUID, Name: row.Name, Country: row.Country,
		City: row.City, Address: row.Address, Stars: row.Stars, Price: row.Price,
	}
}
