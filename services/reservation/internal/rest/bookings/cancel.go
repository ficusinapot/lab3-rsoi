package bookings

import (
	"context"
	"log/slog"
	"net/http"

	"lab2/reservation/internal/models/entities"

	"lab2/reservation/internal/models/coreifc"

	"github.com/danielgtaylor/huma/v2"
	"lab2/reservation/internal/rest/response"
)

func registerCancel(api huma.API, service coreifc.Bookings, logger *slog.Logger) {
	huma.Register(api, huma.Operation{
		OperationID:   "cancel-reservation",
		Method:        http.MethodDelete,
		Path:          "/reservations/{reservationUid}",
		Tags:          []string{reservationsTag},
		DefaultStatus: http.StatusNoContent,
		Errors: []int{
			http.StatusBadRequest,
			http.StatusNotFound,
			http.StatusInternalServerError,
		},
	}, func(ctx context.Context, input *ItemInput) (*struct{}, error) {
		var err error
		switch {
		case input.Key == "":
			err = service.Cancel(ctx, input.UID, input.Username)
		case input.Key != input.UID.String():
			err = entities.ErrInvalidInput
		default:
			err = service.CancelKnown(ctx, input.UID, input.Username)
		}
		if err != nil {
			return nil, response.Error(ctx, logger, err)
		}
		return nil, nil
	})
}
