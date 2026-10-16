//go:build integration

package integration

import (
	"strings"
	"testing"
	"uuid"

	storage "lab2/loyalty/internal/postgres/repos"
)

func TestUnicodeUsername(t *testing.T) {
	t.Parallel()
	client, other := databaseClients(t)
	repo := storage.New(client)
	user := strings.Repeat("я", 44) + uuid.New().String()
	if _, err := repo.Get(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Change(t.Context(), changeCommand(t, user, 1, uuid.New().String())); err != nil {
		t.Fatal(err)
	}
	got, err := storage.New(other).Get(t.Context(), user)
	if err != nil || got.ReservationCount != 1 {
		t.Fatalf("stored Unicode user: %+v, error: %v", got, err)
	}
}
