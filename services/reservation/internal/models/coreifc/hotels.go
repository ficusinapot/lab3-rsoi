package coreifc

import (
	"context"
	"uuid"

	"lab2/reservation/internal/models/entities"
)

//go:generate go tool mockgen -source=hotels.go -destination=../../rest/hotels_mock_test.go -package=resttransport Hotels

type Hotels interface {
	List(context.Context, entities.PageQuery) (entities.Page, error)
	Get(context.Context, uuid.UUID) (entities.Hotel, error)
}
