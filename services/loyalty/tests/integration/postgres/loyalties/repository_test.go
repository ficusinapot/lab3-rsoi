//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sync"
	"testing"
	"time"
	"uuid"

	"lab2/loyalty/ent/operation"

	"lab2/loyalty/ent/loyalty"
	model "lab2/loyalty/internal/models/entities"
	storage "lab2/loyalty/internal/postgres/repos"
)

func TestLoyaltyRepository(t *testing.T) {
	t.Parallel()
	client, other := databaseClients(t)
	repositories := []*storage.Repository{storage.New(client), storage.New(other)}

	t.Run("new user starts at bronze", func(t *testing.T) {
		t.Parallel()
		item, err := repositories[0].Get(t.Context(), t.Name())
		if err != nil {
			t.Fatal(err)
		}
		want := model.Loyalty{Status: model.StatusBronze, Discount: 5, ReservationCount: 0}
		if item != want {
			t.Fatalf("loyalty = %+v, want %+v", item, want)
		}
		stored, err := repositories[1].Get(t.Context(), t.Name())
		if err != nil {
			t.Fatal(err)
		}
		if stored != want {
			t.Fatalf("stored = %+v, want %+v", stored, want)
		}
	})

	t.Run("concurrent creation produces one user", func(t *testing.T) {
		t.Parallel()
		var workers sync.WaitGroup
		type result struct {
			item model.Loyalty
			err  error
		}
		results := make(chan result, 20)
		want := model.Loyalty{Status: model.StatusBronze, Discount: 5, ReservationCount: 0}
		for index := range 20 {
			workers.Go(func() {
				item, err := repositories[index%2].Get(t.Context(), t.Name())
				results <- result{item: item, err: err}
			})
		}
		workers.Wait()
		for range 20 {
			got := <-results
			if got.err != nil {
				t.Fatal(got.err)
			}
			if got.item != want {
				t.Fatalf("concurrent Get = %+v, want %+v", got.item, want)
			}
		}
		count, err := client.Loyalty.Query().Where(loyalty.UsernameEQ(t.Name())).Count(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("user rows = %d, want 1", count)
		}
		item, err := repositories[1].Get(t.Context(), t.Name())
		if err != nil {
			t.Fatal(err)
		}
		if item != want {
			t.Fatalf("loyalty = %+v, want %+v", item, want)
		}
	})

	for _, tc := range []struct {
		name  string
		start int
		delta int
		want  model.Loyalty
	}{
		{
			name: "increase below silver", start: 8, delta: 1,
			want: model.Loyalty{Status: model.StatusBronze, Discount: 5, ReservationCount: 9},
		},
		{
			name: "increase to silver", start: 9, delta: 1,
			want: model.Loyalty{Status: model.StatusSilver, Discount: 7, ReservationCount: 10},
		},
		{
			name: "increase below gold", start: 18, delta: 1,
			want: model.Loyalty{Status: model.StatusSilver, Discount: 7, ReservationCount: 19},
		},
		{
			name: "increase to gold", start: 19, delta: 1,
			want: model.Loyalty{Status: model.StatusGold, Discount: 10, ReservationCount: 20},
		},
		{
			name: "decrease within gold", start: 21, delta: -1,
			want: model.Loyalty{Status: model.StatusGold, Discount: 10, ReservationCount: 20},
		},
		{
			name: "decrease to silver", start: 20, delta: -1,
			want: model.Loyalty{Status: model.StatusSilver, Discount: 7, ReservationCount: 19},
		},
		{
			name: "decrease within silver", start: 11, delta: -1,
			want: model.Loyalty{Status: model.StatusSilver, Discount: 7, ReservationCount: 10},
		},
		{
			name: "decrease to bronze", start: 10, delta: -1,
			want: model.Loyalty{Status: model.StatusBronze, Discount: 5, ReservationCount: 9},
		},
		{
			name: "decrease to zero", start: 1, delta: -1,
			want: model.Loyalty{Status: model.StatusBronze, Discount: 5, ReservationCount: 0},
		},
		{
			name: "count cannot become negative", start: 0, delta: -1,
			want: model.Loyalty{Status: model.StatusBronze, Discount: 5, ReservationCount: 0},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for range tc.start {
				if _, err := repositories[0].Change(t.Context(), changeCommand(t, t.Name(), 1, "")); err != nil {
					t.Fatal(err)
				}
			}
			item, err := repositories[1].Change(t.Context(), changeCommand(t, t.Name(), tc.delta, ""))
			if err != nil {
				t.Fatal(err)
			}
			if item != tc.want {
				t.Fatalf("loyalty = %+v, want %+v", item, tc.want)
			}
			stored, err := repositories[0].Get(t.Context(), t.Name())
			if err != nil {
				t.Fatal(err)
			}
			if stored != tc.want {
				t.Fatalf("stored = %+v, want %+v", stored, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name  string
		start int
		mixed bool
	}{
		{name: "concurrent increments", start: 0},
		{name: "concurrent increments and decrements", start: 40, mixed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for range tc.start {
				if _, err := repositories[0].Change(t.Context(), changeCommand(t, t.Name(), 1, "")); err != nil {
					t.Fatal(err)
				}
			}
			var workers sync.WaitGroup
			results := make(chan error, 40)
			for index := range 40 {
				delta := 1
				if tc.mixed && index%2 == 0 {
					delta = -1
				}
				workers.Go(func() {
					_, err := repositories[index%2].Change(t.Context(), changeCommand(t, t.Name(), delta, ""))
					results <- err
				})
			}
			workers.Wait()
			for range 40 {
				if err := <-results; err != nil {
					t.Fatal(err)
				}
			}
			item, err := repositories[1].Get(t.Context(), t.Name())
			if err != nil {
				t.Fatal(err)
			}
			want := model.Loyalty{Status: model.StatusGold, Discount: 10, ReservationCount: 40}
			if item != want {
				t.Fatalf("loyalty = %+v, want %+v", item, want)
			}
		})
	}

	t.Run("locked row times out without changing count", func(t *testing.T) {
		t.Parallel()
		if _, err := repositories[0].Change(t.Context(), changeCommand(t, t.Name(), 1, "")); err != nil {
			t.Fatal(err)
		}
		tx, err := client.Tx(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("rollback: %v", err)
			}
		})
		if _, err := tx.Loyalty.Query().Where(loyalty.UsernameEQ(t.Name())).ForUpdate().Only(t.Context()); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		_, err = repositories[1].Change(ctx, changeCommand(t, t.Name(), 1, ""))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Change error = %v, want context.DeadlineExceeded", err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		item, err := repositories[1].Get(t.Context(), t.Name())
		if err != nil {
			t.Fatal(err)
		}
		want := model.Loyalty{Status: model.StatusBronze, Discount: 5, ReservationCount: 1}
		if item != want {
			t.Fatalf("stored = %+v, want %+v", item, want)
		}
	})
}

func TestOperationIdempotency(t *testing.T) {
	t.Parallel()
	client, other := databaseClients(t)
	first, second := storage.New(client), storage.New(other)
	name, key := t.Name(), t.Name()+":increment"
	var workers sync.WaitGroup
	failures := make(chan error, 20)
	for index := range 20 {
		workers.Go(func() {
			repo := first
			if index%2 == 0 {
				repo = second
			}
			_, err := repo.Change(t.Context(), changeCommand(t, name, 1, key))
			failures <- err
		})
	}
	workers.Wait()
	for range 20 {
		if err := <-failures; err != nil {
			t.Fatal(err)
		}
	}
	item, err := second.Get(t.Context(), name)
	if err != nil || item.ReservationCount != 1 {
		t.Fatal(item, err)
	}
	// A new repository/client still sees the durable key.
	if _, err := second.Change(t.Context(), changeCommand(t, name, 1, key)); err != nil {
		t.Fatal(err)
	}
	for _, conflict := range []struct {
		user  string
		delta int
	}{
		{user: name, delta: -1}, {user: name + " other", delta: 1},
	} {
		_, changeErr := second.Change(t.Context(), changeCommand(t, conflict.user, conflict.delta, key))
		if !errors.Is(changeErr, model.ErrInvalidInput) {
			t.Fatalf("conflicting key error = %v", changeErr)
		}
	}
	item, err = first.Get(t.Context(), name)
	if err != nil || item.ReservationCount != 1 {
		t.Fatal(item, err)
	}
	if _, err := first.Change(t.Context(), changeCommand(t, name, -1, name+":decrement")); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Change(t.Context(), changeCommand(t, name, -1, name+":decrement")); err != nil {
		t.Fatal(err)
	}
	item, err = first.Get(t.Context(), name)
	if err != nil || item.ReservationCount != 0 {
		t.Fatal(item, err)
	}
}

func TestOperationRollsBackWithCounter(t *testing.T) {
	t.Parallel()
	client, _ := databaseClients(t)
	repo := storage.New(client)
	name, key := t.Name(), t.Name()+":overflow"
	if _, err := repo.Get(t.Context(), name); err != nil {
		t.Fatal(err)
	}
	if err := client.Loyalty.Update().Where(loyalty.UsernameEQ(name)).
		SetReservationCount(math.MaxInt32).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Change(t.Context(), changeCommand(t, name, 1, key)); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal(err)
	}
	count, err := client.Operation.Query().Where(operation.OperationIDEQ(key)).Count(t.Context())
	if err != nil || count != 0 {
		t.Fatalf("rolled back keys = %d, err = %v", count, err)
	}
	if err := client.Loyalty.Update().Where(loyalty.UsernameEQ(name)).
		SetReservationCount(0).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	item, err := repo.Change(t.Context(), changeCommand(t, name, 1, key))
	if err != nil || item.ReservationCount != 1 {
		t.Fatal(item, err)
	}
}

func changeCommand(t *testing.T, user string, delta int, key string) model.LoyaltyChange {
	t.Helper()
	command, err := model.NewLoyaltyChange(user, delta, key)
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func TestCreationResolutionSerializesWithIncrement(t *testing.T) {
	t.Parallel()
	client, other := databaseClients(t)
	first, second := storage.New(client), storage.New(other)
	user := "race-" + uuid.New().String()
	committedCount := 0
	for range 20 {
		id := uuid.New()
		command := changeCommand(t, user, 1, id.String()+":create")
		resolution := model.CreationResolution{User: user, ReservationUID: id}
		gate := make(chan struct{})
		changed := make(chan error, 1)
		resolved := make(chan struct {
			committed bool
			err       error
		}, 1)
		go func() { <-gate; _, err := first.Change(t.Context(), command); changed <- err }()
		go func() {
			<-gate
			committed, err := second.ResolveCreation(t.Context(), resolution)
			resolved <- struct {
				committed bool
				err       error
			}{committed, err}
		}()
		close(gate)
		decision, changeErr := <-resolved, <-changed
		if decision.err != nil {
			t.Fatal(decision.err)
		}
		if decision.committed {
			committedCount++
			if changeErr != nil {
				t.Fatal(changeErr)
			}
		} else if !errors.Is(changeErr, model.ErrInvalidInput) {
			t.Fatalf("aborted increment error = %v", changeErr)
		}
		repeated, err := first.ResolveCreation(t.Context(), resolution)
		if repeated != decision.committed || err != nil {
			t.Fatal(repeated, err)
		}
	}
	got, err := second.Get(t.Context(), user)
	if err != nil || got.ReservationCount != committedCount {
		t.Fatal(got, committedCount, err)
	}
}

func TestRollbackWithoutIncrementPreservesExistingCount(t *testing.T) {
	t.Parallel()
	client, other := databaseClients(t)
	first, second := storage.New(client), storage.New(other)
	user := "rollback-" + uuid.New().String()
	if _, err := first.Change(t.Context(), changeCommand(t, user, 1, "")); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	resolution := model.CreationResolution{User: user, ReservationUID: id}
	committed, err := second.ResolveCreation(t.Context(), resolution)
	if committed || err != nil {
		t.Fatal(committed, err)
	}
	rollback := changeCommand(t, user, -1, id.String()+":rollback")
	for _, repo := range []*storage.Repository{first, second} {
		got, err := repo.Change(t.Context(), rollback)
		if err != nil || got.ReservationCount != 1 {
			t.Fatal(got, err)
		}
	}
	_, changeErr := first.Change(t.Context(), changeCommand(t, user, 1, id.String()+":create"))
	if !errors.Is(changeErr, model.ErrInvalidInput) {
		t.Fatal("late increment accepted", changeErr)
	}
}
