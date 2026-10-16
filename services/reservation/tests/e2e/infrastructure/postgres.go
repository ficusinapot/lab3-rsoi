//go:build e2e

package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

func newPostgres(
	t *testing.T,
	ctx context.Context,
	net *testcontainers.DockerNetwork,
) (testcontainers.Container, string) {
	t.Helper()
	database, err := testcontainers.Run(ctx, "postgres:13-alpine",
		network.WithNetwork([]string{"reservation-db"}, net),
		testcontainers.WithEnv(map[string]string{
			"POSTGRES_DB":       "reservations",
			"POSTGRES_USER":     "program",
			"POSTGRES_PASSWORD": "test",
		}),
		testcontainers.WithExposedPorts(postgresPort),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).
				WithStartupTimeout(2*time.Minute),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, database)
	host, err := database.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := database.MappedPort(ctx, postgresPort)
	if err != nil {
		t.Fatal(err)
	}
	return database, databaseURL(host, port.Port())
}
