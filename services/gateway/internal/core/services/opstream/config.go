package opstream

import (
	"time"

	"github.com/samber/oops"
)

type Config struct {
	CompensationCapacity int           `mapstructure:"compensation_capacity"`
	CompensationWorkers  int           `mapstructure:"compensation_workers"`
	CancellationWorkers  int           `mapstructure:"cancellation_workers"`
	LoyaltyWorkers       int           `mapstructure:"loyalty_workers"`
	PollInterval         time.Duration `mapstructure:"poll_interval"`
	LeaseDuration        time.Duration `mapstructure:"lease_duration"`
	StorageTimeout       time.Duration `mapstructure:"storage_timeout"`
	HistoryTTL           time.Duration `mapstructure:"history_ttl"`
	Capacity             int           `mapstructure:"capacity"`
	Concurrency          int           `mapstructure:"concurrency"`
	RetryInterval        time.Duration `mapstructure:"retry_interval"`
	AttemptTimeout       time.Duration `mapstructure:"attempt_timeout"`
	RollbackTimeout      time.Duration `mapstructure:"rollback_timeout"`
	ShutdownTimeout      time.Duration `mapstructure:"shutdown_timeout"`
}

func (c Config) Validate() error {
	if c.Capacity <= 0 || c.Concurrency <= 0 || c.Concurrency > c.Capacity {
		return oops.Errorf("retry capacity and concurrency must be positive; concurrency must not exceed capacity")
	}
	if c.RetryInterval <= 0 || c.AttemptTimeout <= 0 || c.RollbackTimeout <= 0 || c.ShutdownTimeout <= 0 {
		return oops.Errorf("retry timeouts must be positive")
	}
	if c.CompensationCapacity <= 0 || c.CompensationCapacity >= c.Capacity ||
		c.CompensationWorkers <= 0 || c.CancellationWorkers <= 0 || c.LoyaltyWorkers <= 0 ||
		c.CompensationWorkers+c.CancellationWorkers+c.LoyaltyWorkers != c.Concurrency {
		return oops.Errorf("invalid operation isolation budgets")
	}
	if c.PollInterval <= 0 || c.StorageTimeout <= 0 || c.HistoryTTL <= 0 ||
		c.LeaseDuration <= c.AttemptTimeout+c.StorageTimeout ||
		c.LeaseDuration <= c.RollbackTimeout+c.StorageTimeout {
		return oops.Errorf("invalid operation lease and storage timeouts")
	}
	return nil
}
