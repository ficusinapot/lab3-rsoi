package hotels

import (
	"uuid"

	"lab2/reservation/internal/models/entities"

	"github.com/samber/mo"
)

type ListInput struct {
	Page mo.Option[int] `query:"page"`
	Size mo.Option[int] `query:"size"`
}

type ListOutput struct {
	Body PageResponse
}

type GetInput struct {
	UID uuid.UUID `path:"hotelUid"`
}

type GetOutput struct {
	Body HotelResponse
}

type HotelResponse struct {
	HotelUID uuid.UUID `json:"hotelUid" format:"uuid"`
	Name     string    `json:"name"`
	Country  string    `json:"country"`
	City     string    `json:"city"`
	Address  string    `json:"address"`
	Stars    *int      `json:"stars"`
	Price    int       `json:"price"`
}

func hotelResponse(item entities.Hotel) HotelResponse {
	return HotelResponse{
		HotelUID: item.HotelUID, Name: item.Name, Country: item.Country,
		City: item.City, Address: item.Address, Stars: item.Stars, Price: item.Price,
	}
}

type PageResponse struct {
	Page          int             `json:"page"`
	PageSize      int             `json:"pageSize"`
	TotalElements int             `json:"totalElements"`
	Items         []HotelResponse `json:"items"`
}

func pageResponse(page entities.Page) PageResponse {
	var items []HotelResponse
	if page.Items != nil {
		items = make([]HotelResponse, len(page.Items))
		for i, item := range page.Items {
			items[i] = hotelResponse(item)
		}
	}
	return PageResponse{
		Page: page.Page, PageSize: page.PageSize, TotalElements: page.TotalElements, Items: items,
	}
}
