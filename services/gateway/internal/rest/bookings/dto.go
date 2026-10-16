package bookings

import (
	openapi_types "github.com/oapi-codegen/runtime/types"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/rest/bookings/api"
)

func hotelResponse(h entities.Hotel) api.HotelResponse {
	return api.HotelResponse{
		HotelUid: h.HotelUID, Name: h.Name, Country: h.Country, City: h.City,
		Address: h.Address, Stars: h.Stars, Price: h.Price,
	}
}

func pageResponse(p entities.Page) api.PaginationResponse {
	items := make([]api.HotelResponse, len(p.Items))
	for i, item := range p.Items {
		items[i] = hotelResponse(item)
	}
	return api.PaginationResponse{Page: p.Page, PageSize: p.PageSize, TotalElements: p.TotalElements, Items: items}
}

func paymentResponse(p entities.Payment) api.PaymentInfo {
	if p.Status == "" {
		return api.PaymentInfo{}
	}
	return api.PaymentInfo{Status: new(api.PaymentInfoStatus(p.Status)), Price: new(p.Price)}
}

func loyaltyResponse(l entities.Loyalty) api.LoyaltyInfoResponse {
	if l.Status == "" {
		return api.LoyaltyInfoResponse{}
	}
	return api.LoyaltyInfoResponse{
		Status:   new(api.LoyaltyInfoResponseStatus(l.Status)),
		Discount: new(l.Discount), ReservationCount: new(l.ReservationCount),
	}
}

func reservationResponse(r entities.Reservation) api.ReservationResponse {
	return api.ReservationResponse{
		ReservationUid: r.ReservationUID,
		Hotel: api.HotelInfo{
			HotelUid: r.Hotel.HotelUID, Name: r.Hotel.Name,
			FullAddress: r.Hotel.FullAddress, Stars: r.Hotel.Stars,
		},
		StartDate: openapi_types.Date{Time: r.StartDate}, EndDate: openapi_types.Date{Time: r.EndDate},
		Status: api.ReservationResponseStatus(r.Status), Payment: paymentResponse(r.Payment),
	}
}

func reservationResponses(rows []entities.Reservation) []api.ReservationResponse {
	items := make([]api.ReservationResponse, len(rows))
	for i, row := range rows {
		items[i] = reservationResponse(row)
	}
	return items
}

func createdResponse(c entities.Created) api.CreateReservationResponse {
	return api.CreateReservationResponse{
		ReservationUid: c.ReservationUID, HotelUid: c.HotelUID,
		StartDate: openapi_types.Date{Time: c.StartDate}, EndDate: openapi_types.Date{Time: c.EndDate},
		Discount: c.Discount, Status: api.CreateReservationResponseStatus(c.Status), Payment: paymentResponse(c.Payment),
	}
}

func userInfoResponse(u entities.UserInfo) api.UserInfoResponse {
	return api.UserInfoResponse{Reservations: reservationResponses(u.Reservations), Loyalty: loyaltyResponse(u.Loyalty)}
}
