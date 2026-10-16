package opstream

import (
	"context"
	"errors"
	"strings"
	"time"
	"uuid"

	"github.com/redis/go-redis/v9"
	"github.com/samber/oops"
	streamcfg "lab2/gateway/internal/core/services/opstream"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/opstreamifc"
)

type Config struct {
	URL         string        `mapstructure:"url"`
	Prefix      string        `mapstructure:"prefix"`
	PoolSize    int           `mapstructure:"pool_size"`
	DialTimeout time.Duration `mapstructure:"dial_timeout"`
}

func (c Config) Validate() error {
	if _, err := redis.ParseURL(c.URL); err != nil {
		return oops.Wrapf(err, "invalid Redis URL")
	}
	if c.Prefix == "" || !strings.Contains(c.Prefix, "{") || !strings.Contains(c.Prefix, "}") ||
		c.PoolSize <= 0 || c.DialTimeout <= 0 {
		return oops.Errorf("invalid Redis connection configuration")
	}
	return nil
}

type Store struct {
	client *redis.Client
	keys   keyspace
	owner  uuid.UUID
	cfg    streamcfg.Config
}

var _ opstreamifc.Store = (*Store)(nil)

func New(ctx context.Context, c Config, cfg streamcfg.Config) (*Store, error) {
	options, err := redis.ParseURL(c.URL)
	if err != nil {
		return nil, oops.Wrapf(err, "parse Redis URL")
	}
	options.PoolSize = c.PoolSize
	options.DialTimeout = c.DialTimeout
	options.ReadTimeout = cfg.StorageTimeout
	options.WriteTimeout = cfg.StorageTimeout
	options.PoolTimeout = cfg.StorageTimeout
	options.MaxRetries = -1
	options.ContextTimeoutEnabled = true
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, storeError(err)
	}
	store := &Store{
		client: client, owner: uuid.New(), cfg: cfg, keys: newKeyspace(c.Prefix),
	}
	for _, stream := range store.keys.streams() {
		err := client.XGroupCreateMkStream(ctx, stream, consumerGroup, "0").Err()
		if err != nil && !strings.HasPrefix(err.Error(), "BUSYGROUP ") {
			_ = client.Close()
			return nil, storeError(err)
		}
	}
	return store, nil
}
func (s *Store) Close() error { return oops.Wrapf(s.client.Close(), "close Redis") }

func storeError(err error) error {
	if err == nil {
		return nil
	}
	return oops.Wrapf(errors.Join(entities.ErrQueueUnavailable, err), "operation storage failed")
}
