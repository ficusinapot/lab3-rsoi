package loyalties

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/samber/oops"
	"go.uber.org/mock/gomock"
	"lab2/loyalty/internal/models/entities"
)

const getOperation = "get"

func TestGet(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Test Max", strings.Repeat("я", 80)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := NewMockRepository(gomock.NewController(t))
			want := entities.ForCount(25)
			repo.EXPECT().Get(gomock.Any(), name).Return(want, nil)
			got, err := New(repo).Get(t.Context(), name)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got != want {
				t.Errorf("loyalty = %+v, want %+v", got, want)
			}
		})
	}
}

func TestChange(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		delta, count int
	}{
		{name: "increase", delta: 1, count: 10},
		{name: "decrease", delta: -1, count: 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := NewMockRepository(gomock.NewController(t))
			want := entities.ForCount(tc.count)
			repo.EXPECT().Change(gomock.Any(), changeCommand(t, "Test Max", tc.delta, "")).Return(want, nil)
			got, err := New(repo).Change(t.Context(), "Test Max", tc.delta, "")
			if err != nil {
				t.Fatalf("Change: %v", err)
			}
			if got != want {
				t.Errorf("loyalty = %+v, want %+v", got, want)
			}
		})
	}
}

func TestRejectsInvalidUsername(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, username string }{
		{name: "empty", username: ""},
		{name: "whitespace", username: " \t "},
		{name: "invalid utf8", username: string([]byte{0xff})},
		{name: "too many characters", username: strings.Repeat("я", 81)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, operation := range []string{getOperation, "change"} {
				t.Run(operation, func(t *testing.T) {
					repo := NewMockRepository(gomock.NewController(t))
					service := New(repo)
					var err error
					if operation == getOperation {
						_, err = service.Get(t.Context(), tc.username)
					} else {
						_, err = service.Change(t.Context(), tc.username, 1, "")
					}
					if !errors.Is(err, entities.ErrInvalidInput) {
						t.Fatalf("error = %v, want invalid input", err)
					}
				})
			}
		})
	}
}

func TestChangeRejectsInvalidDelta(t *testing.T) {
	t.Parallel()
	for _, delta := range []int{-2, 0, 2} {
		t.Run(strconv.Itoa(delta), func(t *testing.T) {
			t.Parallel()
			repo := NewMockRepository(gomock.NewController(t))
			_, err := New(repo).Change(t.Context(), "Test Max", delta, "")
			if !errors.Is(err, entities.ErrInvalidInput) {
				t.Fatalf("delta %d: error = %v, want invalid input", delta, err)
			}
		})
	}
}

func TestRepositoryErrorPreserved(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{getOperation, "change"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			repo := NewMockRepository(gomock.NewController(t))
			cause := oops.Errorf("storage failed")
			service := New(repo)
			var err error
			if operation == getOperation {
				repo.EXPECT().Get(gomock.Any(), "Test Max").Return(entities.Loyalty{}, cause)
				_, err = service.Get(t.Context(), "Test Max")
			} else {
				repo.EXPECT().Change(gomock.Any(), changeCommand(t, "Test Max", 1, "")).Return(entities.Loyalty{}, cause)
				_, err = service.Change(t.Context(), "Test Max", 1, "")
			}
			if !errors.Is(err, cause) {
				t.Fatalf("error = %v, want wrapped %v", err, cause)
			}
		})
	}
}

func changeCommand(t *testing.T, user string, delta int, key string) entities.LoyaltyChange {
	t.Helper()
	command, err := entities.NewLoyaltyChange(user, delta, key)
	if err != nil {
		t.Fatal(err)
	}
	return command
}
