package payments

import (
	"context"
	"math"
	"uuid"

	"github.com/samber/oops"
	"lab2/payment/internal/models/dbifc"
	"lab2/payment/internal/models/entities"
)

type Service struct {
	repo dbifc.Payments
}

func New(repo dbifc.Payments) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, price int) (entities.Payment, error) {
	return s.CreateWithID(ctx, uuid.New(), price)
}

func (s *Service) CreateWithID(ctx context.Context, id uuid.UUID, price int) (entities.Payment, error) {
	if id == uuid.Nil() || price < 0 || price > math.MaxInt32 {
		return entities.Payment{}, oops.Wrap(entities.ErrInvalidInput)
	}
	item, err := s.repo.Create(ctx, id, price)
	return item, oops.Wrapf(err, "create payment")
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (entities.Payment, error) {
	item, err := s.repo.Get(ctx, id)
	return item, oops.Wrapf(err, "get payment")
}

func (s *Service) Cancel(ctx context.Context, id uuid.UUID) error {
	return oops.Wrapf(s.repo.Cancel(ctx, id), "cancel payment")
}

func (s *Service) CancelKnown(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil() {
		return oops.Wrap(entities.ErrInvalidInput)
	}
	return oops.Wrapf(s.repo.CancelKnown(ctx, id), "cancel known payment")
}
