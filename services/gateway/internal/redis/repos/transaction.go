package repos

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
	"github.com/samber/oops"
)

const (
	stateReserved    = "reserved"
	stateRunning     = "running"
	stateDeferred    = "deferred"
	stateCompleted   = "completed"
	stateFailed      = "failed"
	laneCompensation = "compensation"
	laneCancellation = "cancellation"
	laneLoyalty      = "loyalty"
	consumerGroup    = "workers"
)

type operationRecord struct {
	State   string `redis:"state"`
	Owner   string `redis:"owner"`
	Token   string `redis:"token"`
	Task    string `redis:"task"`
	Lane    string `redis:"lane"`
	Message string `redis:"message"`
	Number  uint64 `redis:"number"`
	Until   int64  `redis:"until"`
}

// update retries only optimistic conflicts; transport failures may have an unknown outcome.
func (s *Store) update(
	ctx context.Context,
	key string,
	watched []string,
	apply func(*redis.Tx, operationRecord, int64) (bool, error),
) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.StorageTimeout)
	defer cancel()
	watched = append([]string{s.keys[0] + key}, watched...)
	for {
		if err := ctx.Err(); err != nil {
			return false, oops.Wrap(err)
		}
		var updated bool
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			var record operationRecord
			if err := tx.HGetAll(ctx, s.keys[0]+key).Scan(&record); err != nil {
				return oops.Wrap(err)
			}
			now, err := tx.Time(ctx).Result()
			if err != nil {
				return oops.Wrap(err)
			}
			updated, err = apply(tx, record, now.UnixMilli())
			return oops.Wrap(err)
		}, watched...)
		if !errors.Is(err, redis.TxFailedErr) {
			return updated && err == nil, oops.Wrap(err)
		}
	}
}

func (s *Store) laneKeys(lane string) (string, string) {
	switch lane {
	case laneCompensation:
		return s.keys[4], s.keys[8]
	case laneCancellation:
		return s.keys[5], s.keys[9]
	default:
		return s.keys[6], s.keys[10]
	}
}

func (s *Store) schedule(ctx context.Context, pipe redis.Pipeliner, key, lane string, when int64) {
	for _, queue := range s.keys[4:7] {
		pipe.ZRem(ctx, queue, key)
	}
	queue, _ := s.laneKeys(lane)
	pipe.ZAdd(ctx, queue, redis.Z{Score: float64(when), Member: key})
}

func (s *Store) remove(ctx context.Context, pipe redis.Pipeliner, key string) {
	for _, set := range s.keys[1:4] {
		pipe.SRem(ctx, set, key)
	}
	for _, queue := range s.keys[4:7] {
		pipe.ZRem(ctx, queue, key)
	}
}

func (s *Store) acknowledge(ctx context.Context, pipe redis.Pipeliner, key string, record operationRecord) {
	if record.Message == "" {
		return
	}
	_, stream := s.laneKeys(record.Lane)
	pipe.XAck(ctx, stream, consumerGroup, record.Message)
	pipe.XDel(ctx, stream, record.Message)
	pipe.Incr(ctx, stream+":version")
	pipe.HDel(ctx, s.keys[0]+key, "message")
}

// Redis does not invalidate WATCH for PEL changes. Every delivery mutation
// increments this version in the same transaction as the Streams command.
func (s *Store) deliveryOwned(ctx context.Context, tx *redis.Tx, record operationRecord) (bool, error) {
	if record.Message == "" {
		return false, nil
	}
	_, stream := s.laneKeys(record.Lane)
	if err := tx.Watch(ctx, stream, stream+":version").Err(); err != nil {
		return false, oops.Wrap(err)
	}
	pending, err := tx.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: stream, Group: consumerGroup, Start: record.Message, End: record.Message, Count: 1,
	}).Result()
	return len(pending) == 1 && pending[0].Consumer == s.owner.String(), oops.Wrap(err)
}
