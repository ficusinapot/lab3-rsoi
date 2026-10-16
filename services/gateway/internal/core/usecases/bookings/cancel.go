package bookings

import (
	"context"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
	"lab2/platform/observability/metrics"
)

func (s *Commands) Cancel(ctx context.Context, input entities.CancelBooking) (err error) {
	id, user := input.ReservationUID, input.User
	defer metrics.Instrument(metrics.PreInstrument(ctx, s.Cancel), &err)
	if err = validateUser(user); err != nil {
		return oops.Wrap(err)
	}
	return oops.Wrap(s.groups.Cancel(ctx, user, id))
}
