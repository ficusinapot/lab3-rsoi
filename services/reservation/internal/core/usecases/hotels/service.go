package hotels

import (
	"context"
	"uuid"

	"github.com/samber/oops"
	"lab2/reservation/internal/models/dbifc"
	"lab2/reservation/internal/models/entities"
)

type PaginationConfig struct {
	DefaultPage int `mapstructure:"default_page"`
	DefaultSize int `mapstructure:"default_size"`
	MaxSize     int `mapstructure:"max_size"`
}

func (c PaginationConfig) Validate() error {
	if c.DefaultPage < 1 || c.DefaultSize < 1 {
		return oops.Errorf("pagination defaults must be positive")
	}
	offsetOverflow := int64(c.DefaultPage-1) > (1<<31-1)/int64(c.DefaultSize)
	if c.MaxSize < c.DefaultSize || offsetOverflow {
		return oops.In("configuration").
			Code("invalid_pagination").Errorf("invalid pagination configuration")
	}
	return nil
}

type Service struct {
	repo       dbifc.Hotels
	pagination PaginationConfig
}

func New(repo dbifc.Hotels, cfg PaginationConfig) *Service {
	return &Service{repo: repo, pagination: cfg}
}

func (s *Service) List(ctx context.Context, query entities.PageQuery) (entities.Page, error) {
	page, size := s.pagination.DefaultPage, s.pagination.DefaultSize
	if query.Page != nil {
		page = *query.Page
	}
	if query.Size != nil {
		size = *query.Size
	}
	if page < 1 || size < 1 {
		return entities.Page{}, oops.In("validation").
			Code("invalid_input").
			Public("invalid page or size").
			Wrap(entities.ErrInvalidInput)
	}
	offsetOverflow := int64(page-1) > (1<<31-1)/int64(size)
	if size > s.pagination.MaxSize || offsetOverflow {
		return entities.Page{}, oops.In("validation").
			Code("invalid_input").
			Public("invalid page or size").
			Wrap(entities.ErrInvalidInput)
	}
	result, err := s.repo.List(ctx, page, size)
	return result, oops.FromContext(ctx).
		In("hotels.usecase").
		With("operation", "list").
		Wrapf(err, "operation failed")
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (entities.Hotel, error) {
	result, err := s.repo.Get(ctx, id)
	return result, oops.FromContext(ctx).
		In("hotels.usecase").
		With("operation", "get").
		Wrapf(err, "operation failed")
}
