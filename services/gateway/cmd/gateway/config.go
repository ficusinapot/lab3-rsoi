package main

import (
	"errors"

	redisopstream "lab2/gateway/internal/redis/opstream"

	"lab2/gateway/internal/core/services/opstream"

	"github.com/samber/oops"
	"lab2/gateway/internal/httpclient"
	"lab2/platform/logging"
	"lab2/platform/observability/metrics"
	"lab2/platform/rest"
)

type Config struct {
	Redis                 redisopstream.Config     `mapstructure:"redis"`
	Breaker               httpclient.BreakerConfig `mapstructure:"breaker"`
	OpStream              opstream.Config          `mapstructure:"opstream"`
	EnrichmentConcurrency int                      `mapstructure:"enrichment_concurrency"`
	HTTP                  rest.Config              `mapstructure:"http"`
	Logging               logging.Config           `mapstructure:"logging"`
	Metrics               metrics.Config           `mapstructure:"metrics"`
	OpenAPI               rest.OpenAPIConfig       `mapstructure:"openapi"`
	Clients               ClientsConfig            `mapstructure:"clients"`
}

func (c Config) Validate() error {
	if c.HTTP.APIPrefix != "/api/v1" {
		return oops.Errorf("gateway HTTP API prefix must be /api/v1")
	}
	if c.Breaker.FailureThreshold <= 0 || c.Breaker.ResetTimeout <= 0 {
		return oops.Errorf("breaker threshold and timeout must be positive")
	}
	if c.EnrichmentConcurrency <= 0 {
		return oops.Errorf("enrichment_concurrency must be positive")
	}
	return oops.Wrapf(errors.Join(c.HTTP.Validate(), c.Logging.Validate(),
		c.Metrics.Validate(), c.OpenAPI.Validate(), c.Clients.Validate(), c.OpStream.Validate(), c.Redis.Validate()),
		"validate gateway configuration")
}

// ClientsConfig gives each downstream its own transport and concurrency budget.
type ClientsConfig struct {
	Reservation httpclient.Config `mapstructure:"reservation"`
	Payment     httpclient.Config `mapstructure:"payment"`
	Loyalty     httpclient.Config `mapstructure:"loyalty"`
}

func (c ClientsConfig) Validate() error {
	return oops.Wrapf(errors.Join(c.Reservation.Validate(), c.Payment.Validate(), c.Loyalty.Validate()),
		"validate downstream clients")
}
