//go:build integration

package integration

import (
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"lab2/loyalty/ent"
	"lab2/loyalty/internal/postgres"
	"lab2/platform/config"
	"lab2/platform/database"
)

func databaseClients(t *testing.T) (*ent.Client, *ent.Client) {
	t.Helper()
	path := os.Getenv("TEST_DATABASE_CONFIG")
	if path == "" {
		t.Fatal("TEST_DATABASE_CONFIG is not set; run make integration-test")
	}
	cfg, err := config.Load[database.Config](path)
	if err != nil {
		t.Fatal(err)
	}
	clients := make([]*ent.Client, 2)
	for index := range clients {
		db, err := database.Open(t.Context(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		client := postgres.NewClient(db)
		t.Cleanup(func() {
			if err := client.Close(); err != nil {
				t.Errorf("close Ent client: %v", err)
			}
		})
		clients[index] = client
	}
	return clients[0], clients[1]
}
