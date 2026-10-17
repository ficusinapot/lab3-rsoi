package repos

import (
	"context"
	"net/http"
	"uuid"

	"github.com/samber/oops"
	"lab2/gateway/internal/clients/contracts"
	"lab2/gateway/internal/httpclient"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/payment/converters"
)

type Repository struct {
	client  *contracts.ClientWithResponses
	breaker *httpclient.Breaker
}

func New(
	transport *httpclient.Client, baseURL string, breaker *httpclient.Breaker,
) (*Repository, error) {
	client, err := contracts.NewClientWithResponses(baseURL, contracts.WithHTTPClient(transport))
	if err != nil {
		return nil, oops.Wrapf(err, "create payment API client")
	}
	return &Repository{client: client, breaker: breaker}, nil
}

func (r *Repository) Payment(ctx context.Context, id uuid.UUID) (entities.Payment, error) {
	result, err := httpclient.Read(ctx, r.breaker, entities.ErrPaymentUnavailable, func() (entities.Payment, error) {
		response, err := r.client.GetPaymentWithResponse(ctx, id)
		if err != nil {
			return entities.Payment{}, oops.Wrapf(httpclient.ResponseReadError(err), "get payment")
		}
		if err := httpclient.CheckStatus(response.HTTPResponse, http.StatusOK); err != nil {
			return entities.Payment{}, oops.Wrapf(err, "get payment")
		}
		if response.JSON200 == nil {
			return entities.Payment{}, entities.ErrUpstreamUnavailable
		}
		return converters.Entity(*response.JSON200), nil
	})
	return result, oops.Wrap(err)
}

func (r *Repository) CreatePayment(
	ctx context.Context, input entities.PaymentCreate,
) (_ entities.InternalPayment, err error) {
	defer func() { err = httpclient.ServiceError(ctx, entities.ErrPaymentUnavailable, err) }()
	request := contracts.CreatePayment{Price: input.Price}
	if input.PaymentUID == uuid.Nil() {
		return entities.InternalPayment{}, oops.Wrap(entities.ErrInvalidInput)
	}
	request.PaymentUid = &input.PaymentUID
	response, err := r.client.CreatePaymentWithResponse(ctx, request)
	if err != nil {
		return entities.InternalPayment{}, oops.Wrapf(httpclient.ResponseReadError(err), "create payment")
	}
	if err := httpclient.CheckStatus(response.HTTPResponse, http.StatusCreated); err != nil {
		return entities.InternalPayment{}, oops.Wrapf(err, "create payment")
	}
	if response.JSON201 == nil {
		return entities.InternalPayment{}, entities.ErrUpstreamUnavailable
	}
	if response.JSON201.PaymentUid != input.PaymentUID {
		return entities.InternalPayment{}, oops.Wrap(entities.ErrUpstreamUnavailable)
	}
	return converters.InternalEntity(*response.JSON201), nil
}

func (r *Repository) CancelPayment(ctx context.Context, id uuid.UUID) (err error) {
	defer func() { err = httpclient.ServiceError(ctx, entities.ErrPaymentUnavailable, err) }()
	response, err := r.client.CancelPaymentWithResponse(ctx, id, func(_ context.Context, request *http.Request) error {
		request.Header.Set("Idempotency-Key", id.String())
		return nil
	})
	if err != nil {
		return oops.Wrapf(httpclient.ResponseReadError(err), "cancel payment")
	}
	return oops.Wrap(httpclient.CheckStatus(response.HTTPResponse, http.StatusNoContent))
}
