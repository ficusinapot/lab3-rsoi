package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/samber/oops"
)

type Config struct {
	URL                   string        `mapstructure:"url"`
	MaxOpenConnections    int           `mapstructure:"max_open_connections"`
	MaxIdleConnections    int           `mapstructure:"max_idle_connections"`
	ConnectionMaxLifetime time.Duration `mapstructure:"connection_max_lifetime"`
	ConnectionMaxIdleTime time.Duration `mapstructure:"connection_max_idle_time"`
	ConnectTimeout        time.Duration `mapstructure:"connect_timeout"`
}

func (c Config) Validate() error {
	invalidConnection := c.URL == "" || c.ConnectTimeout <= 0
	invalidPool := c.MaxOpenConnections < 1 || c.MaxIdleConnections < 0
	invalidLifetime := c.ConnectionMaxLifetime <= 0 || c.ConnectionMaxIdleTime <= 0
	if invalidConnection || invalidPool || c.MaxIdleConnections > c.MaxOpenConnections || invalidLifetime {
		return oops.Errorf("invalid database connection or pool configuration")
	}
	return nil
}

func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.URL)
	if err != nil {
		return nil, oops.Wrapf(err, "open database")
	}
	db.SetMaxOpenConns(cfg.MaxOpenConnections)
	db.SetMaxIdleConns(cfg.MaxIdleConnections)
	db.SetConnMaxLifetime(cfg.ConnectionMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnectionMaxIdleTime)
	ctx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, oops.Wrapf(errors.Join(err, db.Close()), "connect database")
	}
	return db, nil
}
