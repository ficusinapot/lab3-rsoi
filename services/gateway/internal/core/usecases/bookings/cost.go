package bookings

import (
	"math"

	"lab2/gateway/internal/models/entities"

	"github.com/samber/oops"
)

func bookingNights(input entities.CreateBooking) (int64, error) {
	start, end := input.StartDate, input.EndDate
	if start.IsZero() || !end.After(start) {
		return 0, oops.Wrap(entities.ErrInvalidInput)
	}
	return (end.Unix() - start.Unix()) / 86400, nil
}

func cost(nights int64, price, discount int) (int, error) {
	if nights <= 0 || price < 0 || discount < 0 || discount > 100 {
		return 0, oops.Wrap(entities.ErrInvalidInput)
	}
	factor := int64(100 - discount)
	if price > 0 && nights > math.MaxInt64/int64(price) {
		return 0, oops.Wrap(entities.ErrInvalidInput)
	}
	base := nights * int64(price)
	if factor > 0 && base > math.MaxInt64/factor {
		return 0, oops.Wrap(entities.ErrInvalidInput)
	}
	total := base * factor / 100
	if total > math.MaxInt32 {
		return 0, oops.Wrap(entities.ErrInvalidInput)
	}
	return int(total), nil
}
