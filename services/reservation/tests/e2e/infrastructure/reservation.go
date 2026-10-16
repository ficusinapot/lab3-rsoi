//go:build e2e

package infrastructure

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/build"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

func newReservation(
	t *testing.T,
	ctx context.Context,
	net *testcontainers.DockerNetwork,
	root string,
) (testcontainers.Container, string) {
	t.Helper()
	image := testcontainers.ContainerCustomizer(testcontainers.WithDockerfile(testcontainers.FromDockerfile{
		Context: root, Dockerfile: filepath.Join("services", "reservation", "Dockerfile"),
		KeepImage: true, BuildLogWriter: io.Discard,
		BuildOptionsModifier: func(opts *build.ImageBuildOptions) { opts.Version = build.BuilderBuildKit },
	}))
	if os.Getenv("RESERVATION_E2E_PREBUILT") == "1" {
		image = testcontainers.WithImage("lab3-reservation")
	}
	service, err := testcontainers.Run(ctx, "", image,
		network.WithNetwork([]string{"reservation"}, net),
		testcontainers.WithExposedPorts(appPort),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/manage/health").WithPort(appPort).WithStartupTimeout(3*time.Minute),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, service)
	host, err := service.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := service.MappedPort(ctx, appPort)
	if err != nil {
		t.Fatal(err)
	}
	return service, "http://" + host + ":" + port.Port()
}
