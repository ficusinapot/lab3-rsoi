package config

import (
	"errors"

	"github.com/samber/oops"
	"lab2/platform/database"
	"lab2/platform/logging"
	"lab2/platform/observability/metrics"
	"lab2/platform/rest"
	"lab2/reservation/internal/core/usecases/hotels"
)

type Config struct {
	HTTP       rest.Config             `mapstructure:"http"`
	Database   database.Config         `mapstructure:"database"`
	Pagination hotels.PaginationConfig `mapstructure:"pagination"`
	Logging    logging.Config          `mapstructure:"logging"`
	Metrics    metrics.Config          `mapstructure:"metrics"`
	OpenAPI    rest.OpenAPIConfig      `mapstructure:"openapi"`
}

func (c Config) Validate() error {
	return oops.Wrapf(errors.Join(
		c.HTTP.Validate(),
		c.Database.Validate(),
		c.Pagination.Validate(),
		c.Logging.Validate(),
		c.Metrics.Validate(),
		c.OpenAPI.Validate(),
	), "validate service configuration")
}
