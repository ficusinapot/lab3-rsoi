package payments

import (
	"context"
	"errors"
	"math"
	"testing"
	"uuid"

	"github.com/samber/oops"
	"go.uber.org/mock/gomock"
	"lab2/payment/internal/models/entities"
)

func TestCreate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		price int
	}{
		{name: "zero price", price: 0},
		{name: "booking price", price: 27000},
		{name: "maximum price", price: math.MaxInt32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := NewMockRepository(gomock.NewController(t))
			var want entities.Payment
			repo.EXPECT().Create(gomock.Any(), gomock.Any(), tc.price).DoAndReturn(
				func(_ context.Context, id uuid.UUID, price int) (entities.Payment, error) {
					if id == uuid.Nil() {
						t.Fatal("created payment has a nil uuid")
					}
					want = entities.Payment{PaymentUID: id, Price: price, Status: entities.StatusPaid}
					return want, nil
				})
			got, err := New(repo).Create(t.Context(), tc.price)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if got != want {
				t.Errorf("payment = %+v, want %+v", got, want)
			}
		})
	}
}

func TestCreateRejectsInvalidPrice(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		price int
	}{
		{name: "negative price", price: -1},
		{name: "price exceeds storage range", price: math.MaxInt32 + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := NewMockRepository(gomock.NewController(t))
			_, err := New(repo).Create(t.Context(), tc.price)
			if !errors.Is(err, entities.ErrInvalidInput) {
				t.Fatalf("error = %v, want invalid input", err)
			}
		})
	}
}

func TestGet(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	want := entities.Payment{PaymentUID: id, Price: 27000, Status: entities.StatusPaid}
	repo := NewMockRepository(gomock.NewController(t))
	repo.EXPECT().Get(gomock.Any(), id).Return(want, nil)
	got, err := New(repo).Get(t.Context(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("payment = %+v, want %+v", got, want)
	}
}

func TestCancel(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	repo := NewMockRepository(gomock.NewController(t))
	repo.EXPECT().Cancel(gomock.Any(), id).Return(nil)
	if err := New(repo).Cancel(t.Context(), id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
}

func TestRepositoryErrorPreserved(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"create", "get", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			repo := NewMockRepository(gomock.NewController(t))
			cause := oops.Errorf("storage failed")
			service := New(repo)
			id := uuid.New()
			var err error
			switch operation {
			case "create":
				repo.EXPECT().Create(gomock.Any(), gomock.Any(), 1).Return(entities.Payment{}, cause)
				_, err = service.Create(t.Context(), 1)
			case "get":
				repo.EXPECT().Get(gomock.Any(), id).Return(entities.Payment{}, cause)
				_, err = service.Get(t.Context(), id)
			case "cancel":
				repo.EXPECT().Cancel(gomock.Any(), id).Return(cause)
				err = service.Cancel(t.Context(), id)
			}
			if !errors.Is(err, cause) {
				t.Fatalf("error = %v, want wrapped %v", err, cause)
			}
		})
	}
}
