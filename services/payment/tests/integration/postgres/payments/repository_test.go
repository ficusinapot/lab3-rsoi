//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"uuid"

	model "lab2/payment/internal/models/entities"
	"lab2/payment/internal/postgres/repos"
)

func TestPaymentRepository(t *testing.T) {
	t.Parallel()
	client, other := databaseClients(t)
	repository := repos.New(client)
	reader := repos.New(other)

	t.Run("create and read through another pool", func(t *testing.T) {
		t.Parallel()
		id := uuid.New()
		want := model.Payment{PaymentUID: id, Status: model.StatusPaid, Price: 28500}
		created, err := repository.Create(t.Context(), id, want.Price)
		if err != nil {
			t.Fatal(err)
		}
		if created != want {
			t.Fatalf("created = %+v, want %+v", created, want)
		}
		stored, err := reader.Get(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if stored != want {
			t.Fatalf("stored = %+v, want %+v", stored, want)
		}
	})

	for _, tc := range []struct {
		name  string
		calls int
	}{
		{name: "cancel", calls: 1},
		{name: "cancel again", calls: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id := uuid.New()
			if _, err := repository.Create(t.Context(), id, 28500); err != nil {
				t.Fatal(err)
			}
			for range tc.calls {
				if err := repository.Cancel(t.Context(), id); err != nil {
					t.Fatal(err)
				}
			}
			stored, err := reader.Get(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			want := model.Payment{PaymentUID: id, Status: model.StatusCanceled, Price: 28500}
			if stored != want {
				t.Fatalf("stored = %+v, want %+v", stored, want)
			}
		})
	}

	t.Run("read unknown payment", func(t *testing.T) {
		t.Parallel()
		if _, err := repository.Get(t.Context(), uuid.New()); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("Get error = %v, want ErrNotFound", err)
		}
	})

	t.Run("cancel unknown payment", func(t *testing.T) {
		t.Parallel()
		if err := repository.Cancel(t.Context(), uuid.New()); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("Cancel error = %v, want ErrNotFound", err)
		}
	})

	t.Run("read with canceled context", func(t *testing.T) {
		t.Parallel()
		id := uuid.New()
		if _, err := repository.Create(t.Context(), id, 28500); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := repository.Get(ctx, id); !errors.Is(err, context.Canceled) {
			t.Fatalf("Get error = %v, want context.Canceled", err)
		}
	})
}
