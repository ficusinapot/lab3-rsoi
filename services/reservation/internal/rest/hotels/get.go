package hotels

import (
	"context"
	"log/slog"
	"net/http"

	"lab2/reservation/internal/models/coreifc"

	"github.com/danielgtaylor/huma/v2"
	"lab2/reservation/internal/rest/response"
)

func registerGet(api huma.API, service coreifc.Hotels, logger *slog.Logger) {
	huma.Register(api, huma.Operation{
		OperationID: "get-hotel",
		Method:      http.MethodGet,
		Path:        "/hotels/{hotelUid}",
		Tags:        []string{hotelsTag},
		Errors: []int{
			http.StatusBadRequest,
			http.StatusNotFound,
			http.StatusInternalServerError,
		},
	}, func(ctx context.Context, input *GetInput) (*GetOutput, error) {
		hotel, err := service.Get(ctx, input.UID)
		if err != nil {
			return nil, response.Error(ctx, logger, err)
		}
		return &GetOutput{Body: hotelResponse(hotel)}, nil
	})
}
