package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/oops"
	"lab2/loyalty/internal/core/usecases/loyalties"
	"lab2/loyalty/internal/postgres"
	storage "lab2/loyalty/internal/postgres/repos"
	transport "lab2/loyalty/internal/rest/loyalties"
	"lab2/platform/database"
	"lab2/platform/logging"
	"lab2/platform/observability/metrics"
	"lab2/platform/rest"
)

func Run(ctx context.Context, cfg Config) (runErr error) {
	logger, closeLogs, err := logging.New(cfg.Logging)
	if err != nil {
		return oops.Wrapf(err, "initialize logging")
	}
	defer closeLogs()
	pool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return oops.Wrapf(err, "initialize database")
	}
	defer func() {
		if err := pool.Close(); err != nil {
			runErr = errors.Join(runErr, oops.Wrapf(err, "close database"))
		}
	}()
	telemetry, err := metrics.New(cfg.Metrics)
	if err != nil {
		return oops.Wrap(err)
	}
	if err = telemetry.RegisterDatabase(pool); err != nil {
		return oops.Wrap(err)
	}
	stop, err := telemetry.StartAutometrics(cfg.Metrics)
	if err != nil {
		return oops.Wrap(err)
	}
	defer stop(nil)
	service := loyalties.New(storage.New(postgres.NewClient(pool)))
	router, api := rest.New(cfg.HTTP, cfg.OpenAPI, rest.Observability(logger, telemetry))
	transport.Register(api, service, logger, cfg.HTTP)
	router.Handle(cfg.Metrics.Path, telemetry.Handler())
	router.Get("/manage/health", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.PingContext(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	logger.Info("service listening", "address", cfg.HTTP.Address)
	return oops.Wrapf(rest.Serve(ctx, cfg.HTTP, router), "run service")
}
