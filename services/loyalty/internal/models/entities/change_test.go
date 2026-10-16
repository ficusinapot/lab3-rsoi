package entities

import (
	"errors"
	"math"
	"testing"
	"uuid"
)

const (
	changeUser  = "user"
	rollbackKey = "booking:rollback"
)

func TestLoyaltyChangeDecisions(t *testing.T) {
	t.Parallel()
	user := changeUser
	for _, tc := range []struct {
		name             string
		delta            int
		key              string
		count            int
		applied, related *ChangeRecord
		want             int
		invalid          bool
	}{
		{name: "legacy increase", delta: 1, count: 9, want: 10},
		{name: "legacy decrease", delta: -1, count: 10, want: 9},
		{name: "zero boundary", delta: -1, want: 0},
		{name: "creation", delta: 1, key: "booking:create", count: 1, want: 2},
		{
			name: "creation after abort", delta: 1, key: "booking:create",
			related: &ChangeRecord{User: user, Delta: -1}, invalid: true,
		},
		{name: "rollback without increment", delta: -1, key: rollbackKey, count: 5, want: 5},
		{
			name: "rollback after increment", delta: -1, key: rollbackKey, count: 5,
			related: &ChangeRecord{User: user, Delta: 1}, want: 4,
		},
		{
			name: "foreign increment", delta: -1, key: rollbackKey, count: 5,
			related: &ChangeRecord{User: "other", Delta: 1}, invalid: true,
		},
		{
			name: "repeated decrement", delta: -1, key: "booking:cancel", count: 5,
			applied: &ChangeRecord{User: user, Delta: -1}, want: 5,
		},
		{
			name: "key content conflict", delta: -1, key: "booking:cancel", count: 5,
			applied: &ChangeRecord{User: user, Delta: 1}, invalid: true,
		},
		{name: "count overflow", delta: 1, count: math.MaxInt32, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			command, err := NewLoyaltyChange(user, tc.delta, tc.key)
			if err != nil {
				t.Fatal(err)
			}
			got, err := command.Decide(ForCount(tc.count), tc.applied, tc.related)
			if tc.invalid {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("error = %v", err)
				}
			} else if err != nil || got != ForCount(tc.want) {
				t.Fatalf("decision = %+v, error = %v, want count %d", got, err, tc.want)
			}
		})
	}
}

func TestCreationResolutionDecisions(t *testing.T) {
	t.Parallel()
	command := CreationResolution{User: changeUser, ReservationUID: uuid.New()}
	for _, tc := range []struct {
		name               string
		applied            *ChangeRecord
		committed, invalid bool
	}{
		{name: "absent increment"},
		{name: "committed increment", applied: &ChangeRecord{User: changeUser, Delta: 1}, committed: true},
		{name: "foreign record", applied: &ChangeRecord{User: "other", Delta: 1}, invalid: true},
		{name: "wrong action", applied: &ChangeRecord{User: changeUser, Delta: -1}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			committed, err := command.Decide(tc.applied)
			if committed != tc.committed || errors.Is(err, ErrInvalidInput) != tc.invalid {
				t.Fatalf("decision = %v, error = %v", committed, err)
			}
		})
	}
}
