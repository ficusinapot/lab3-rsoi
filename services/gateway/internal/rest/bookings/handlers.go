package bookings

import (
	"context"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/rest/bookings/api"
)

func (h *handler) GetApiV1Hotels(
	ctx context.Context,
	request api.GetApiV1HotelsRequestObject,
) (api.GetApiV1HotelsResponseObject, error) {
	result, err := h.queries.Hotels(ctx, entities.PageQuery{
		Page: request.Params.Page, Size: request.Params.Size,
	})
	if err != nil {
		return nil, oops.Wrapf(err, "list hotels")
	}
	return api.GetApiV1Hotels200JSONResponse(pageResponse(result)), nil
}

func (h *handler) GetApiV1Loyalty(
	ctx context.Context,
	request api.GetApiV1LoyaltyRequestObject,
) (api.GetApiV1LoyaltyResponseObject, error) {
	result, err := h.queries.Loyalty(ctx, request.Params.XUserName)
	if err != nil {
		return nil, oops.Wrapf(err, "get loyalty")
	}
	return api.GetApiV1Loyalty200JSONResponse(loyaltyResponse(result)), nil
}

func (h *handler) GetApiV1Me(
	ctx context.Context,
	request api.GetApiV1MeRequestObject,
) (api.GetApiV1MeResponseObject, error) {
	result, err := h.queries.Me(ctx, request.Params.XUserName)
	if err != nil {
		return nil, oops.Wrapf(err, "get user info")
	}
	return api.GetApiV1Me200JSONResponse(userInfoResponse(result)), nil
}

func (h *handler) GetApiV1Reservations(
	ctx context.Context,
	request api.GetApiV1ReservationsRequestObject,
) (api.GetApiV1ReservationsResponseObject, error) {
	result, err := h.queries.List(ctx, request.Params.XUserName)
	if err != nil {
		return nil, oops.Wrapf(err, "list reservations")
	}
	return api.GetApiV1Reservations200JSONResponse(reservationResponses(result)), nil
}

func (h *handler) GetApiV1ReservationsReservationUid(
	ctx context.Context,
	request api.GetApiV1ReservationsReservationUidRequestObject,
) (api.GetApiV1ReservationsReservationUidResponseObject, error) {
	result, err := h.queries.Get(ctx, request.ReservationUid, request.Params.XUserName)
	if err != nil {
		return nil, oops.Wrapf(err, "get reservation")
	}
	return api.GetApiV1ReservationsReservationUid200JSONResponse(reservationResponse(result)), nil
}

func (h *handler) PostApiV1Reservations(
	ctx context.Context,
	request api.PostApiV1ReservationsRequestObject,
) (api.PostApiV1ReservationsResponseObject, error) {
	result, err := h.commands.Create(ctx, entities.CreateBooking{
		User:     request.Params.XUserName,
		HotelUID: request.Body.HotelUid, StartDate: request.Body.StartDate.Time, EndDate: request.Body.EndDate.Time,
	})
	if err != nil {
		return nil, oops.Wrapf(err, "create reservation")
	}
	return api.PostApiV1Reservations200JSONResponse(createdResponse(result)), nil
}

func (h *handler) DeleteApiV1ReservationsReservationUid(
	ctx context.Context,
	request api.DeleteApiV1ReservationsReservationUidRequestObject,
) (api.DeleteApiV1ReservationsReservationUidResponseObject, error) {
	if err := h.commands.Cancel(ctx, entities.CancelBooking{
		ReservationUID: request.ReservationUid, User: request.Params.XUserName,
	}); err != nil {
		return nil, oops.Wrapf(err, "cancel reservation")
	}
	return api.DeleteApiV1ReservationsReservationUid204Response{}, nil
}
