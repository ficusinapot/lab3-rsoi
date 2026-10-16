package bookings

import (
	"lab2/gateway/internal/models/coreifc"
	"lab2/gateway/internal/models/loyaltyifc"
	"lab2/gateway/internal/models/operationifc"
	"lab2/gateway/internal/models/reservationifc"
)

type Commands struct {
	reservations reservationifc.BookingSource
	loyalty      loyaltyifc.BookingSource
	groups       operationifc.Groups
}

var _ coreifc.BookingCommands = (*Commands)(nil)

func NewCommands(
	reservations reservationifc.BookingSource,
	loyalty loyaltyifc.BookingSource,
	groups operationifc.Groups,
) *Commands {
	return &Commands{
		reservations: reservations, loyalty: loyalty,
		groups: groups,
	}
}
