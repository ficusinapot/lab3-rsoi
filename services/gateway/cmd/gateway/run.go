package main

import (
	"context"
	"errors"

	redisopstream "lab2/gateway/internal/redis/opstream"

	"lab2/gateway/internal/core/services/opstream"

	"lab2/gateway/internal/core/usecases/bookings"
	"lab2/gateway/internal/core/usecases/operations"
	"lab2/gateway/internal/httpclient"
	loyaltyrepo "lab2/gateway/internal/loyalty/repos"
	paymentrepo "lab2/gateway/internal/payment/repos"
	reservationrepo "lab2/gateway/internal/reservation/repos"
	transport "lab2/gateway/internal/rest/bookings"
	systemtransport "lab2/gateway/internal/rest/system"
	"lab2/platform/logging"
	"lab2/platform/observability/metrics"
	"lab2/platform/rest"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/samber/oops"
)

func Run(ctx context.Context, cfg Config) (err error) {
	logger, closeLogs, err := logging.New(cfg.Logging)
	if err != nil {
		return oops.Wrapf(err, "initialize logging")
	}
	defer closeLogs()
	telemetry, err := metrics.New(cfg.Metrics)
	if err != nil {
		return oops.Wrap(err)
	}
	stop, err := telemetry.StartAutometrics(cfg.Metrics)
	if err != nil {
		return oops.Wrap(err)
	}
	defer stop(nil)
	router := chi.NewRouter()
	router.Use(rest.Observability(logger, telemetry))
	router.Use(middleware.Timeout(cfg.HTTP.RequestTimeout))
	reservationClient := httpclient.New(cfg.Clients.Reservation)
	defer reservationClient.CloseIdleConnections()
	paymentClient := httpclient.New(cfg.Clients.Payment)
	defer paymentClient.CloseIdleConnections()
	loyaltyClient := httpclient.New(cfg.Clients.Loyalty)
	defer loyaltyClient.CloseIdleConnections()
	reservations, err := reservationrepo.New(
		reservationClient, cfg.Clients.Reservation.URL, httpclient.NewBreaker(cfg.Breaker),
	)
	if err != nil {
		return oops.Wrap(err)
	}
	payments, err := paymentrepo.New(paymentClient, cfg.Clients.Payment.URL, httpclient.NewBreaker(cfg.Breaker))
	if err != nil {
		return oops.Wrap(err)
	}
	loyalty, err := loyaltyrepo.New(loyaltyClient, cfg.Clients.Loyalty.URL, httpclient.NewBreaker(cfg.Breaker))
	if err != nil {
		return oops.Wrap(err)
	}

	actions := operations.NewActions(reservations, payments, loyalty)
	processor := operations.New(actions)
	storageCtx, cancelStorage := context.WithTimeout(ctx, cfg.OpStream.StorageTimeout)
	store, err := redisopstream.New(storageCtx, cfg.Redis, cfg.OpStream)
	cancelStorage()
	if err != nil {
		return oops.Wrap(err)
	}
	defer func() { err = oops.Wrap(errors.Join(err, store.Close())) }()
	stream := opstream.New(cfg.OpStream, store, processor, logger)
	streamCtx, stopStream := context.WithCancel(context.WithoutCancel(ctx))
	streamDone := make(chan struct{})
	go func() { defer close(streamDone); stream.Run(streamCtx) }()
	defer func() {
		stopStream()
		<-streamDone
		drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.OpStream.ShutdownTimeout)
		defer cancel()
		if drainErr := stream.Drain(drainCtx); drainErr != nil {
			logger.ErrorContext(drainCtx, "operation drain failed", "error", drainErr)
			err = oops.Wrapf(errors.Join(err, drainErr), "shutdown gateway")
		}
	}()
	commands := bookings.NewCommands(
		reservations, loyalty, operations.NewGroups(stream, cfg.OpStream.RollbackTimeout, reservations, payments, loyalty),
	)
	queries := bookings.NewQueries(reservations, payments, loyalty, cfg.EnrichmentConcurrency)
	transport.Register(router, commands, queries, logger, cfg.HTTP)
	if err := systemtransport.Register(router, cfg.HTTP, cfg.OpenAPI, telemetry, cfg.Metrics.Path); err != nil {
		return oops.Wrapf(err, "register service endpoints")
	}
	logger.Info("gateway listening", "address", cfg.HTTP.Address)
	return oops.Wrapf(rest.Serve(ctx, cfg.HTTP, router), "run gateway")
}
