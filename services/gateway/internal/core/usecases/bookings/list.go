package bookings

import (
	"context"
	"sync"
	"uuid"

	"github.com/samber/oops"
	"golang.org/x/sync/errgroup"
	"lab2/gateway/internal/models/entities"
)

func (s *Queries) List(ctx context.Context, user string) ([]entities.Reservation, error) {
	if err := validateUser(user); err != nil {
		return nil, err
	}
	rows, err := s.reservations.Reservations(ctx, user)
	if err != nil {
		return nil, oops.Wrapf(err, "list reservations")
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(s.concurrency)
	// Each request shares one completed hotel read per UUID, including its error.
	hotels := make(map[uuid.UUID]func() (entities.Hotel, error))
	for _, row := range rows {
		if _, exists := hotels[row.HotelUID]; exists {
			continue
		}
		hotels[row.HotelUID] = sync.OnceValues(func() (entities.Hotel, error) {
			return s.reservations.Hotel(groupCtx, row.HotelUID)
		})
	}
	items := make([]entities.Reservation, len(rows))
	for index, row := range rows {
		if groupCtx.Err() != nil {
			break
		}
		group.Go(func() error {
			if err := groupCtx.Err(); err != nil {
				return oops.Wrap(err)
			}
			hotel, err := hotels[row.HotelUID]()
			if err != nil {
				return oops.Wrapf(err, "get hotel")
			}
			item, err := s.reservationWithHotel(groupCtx, row, hotel)
			if err != nil {
				return err
			}
			items[index] = item
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, oops.Wrap(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, oops.Wrapf(err, "enrich reservations")
	}
	return items, nil
}
