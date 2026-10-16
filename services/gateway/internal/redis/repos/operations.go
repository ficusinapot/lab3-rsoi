package repos

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/redis/go-redis/v9"
	"github.com/samber/oops"
	"lab2/gateway/internal/core/services/opstream"
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
	keys   []string
	owner  uuid.UUID
	cfg    opstream.Config
}

var _ opstreamifc.Store = (*Store)(nil)

func New(ctx context.Context, c Config, cfg opstream.Config) (*Store, error) {
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
	prefix := c.Prefix
	store := &Store{
		client: client, owner: uuid.New(), cfg: cfg,
		keys: []string{
			prefix + "record:", prefix + "active", prefix + laneCompensation, prefix + "normal",
			prefix + "due:compensation", prefix + "due:cancellation", prefix + "due:loyalty", prefix + "generation",
			prefix + "stream:compensation", prefix + "stream:cancellation", prefix + "stream:loyalty",
		},
	}
	for _, stream := range store.keys[8:] {
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

func (s *Store) Inspect(ctx context.Context, key uuid.UUID) (entities.OperationInfo, error) {
	values, err := s.client.HMGet(ctx, s.keys[0]+key.String(),
		"state", "owner", "number", "until", "error", "task").Result()
	if err != nil {
		return entities.OperationInfo{}, storeError(err)
	}
	if values[0] == nil {
		return entities.OperationInfo{}, oops.Wrap(entities.ErrNotFound)
	}
	info := entities.OperationInfo{Key: key}
	fields := make([]string, len(values))
	for i, value := range values {
		if value != nil {
			var ok bool
			fields[i], ok = value.(string)
			if !ok {
				return info, storeError(entities.ErrInvalidInput)
			}
		}
	}
	info.State, info.Owner, info.Error = fields[0], fields[1], fields[4]
	info.Attempt, err = strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return info, storeError(err)
	}
	milliseconds, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return info, storeError(err)
	}
	if milliseconds != 0 {
		info.Until = time.UnixMilli(milliseconds).UTC()
	}
	if fields[5] != "" {
		info.Task = &entities.Operation{}
		if err = decodeTask([]byte(fields[5]), info.Task); err != nil {
			return info, storeError(err)
		}
	}
	return info, nil
}
