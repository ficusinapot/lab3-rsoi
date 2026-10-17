package repos

import (
	"context"
	"net/http"
	"uuid"

	"lab2/gateway/internal/clients/contracts"
	"lab2/gateway/internal/httpclient"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/reservation/converters"

	"github.com/samber/oops"
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
		return nil, oops.Wrapf(err, "create reservation API client")
	}
	return &Repository{client: client, breaker: breaker}, nil
}

func (r *Repository) Hotels(ctx context.Context, query entities.PageQuery) (entities.Page, error) {
	result, err := httpclient.Read(ctx, r.breaker, entities.ErrReservationUnavailable, func() (entities.Page, error) {
		response, err := r.client.ListHotelsWithResponse(ctx, &contracts.ListHotelsParams{Page: query.Page, Size: query.Size})
		if err != nil {
			return entities.Page{}, oops.With("operation", "list hotels").Wrap(httpclient.ResponseReadError(err))
		}
		if err := httpclient.CheckStatus(response.HTTPResponse, http.StatusOK); err != nil {
			return entities.Page{}, oops.Wrapf(err, "list hotels")
		}
		if response.JSON200 == nil {
			return entities.Page{}, entities.ErrUpstreamUnavailable
		}
		return converters.PageEntity(*response.JSON200), nil
	})
	return result, oops.Wrap(err)
}

func (r *Repository) Hotel(ctx context.Context, id uuid.UUID) (entities.Hotel, error) {
	result, err := httpclient.Read(ctx, r.breaker, entities.ErrReservationUnavailable, func() (entities.Hotel, error) {
		response, err := r.client.GetHotelWithResponse(ctx, id)
		if err != nil {
			return entities.Hotel{}, oops.With("operation", "get hotel").Wrap(httpclient.ResponseReadError(err))
		}
		if err := httpclient.CheckStatus(response.HTTPResponse, http.StatusOK); err != nil {
			return entities.Hotel{}, oops.Wrapf(err, "get hotel")
		}
		if response.JSON200 == nil {
			return entities.Hotel{}, entities.ErrUpstreamUnavailable
		}
		return converters.HotelEntity(*response.JSON200), nil
	})
	return result, oops.Wrap(err)
}

func (r *Repository) Reservations(ctx context.Context, user string) ([]entities.InternalReservation, error) {
	result, err := httpclient.Read(ctx, r.breaker, entities.ErrReservationUnavailable,
		func() ([]entities.InternalReservation, error) {
			response, err := r.client.ListReservationsWithResponse(ctx, &contracts.ListReservationsParams{XUserName: user})
			if err != nil {
				return nil, oops.With("operation", "list reservations").Wrap(httpclient.ResponseReadError(err))
			}
			if err := httpclient.CheckStatus(response.HTTPResponse, http.StatusOK); err != nil {
				return nil, oops.Wrapf(err, "list reservations")
			}
			if response.JSON200 == nil {
				return nil, entities.ErrUpstreamUnavailable
			}
			items := make([]entities.InternalReservation, len(*response.JSON200))
			for i, item := range *response.JSON200 {
				items[i] = converters.ReservationEntity(item)
			}
			return items, nil
		})
	return result, oops.Wrap(err)
}

func (r *Repository) Reservation(ctx context.Context, id uuid.UUID, user string) (entities.InternalReservation, error) {
	result, err := httpclient.Read(ctx, r.breaker, entities.ErrReservationUnavailable,
		func() (entities.InternalReservation, error) {
			response, err := r.client.GetReservationWithResponse(ctx, id, &contracts.GetReservationParams{XUserName: user})
			if err != nil {
				return entities.InternalReservation{},
					oops.With("operation", "get reservation").Wrap(httpclient.ResponseReadError(err))
			}
			if err := httpclient.CheckStatus(response.HTTPResponse, http.StatusOK); err != nil {
				return entities.InternalReservation{}, oops.Wrapf(err, "get reservation")
			}
			if response.JSON200 == nil {
				return entities.InternalReservation{}, entities.ErrUpstreamUnavailable
			}
			return converters.ReservationEntity(*response.JSON200), nil
		})
	return result, oops.Wrap(err)
}

func (r *Repository) CreateReservation(
	ctx context.Context, user string, input entities.InternalCreate,
) (_ entities.InternalReservation, err error) {
	defer func() { err = httpclient.ServiceError(ctx, entities.ErrReservationUnavailable, err) }()
	response, err := r.client.CreateReservationWithResponse(ctx,
		&contracts.CreateReservationParams{XUserName: user}, converters.NewCreateRequest(input))
	if err != nil {
		return entities.InternalReservation{},
			oops.With("operation", "create reservation").Wrap(httpclient.ResponseReadError(err))
	}
	if err := httpclient.CheckStatus(response.HTTPResponse, http.StatusCreated); err != nil {
		return entities.InternalReservation{}, oops.Wrapf(err, "create reservation")
	}
	if response.JSON201 == nil {
		return entities.InternalReservation{}, entities.ErrUpstreamUnavailable
	}
	if input.ReservationUID != uuid.Nil() && response.JSON201.ReservationUid != input.ReservationUID {
		return entities.InternalReservation{}, oops.Wrap(entities.ErrUpstreamUnavailable)
	}
	return converters.ReservationEntity(*response.JSON201), nil
}

func (r *Repository) CancelReservation(ctx context.Context, id uuid.UUID, user string) (err error) {
	defer func() { err = httpclient.ServiceError(ctx, entities.ErrReservationUnavailable, err) }()
	response, err := r.client.CancelReservationWithResponse(ctx, id, &contracts.CancelReservationParams{XUserName: user},
		func(_ context.Context, request *http.Request) error {
			request.Header.Set("Idempotency-Key", id.String())
			return nil
		})
	if err != nil {
		return oops.With("operation", "cancel reservation").Wrap(httpclient.ResponseReadError(err))
	}
	return oops.Wrap(httpclient.CheckStatus(response.HTTPResponse, http.StatusNoContent))
}
