package entities

import (
	"math"
	"strings"
	"unicode/utf8"
	"uuid"

	"github.com/samber/oops"
)

type ChangeAction uint8

const (
	ChangeUnknown ChangeAction = iota
	ChangeIncrease
	ChangeDecrease
	ChangeCreation
	ChangeRollback
)

type LoyaltyChange struct {
	User   string
	Delta  int
	Key    string
	Action ChangeAction
}

// NewLoyaltyChange preserves unkeyed requests and opaque keys from lab2.
func NewLoyaltyChange(user string, delta int, key string) (LoyaltyChange, error) {
	if (delta != 1 && delta != -1) || len(key) > 128 || !utf8.ValidString(key) ||
		(key != "" && strings.TrimSpace(key) == "") {
		return LoyaltyChange{}, oops.Wrap(ErrInvalidInput)
	}
	action := ChangeIncrease
	if delta == -1 {
		action = ChangeDecrease
	}
	if delta == 1 && strings.HasSuffix(key, ":create") {
		action = ChangeCreation
	}
	if delta == -1 && strings.HasSuffix(key, ":rollback") {
		action = ChangeRollback
	}
	return LoyaltyChange{User: user, Delta: delta, Key: key, Action: action}, nil
}

func (c LoyaltyChange) RelatedKey() string {
	switch c.Action {
	case ChangeCreation:
		return strings.TrimSuffix(c.Key, ":create") + ":rollback"
	case ChangeRollback:
		return strings.TrimSuffix(c.Key, ":rollback") + ":create"
	case ChangeUnknown, ChangeIncrease, ChangeDecrease:
		return ""
	default:
		return ""
	}
}

type ChangeRecord struct {
	User  string
	Delta int
}

func (r ChangeRecord) Validate(user string, delta int) error {
	if r.User != user || r.Delta != delta {
		return oops.Wrap(ErrInvalidInput)
	}
	return nil
}

// Decide is evaluated against records loaded under the user's transaction lock.
func (c LoyaltyChange) Decide(current Loyalty, applied, related *ChangeRecord) (Loyalty, error) {
	expected, err := NewLoyaltyChange(c.User, c.Delta, c.Key)
	if err != nil {
		return Loyalty{}, err
	}
	if expected.Action != c.Action {
		return Loyalty{}, oops.Wrap(ErrInvalidInput)
	}
	if c.Action == ChangeCreation && related != nil {
		return Loyalty{}, oops.Wrap(ErrInvalidInput)
	}
	delta := c.Delta
	if c.Action == ChangeRollback {
		if related == nil {
			delta = 0
		} else if err := related.Validate(c.User, 1); err != nil {
			return Loyalty{}, err
		}
	}
	if applied != nil {
		if err := applied.Validate(c.User, c.Delta); err != nil {
			return Loyalty{}, err
		}
		return current, nil
	}
	if current.ReservationCount == math.MaxInt32 && delta > 0 {
		return Loyalty{}, oops.Wrap(ErrInvalidInput)
	}
	return ForCount(max(0, current.ReservationCount+delta)), nil
}

type CreationResolution struct {
	User           string
	ReservationUID uuid.UUID
}

func (c CreationResolution) CreationKey() string { return c.ReservationUID.String() + ":create" }

func (c CreationResolution) RollbackKey() string { return c.ReservationUID.String() + ":rollback" }

func (c CreationResolution) Decide(applied *ChangeRecord) (bool, error) {
	if c.ReservationUID == uuid.Nil() {
		return false, oops.Wrap(ErrInvalidInput)
	}
	if applied == nil {
		return false, nil
	}
	if err := applied.Validate(c.User, 1); err != nil {
		return false, err
	}
	return true, nil
}
