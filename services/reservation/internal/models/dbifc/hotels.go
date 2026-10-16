package dbifc

import (
	"context"
	"uuid"

	"lab2/reservation/internal/models/entities"
)

type Hotels interface {
	List(context.Context, int, int) (entities.Page, error)
	Get(context.Context, uuid.UUID) (entities.Hotel, error)
}

//go:generate go tool mockgen -source=hotels.go -destination=../../core/usecases/hotels/repository_mock_test.go -package=hotels Hotels
