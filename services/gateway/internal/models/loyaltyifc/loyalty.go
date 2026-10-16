package loyaltyifc

import (
	"context"
	"uuid"

	"lab2/gateway/internal/models/entities"
)

type Updater interface {
	ResolveCreation(context.Context, string, uuid.UUID) (bool, error)
	ChangeLoyalty(context.Context, string, int, string) error
}

type Reader interface {
	Loyalty(context.Context, string) (entities.Loyalty, error)
}

type Client interface {
	Reader
	Updater
}

type BookingSource interface {
	Reader
	ChangeLoyalty(context.Context, string, int, string) error
}
