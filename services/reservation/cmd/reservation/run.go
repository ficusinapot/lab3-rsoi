package main

import (
	"context"
	"errors"

	"lab2/platform/database"
	"lab2/platform/logging"
	"lab2/platform/observability/metrics"
	"lab2/platform/rest"
	serviceconfig "lab2/reservation/internal/config"
	"lab2/reservation/internal/core/usecases/bookings"
	"lab2/reservation/internal/core/usecases/hotels"
	"lab2/reservation/internal/postgres"
	bookingstorage "lab2/reservation/internal/postgres/repos/bookings"
	hotelstorage "lab2/reservation/internal/postgres/repos/hotels"
	httptransport "lab2/reservation/internal/rest"

	"github.com/samber/oops"
)

func Run(ctx context.Context, cfg serviceconfig.Config) (runErr error) {
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
		return oops.Wrapf(err, "initialize metrics")
	}
	if err := telemetry.RegisterDatabase(pool); err != nil {
		return oops.Wrap(err)
	}
	stopMetrics, err := telemetry.StartAutometrics(cfg.Metrics)
	if err != nil {
		return oops.Wrap(err)
	}
	defer stopMetrics(nil)

	client := postgres.NewClient(pool)
	hotelService := hotels.New(hotelstorage.New(client), cfg.Pagination)
	bookingService := bookings.New(bookingstorage.New(client))
	handler := httptransport.New(httptransport.Config{HTTP: cfg.HTTP, OpenAPI: cfg.OpenAPI, MetricsPath: cfg.Metrics.Path},
		httptransport.Dependencies{
			Database: pool,
			Hotels:   hotelService,
			Bookings: bookingService,
			Logger:   logger,
			Metrics:  telemetry,
		})
	logger.Info("reservation service listening", "address", cfg.HTTP.Address)
	return oops.Wrapf(rest.Serve(ctx, cfg.HTTP, handler), "run reservation service")
}
