package payments

import (
	"context"
	"log/slog"
	"net/http"
	"uuid"

	"lab2/payment/internal/models/entities"

	"github.com/danielgtaylor/huma/v2"
	"lab2/payment/internal/models/coreifc"
	"lab2/payment/internal/rest/response"
	"lab2/platform/rest"
)

type (
	CreateInput struct {
		Body CreateRequest
	}
	ItemInput struct {
		UID uuid.UUID `path:"paymentUid"`
		Key string    `header:"Idempotency-Key"`
	}
	Output struct{ Body PaymentResponse }
)

func Register(api huma.API, service coreifc.Payments, logger *slog.Logger, cfg rest.Config) {
	huma.Register(api, huma.Operation{
		OperationID: "create-payment", Method: http.MethodPost, Path: "/payments",
		DefaultStatus: http.StatusCreated, MaxBodyBytes: cfg.MaxBodyBytes, BodyReadTimeout: cfg.ReadTimeout,
	},
		func(ctx context.Context, in *CreateInput) (*Output, error) {
			var item entities.Payment
			var err error
			if in.Body.PaymentUID == nil {
				item, err = service.Create(ctx, in.Body.Price)
			} else {
				item, err = service.CreateWithID(ctx, *in.Body.PaymentUID, in.Body.Price)
			}
			if err != nil {
				return nil, response.Error(ctx, logger, err)
			}
			return &Output{Body: paymentResponse(item)}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "get-payment", Method: http.MethodGet, Path: "/payments/{paymentUid}"},
		func(ctx context.Context, in *ItemInput) (*Output, error) {
			item, err := service.Get(ctx, in.UID)
			if err != nil {
				return nil, response.Error(ctx, logger, err)
			}
			return &Output{Body: paymentResponse(item)}, nil
		})
	huma.Register(api, huma.Operation{
		OperationID: "cancel-payment", Method: http.MethodDelete, Path: "/payments/{paymentUid}",
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *ItemInput) (*struct{}, error) {
		var err error
		switch {
		case in.Key == "":
			err = service.Cancel(ctx, in.UID)
		case in.Key != in.UID.String():
			err = entities.ErrInvalidInput
		default:
			err = service.CancelKnown(ctx, in.UID)
		}
		if err != nil {
			return nil, response.Error(ctx, logger, err)
		}
		return nil, nil
	})
}
