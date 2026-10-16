package repos

import (
	"context"
	"net/http"
	"uuid"

	"github.com/samber/oops"
	"github.com/sony/gobreaker/v2"
	"lab2/gateway/internal/clients/contracts"
	"lab2/gateway/internal/httpclient"
	"lab2/gateway/internal/loyalty/converters"
	"lab2/gateway/internal/models/entities"
)

type Repository struct {
	client  *contracts.ClientWithResponses
	breaker *gobreaker.TwoStepCircuitBreaker[struct{}]
}

func New(
	transport *httpclient.Client, baseURL string, breaker *gobreaker.TwoStepCircuitBreaker[struct{}],
) (*Repository, error) {
	client, err := contracts.NewClientWithResponses(baseURL, contracts.WithHTTPClient(transport))
	if err != nil {
		return nil, oops.Wrapf(err, "create loyalty API client")
	}
	return &Repository{client: client, breaker: breaker}, nil
}

func (r *Repository) Loyalty(ctx context.Context, user string) (entities.Loyalty, error) {
	result, err := httpclient.Read(ctx, r.breaker, entities.ErrLoyaltyUnavailable, func() (entities.Loyalty, error) {
		response, err := r.client.GetLoyaltyWithResponse(ctx, &contracts.GetLoyaltyParams{XUserName: user})
		if err != nil {
			return entities.Loyalty{}, oops.Wrapf(httpclient.ResponseReadError(err), "get loyalty")
		}
		if err := httpclient.CheckStatus(response.HTTPResponse, http.StatusOK); err != nil {
			return entities.Loyalty{}, oops.Wrapf(err, "get loyalty")
		}
		if response.JSON200 == nil {
			return entities.Loyalty{}, entities.ErrUpstreamUnavailable
		}
		return converters.Entity(*response.JSON200), nil
	})
	return result, oops.Wrap(err)
}

func (r *Repository) ChangeLoyalty(ctx context.Context, user string, delta int, operationID string) (err error) {
	defer func() { err = httpclient.ServiceError(ctx, entities.ErrLoyaltyUnavailable, err) }()
	params := &contracts.IncreaseLoyaltyParams{XUserName: user, IdempotencyKey: new(operationID)}
	if delta < 0 {
		response, err := r.client.DecreaseLoyaltyWithResponse(ctx,
			&contracts.DecreaseLoyaltyParams{XUserName: user, IdempotencyKey: new(operationID)})
		if err != nil {
			return oops.Wrapf(httpclient.ResponseReadError(err), "decrease loyalty")
		}
		return oops.Wrap(httpclient.CheckStatus(response.HTTPResponse, http.StatusOK))
	}
	response, err := r.client.IncreaseLoyaltyWithResponse(ctx, params)
	if err != nil {
		return oops.Wrapf(httpclient.ResponseReadError(err), "increase loyalty")
	}
	return oops.Wrap(httpclient.CheckStatus(response.HTTPResponse, http.StatusOK))
}

func (r *Repository) ResolveCreation(ctx context.Context, user string, id uuid.UUID) (committed bool, err error) {
	defer func() { err = httpclient.ServiceError(ctx, entities.ErrLoyaltyUnavailable, err) }()
	response, err := r.client.ResolveCreationWithResponse(ctx, id, &contracts.ResolveCreationParams{XUserName: user})
	if err != nil {
		return false, oops.Wrapf(httpclient.ResponseReadError(err), "resolve creation")
	}
	if err := httpclient.CheckStatus(response.HTTPResponse, http.StatusOK); err != nil {
		return false, oops.Wrap(err)
	}
	if response.JSON200 == nil {
		return false, entities.ErrUpstreamUnavailable
	}
	return response.JSON200.Committed, nil
}
