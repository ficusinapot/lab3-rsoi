package main

import (
	"path/filepath"
	"testing"

	"lab2/platform/config"
)

func TestFaultToleranceConfiguration(t *testing.T) {
	t.Parallel()
	for _, filename := range []string{"config.yaml", "config.docker.yaml", "config.test.yaml"} {
		t.Run(filename, func(t *testing.T) {
			t.Parallel()
			cfg, err := config.Load[Config](filepath.Join("..", "..", filename))
			if err != nil {
				t.Fatal(err)
			}
			for _, change := range []func(*Config){
				func(c *Config) { c.HTTP.APIPrefix = "/booking/v2" },
				func(c *Config) { c.Clients.Reservation.MaxInFlight = 0 },
				func(c *Config) { c.Clients.Payment.MaxInFlight = 0 },
				func(c *Config) { c.Clients.Loyalty.MaxInFlight = 0 },
				func(c *Config) { c.Clients.Payment.URL = "" },
				func(c *Config) { c.Breaker.FailureThreshold = 0 },
				func(c *Config) { c.Breaker.ResetTimeout = 0 },
				func(c *Config) { c.OpStream.Capacity = 0 },
				func(c *Config) { c.OpStream.Concurrency = 0 },
				func(c *Config) { c.OpStream.Concurrency = c.OpStream.Capacity + 1 },
				func(c *Config) { c.OpStream.RetryInterval = 0 },
				func(c *Config) { c.OpStream.AttemptTimeout = 0 },
				func(c *Config) { c.OpStream.RollbackTimeout = 0 },
				func(c *Config) { c.OpStream.ShutdownTimeout = 0 },
			} {
				invalid := cfg
				change(&invalid)
				if err := invalid.Validate(); err == nil {
					t.Fatal("invalid fault tolerance config accepted")
				}
			}
		})
	}
}
