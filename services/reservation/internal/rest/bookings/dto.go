package bookings

import (
	"uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"lab2/reservation/internal/models/entities"
)

type UserInput struct {
	Username string `header:"X-User-Name"`
}

type ItemInput struct {
	UserInput
	UID uuid.UUID `path:"reservationUid"`
	Key string    `header:"Idempotency-Key"`
}

type ListOutput struct {
	Body []ReservationResponse
}

type ItemOutput struct {
	Body ReservationResponse
}

type CreateInput struct {
	UserInput
	Body CreateRequest
}

type CreateRequest struct {
	ReservationUID *uuid.UUID         `json:"reservationUid,omitempty" format:"uuid"`
	HotelUID       uuid.UUID          `json:"hotelUid"`
	PaymentUID     uuid.UUID          `json:"paymentUid"`
	StartDate      openapi_types.Date `json:"startDate" format:"date"`
	EndDate        openapi_types.Date `json:"endDate" format:"date"`
}

type CreateOutput struct {
	Location string `header:"Location"`
	Body     ReservationResponse
}

type ReservationResponse struct {
	ReservationUID uuid.UUID          `json:"reservationUid" format:"uuid"`
	HotelUID       uuid.UUID          `json:"hotelUid" format:"uuid"`
	PaymentUID     uuid.UUID          `json:"paymentUid" format:"uuid"`
	Status         string             `json:"status" enum:"PAID,CANCELED"`
	StartDate      openapi_types.Date `json:"startDate" format:"date"`
	EndDate        openapi_types.Date `json:"endDate" format:"date"`
}

func reservationResponse(item entities.Reservation) ReservationResponse {
	return ReservationResponse{
		ReservationUID: item.ReservationUID, HotelUID: item.HotelUID,
		PaymentUID: item.PaymentUID, Status: string(item.Status),
		StartDate: openapi_types.Date{Time: item.StartDate}, EndDate: openapi_types.Date{Time: item.EndDate},
	}
}

func reservationResponses(items []entities.Reservation) []ReservationResponse {
	if items == nil {
		return nil
	}
	out := make([]ReservationResponse, len(items))
	for i, item := range items {
		out[i] = reservationResponse(item)
	}
	return out
}
