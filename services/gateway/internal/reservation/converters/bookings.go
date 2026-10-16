package converters

import (
	"uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"lab2/gateway/internal/clients/contracts"
	"lab2/gateway/internal/models/entities"
)

func HotelEntity(h contracts.Hotel) entities.Hotel {
	return entities.Hotel{
		HotelUID: h.HotelUid, Name: h.Name, Country: h.Country, City: h.City,
		Address: h.Address, Stars: h.Stars, Price: h.Price,
	}
}

func PageEntity(p contracts.HotelPage) entities.Page {
	items := make([]entities.Hotel, len(p.Items))
	for i, item := range p.Items {
		items[i] = HotelEntity(item)
	}
	return entities.Page{Page: p.Page, PageSize: p.PageSize, TotalElements: p.TotalElements, Items: items}
}

func ReservationEntity(r contracts.Reservation) entities.InternalReservation {
	return entities.InternalReservation{
		ReservationUID: r.ReservationUid, HotelUID: r.HotelUid, PaymentUID: r.PaymentUid,
		StartDate: r.StartDate.Time, EndDate: r.EndDate.Time, Status: entities.ReservationStatus(r.Status),
	}
}

func NewCreateRequest(input entities.InternalCreate) contracts.CreateReservation {
	request := contracts.CreateReservation{
		HotelUid: input.HotelUID, StartDate: openapi_types.Date{Time: input.StartDate},
		EndDate: openapi_types.Date{Time: input.EndDate}, PaymentUid: input.PaymentUID,
	}
	if input.ReservationUID != uuid.Nil() {
		request.ReservationUid = &input.ReservationUID
	}
	return request
}
