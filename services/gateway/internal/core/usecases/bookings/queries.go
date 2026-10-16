package bookings

import (
	"lab2/gateway/internal/models/coreifc"
	"lab2/gateway/internal/models/loyaltyifc"
	"lab2/gateway/internal/models/paymentifc"
	"lab2/gateway/internal/models/reservationifc"
)

type Queries struct {
	reservations reservationifc.Reader
	payments     paymentifc.Reader
	loyalty      loyaltyifc.Reader
	concurrency  int
}

var _ coreifc.BookingQueries = (*Queries)(nil)

func NewQueries(
	reservations reservationifc.Reader, payments paymentifc.Reader, loyalty loyaltyifc.Reader, concurrency int,
) *Queries {
	return &Queries{reservations: reservations, payments: payments, loyalty: loyalty, concurrency: concurrency}
}
