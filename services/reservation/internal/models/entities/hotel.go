package entities

import "uuid"

type Hotel struct {
	HotelUID uuid.UUID
	Name     string
	Country  string
	City     string
	Address  string
	Stars    *int
	Price    int
}

type Page struct {
	Page          int
	PageSize      int
	TotalElements int
	Items         []Hotel
}

type PageQuery struct {
	Page *int
	Size *int
}
