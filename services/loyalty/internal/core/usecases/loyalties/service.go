package loyalties

import (
	"context"
	"strings"
	"unicode/utf8"
	"uuid"

	"lab2/loyalty/internal/models/dbifc"
	"lab2/loyalty/internal/models/entities"

	"github.com/samber/oops"
)

type Service struct {
	repo dbifc.Loyalties
}

func New(repo dbifc.Loyalties) *Service {
	return &Service{repo: repo}
}

func validate(name string) error {
	if strings.TrimSpace(name) == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 80 {
		return oops.Wrap(entities.ErrInvalidInput)
	}
	return nil
}

func (s *Service) Get(ctx context.Context, name string) (entities.Loyalty, error) {
	if err := validate(name); err != nil {
		return entities.Loyalty{}, oops.Wrap(err)
	}
	item, err := s.repo.Get(ctx, name)
	return item, oops.Wrapf(err, "get loyalty")
}

func (s *Service) Change(ctx context.Context, name string, delta int, operationID string) (entities.Loyalty, error) {
	if err := validate(name); err != nil {
		return entities.Loyalty{}, oops.Wrap(err)
	}
	command, err := entities.NewLoyaltyChange(name, delta, operationID)
	if err != nil {
		return entities.Loyalty{}, oops.Wrap(err)
	}
	item, err := s.repo.Change(ctx, command)
	return item, oops.Wrapf(err, "change loyalty")
}

func (s *Service) ResolveCreation(ctx context.Context, name string, id uuid.UUID) (bool, error) {
	if err := validate(name); err != nil {
		return false, oops.Wrap(err)
	}
	if id == uuid.Nil() {
		return false, oops.Wrap(entities.ErrInvalidInput)
	}
	committed, err := s.repo.ResolveCreation(ctx, entities.CreationResolution{User: name, ReservationUID: id})
	return committed, oops.Wrapf(err, "resolve booking creation")
}
