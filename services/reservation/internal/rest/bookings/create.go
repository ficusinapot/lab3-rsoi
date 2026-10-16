package bookings

import (
	"context"
	"log/slog"
	"net/http"
	"uuid"

	"lab2/reservation/internal/models/coreifc"

	"github.com/danielgtaylor/huma/v2"
	"lab2/platform/rest"
	"lab2/reservation/internal/models/entities"
	"lab2/reservation/internal/rest/response"
)

func registerCreate(api huma.API, service coreifc.Bookings, logger *slog.Logger, cfg rest.Config) {
	huma.Register(api, huma.Operation{
		OperationID:   "create-reservation",
		Method:        http.MethodPost,
		Path:          "/reservations",
		Tags:          []string{reservationsTag},
		DefaultStatus: http.StatusCreated,
		Errors: []int{
			http.StatusBadRequest,
			http.StatusNotFound,
			http.StatusUnprocessableEntity,
			http.StatusInternalServerError,
		},
		MaxBodyBytes:    cfg.MaxBodyBytes,
		BodyReadTimeout: cfg.ReadTimeout,
	}, func(ctx context.Context, input *CreateInput) (*CreateOutput, error) {
		request := entities.CreateInput{
			HotelUID: input.Body.HotelUID, PaymentUID: input.Body.PaymentUID,
			StartDate: input.Body.StartDate.Time, EndDate: input.Body.EndDate.Time,
		}
		if input.Body.ReservationUID != nil {
			if *input.Body.ReservationUID == uuid.Nil() {
				return nil, response.Error(ctx, logger, entities.ErrInvalidInput)
			}
			request.ReservationUID = *input.Body.ReservationUID
		}
		item, err := service.Create(ctx, input.Username, request)
		if err != nil {
			return nil, response.Error(ctx, logger, err)
		}
		return &CreateOutput{
			Location: cfg.APIPrefix + "/reservations/" + item.ReservationUID.String(),
			Body:     reservationResponse(item),
		}, nil
	})
}
