//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/go-connections/nat"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const loyaltyServiceName = "loyalty"

type stack struct {
	gateway    string
	redisURL   string
	urls       []string
	db         *sql.DB
	containers map[string]testcontainers.Container
	addresses  map[string]string
}

func newStack(t *testing.T) *stack {
	t.Helper()
	ctx := t.Context()
	root := os.Getenv("GATEWAY_PROJECT_ROOT")
	if root == "" {
		t.Fatal("GATEWAY_PROJECT_ROOT is required")
	}
	nw, err := network.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := nw.Remove(ctx); err != nil {
			t.Error(err)
		}
	})
	redisContainer, err := testcontainers.Run(ctx, "redis:7.4-alpine",
		network.WithNetwork([]string{"operation-redis"}, nw),
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("Ready to accept connections")))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, redisContainer)
	dbContainer, err := testcontainers.Run(ctx, "postgres:13-alpine",
		network.WithNetwork([]string{"reservation-db"}, nw),
		testcontainers.WithEnv(map[string]string{
			"POSTGRES_DB": "reservations", "POSTGRES_USER": "program", "POSTGRES_PASSWORD": "test",
		}),
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).
			WithStartupTimeout(2*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, dbContainer)
	host, err := dbContainer.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := dbContainer.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatal(err)
	}
	dsn := "postgres://program:test@" + net.JoinHostPort(host, port.Port()) + "/reservations?sslmode=disable"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, name := range []string{"payments", "loyalties"} {
		// Database names are fixed test constants, never request input.
		if _, err := db.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
			t.Fatal(err)
		}
	}
	out := &stack{db: db, containers: make(map[string]testcontainers.Container), addresses: make(map[string]string)}
	redisHost, err := redisContainer.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	redisPort, err := redisContainer.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatal(err)
	}
	out.redisURL = "redis://" + net.JoinHostPort(redisHost, redisPort.Port()) + "/0"

	for _, svc := range []struct{ name, port string }{
		{name: reservationServiceName, port: "8070/tcp"},
		{name: paymentServiceName, port: "8060/tcp"},
		{name: loyaltyServiceName, port: "8050/tcp"},
		{name: "gateway", port: "8080/tcp"},
	} {
		var buildLog bytes.Buffer
		image := testcontainers.ContainerCustomizer(testcontainers.WithDockerfile(testcontainers.FromDockerfile{
			Context:    root,
			Dockerfile: filepath.Join("services", svc.name, "Dockerfile"), KeepImage: true, BuildLogWriter: &buildLog,
			BuildOptionsModifier: func(opts *build.ImageBuildOptions) { opts.Version = build.BuilderBuildKit },
		}))
		if os.Getenv("GATEWAY_E2E_PREBUILT") == "1" {
			image = testcontainers.WithImage("lab3-" + svc.name)
		}
		container, err := testcontainers.Run(ctx, "",
			image,
			network.WithNetwork([]string{svc.name}, nw), testcontainers.WithExposedPorts(svc.port),
			testcontainers.WithWaitStrategy(wait.ForHTTP("/manage/health").WithPort(nat.Port(svc.port)).
				WithStartupTimeout(3*time.Minute)))
		if err != nil {
			log := buildLog.Bytes()
			if len(log) > 16384 {
				log = log[len(log)-16384:]
			}
			t.Logf("%s build log: %s", svc.name, log)
			t.Fatal(err)
		}
		testcontainers.CleanupContainer(t, container)
		captureLogs(t, container, svc.name)
		host, err := container.Host(ctx)
		if err != nil {
			t.Fatal(err)
		}
		port, err := container.MappedPort(ctx, nat.Port(svc.port))
		if err != nil {
			t.Fatal(err)
		}
		address := "http://" + net.JoinHostPort(host, port.Port())
		out.urls = append(out.urls, address)
		out.containers[svc.name] = container
		out.addresses[svc.name] = address
		if svc.name == "gateway" {
			out.gateway = address
		}
	}
	_, err = db.ExecContext(ctx, `INSERT INTO hotels (hotel_uid,name,country,city,address,stars,price)
 VALUES ('049161bb-badd-4fa8-9d90-87c9a82b0668','Ararat Park Hyatt Moscow',
 'Россия','Москва','Неглинная ул., 4',5,10000)`)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func captureLogs(t *testing.T, container testcontainers.Container, name string) {
	t.Helper()
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		logs, err := container.Logs(ctx)
		if err != nil {
			t.Logf("%s logs: %v", name, err)
			return
		}
		body, readErr := io.ReadAll(logs)
		if readErr != nil {
			t.Logf("%s read logs: %v", name, readErr)
		}
		if err := logs.Close(); err != nil {
			t.Logf("%s close logs: %v", name, err)
		}
		t.Logf("%s logs:\n%s", name, body)
	})
}
