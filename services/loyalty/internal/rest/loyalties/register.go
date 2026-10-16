package loyalties

import (
	"context"
	"log/slog"
	"net/http"
	"uuid"

	"github.com/danielgtaylor/huma/v2"
	"lab2/loyalty/internal/models/coreifc"
	"lab2/loyalty/internal/rest/response"
	"lab2/platform/rest"
)

type (
	Input struct {
		Username string `header:"X-User-Name"`
	}
	ChangeInput struct {
		Username    string `header:"X-User-Name"`
		OperationID string `header:"Idempotency-Key" maxLength:"128"`
	}
	Output struct{ Body LoyaltyResponse }
)

type ResolveInput struct {
	Username string    `header:"X-User-Name"`
	UID      uuid.UUID `path:"reservationUid"`
}
type ResolveOutput struct {
	Body struct {
		Committed bool `json:"committed"`
	}
}

func Register(api huma.API, service coreifc.Loyalties, logger *slog.Logger, _ rest.Config) {
	huma.Register(api, huma.Operation{OperationID: "get-loyalty", Method: http.MethodGet, Path: "/loyalty"},
		func(ctx context.Context, in *Input) (*Output, error) {
			item, err := service.Get(ctx, in.Username)
			if err != nil {
				return nil, response.Error(ctx, logger, err)
			}
			return &Output{Body: loyaltyResponse(item)}, nil
		})

	huma.Register(api, huma.Operation{
		OperationID: "resolve-creation", Method: http.MethodPost, Path: "/loyalty/reservations/{reservationUid}/resolve",
	}, func(ctx context.Context, in *ResolveInput) (*ResolveOutput, error) {
		committed, err := service.ResolveCreation(ctx, in.Username, in.UID)
		if err != nil {
			return nil, response.Error(ctx, logger, err)
		}
		out := &ResolveOutput{}
		out.Body.Committed = committed
		return out, nil
	})
	for _, op := range []struct {
		method, id string
		delta      int
	}{
		{method: http.MethodPost, id: "increase-loyalty", delta: 1},
		{method: http.MethodDelete, id: "decrease-loyalty", delta: -1},
	} {
		huma.Register(api, huma.Operation{OperationID: op.id, Method: op.method, Path: "/loyalty/reservations"},
			func(ctx context.Context, in *ChangeInput) (*Output, error) {
				item, err := service.Change(ctx, in.Username, op.delta, in.OperationID)
				if err != nil {
					return nil, response.Error(ctx, logger, err)
				}
				return &Output{Body: loyaltyResponse(item)}, nil
			})
	}
}
