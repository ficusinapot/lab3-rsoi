package repos

import (
	"context"
	"strconv"
	"uuid"

	"github.com/redis/go-redis/v9"
	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
)

func (s *Store) Reserve(ctx context.Context, key uuid.UUID, kind entities.OperationKind) (bool, error) {
	class, lane, limit := s.keys[3], laneCancellation, s.cfg.Capacity-s.cfg.CompensationCapacity
	if kind == entities.OperationCompensation {
		class, lane, limit = s.keys[2], laneCompensation, s.cfg.CompensationCapacity
	}
	var full bool
	reserved, err := s.update(ctx, key.String(), []string{s.keys[1], class},
		func(tx *redis.Tx, record operationRecord, now int64) (bool, error) {
			full = false
			active, err := tx.SIsMember(ctx, s.keys[1], key.String()).Result()
			if err != nil || active || record.State == stateCompleted {
				return false, oops.Wrap(err)
			}
			total, err := tx.SCard(ctx, s.keys[1]).Result()
			if err != nil {
				return false, oops.Wrap(err)
			}
			count, err := tx.SCard(ctx, class).Result()
			if err != nil {
				return false, oops.Wrap(err)
			}
			if total >= int64(s.cfg.Capacity) || count >= int64(limit) {
				full = true
				return false, nil
			}
			token, err := tx.Incr(ctx, s.keys[7]).Result()
			if err != nil {
				return false, oops.Wrap(err)
			}
			until := now + s.cfg.LeaseDuration.Milliseconds()
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Del(ctx, s.keys[0]+key.String())
				pipe.HSet(ctx, s.keys[0]+key.String(), map[string]any{
					"state": stateReserved, "owner": s.owner.String(), "token": strconv.FormatInt(token, 10),
					"kind": int(kind), "number": 0, "task": "", "lane": lane, "until": until,
				})
				pipe.SAdd(ctx, s.keys[1], key.String())
				pipe.SAdd(ctx, class, key.String())
				s.schedule(ctx, pipe, key.String(), lane, until)
				return nil
			})
			return err == nil, oops.Wrap(err)
		})
	if err != nil {
		return false, storeError(err)
	}
	if full {
		return false, oops.Wrap(entities.ErrQueueFull)
	}
	return reserved, nil
}

func (s *Store) Release(ctx context.Context, key uuid.UUID) error {
	released, err := s.update(ctx, key.String(), nil,
		func(tx *redis.Tx, record operationRecord, now int64) (bool, error) {
			if record.State != stateReserved || (record.Owner != s.owner.String() || record.Until <= now) {
				return false, nil
			}
			_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				s.remove(ctx, pipe, key.String())
				pipe.HSet(ctx, s.keys[0]+key.String(), "state", stateCompleted, "until", 0, "task", "")
				pipe.PExpire(ctx, s.keys[0]+key.String(), s.cfg.HistoryTTL)
				return nil
			})
			return err == nil, oops.Wrap(err)
		})
	if err != nil {
		return storeError(err)
	}
	if !released {
		return oops.Wrap(entities.ErrOperationBusy)
	}
	return nil
}

func (s *Store) Prepare(ctx context.Context, task entities.Operation) error {
	data, err := encodeTask(task)
	if err != nil {
		return oops.Wrap(err)
	}
	prepared, err := s.update(ctx, task.Key.String(), nil,
		func(tx *redis.Tx, record operationRecord, now int64) (bool, error) {
			if record.State != stateReserved || (record.Owner != s.owner.String() || record.Until <= now) {
				return false, nil
			}
			_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, s.keys[0]+task.Key.String(), "task", data)
				return nil
			})
			return err == nil, oops.Wrap(err)
		})
	if err != nil {
		return storeError(err)
	}
	if !prepared {
		return oops.Wrap(entities.ErrOperationBusy)
	}
	return nil
}

func (s *Store) Pending(ctx context.Context, key uuid.UUID) (entities.Operation, bool, error) {
	var active *redis.BoolCmd
	var fields *redis.MapStringStringCmd
	_, err := s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		active = pipe.SIsMember(ctx, s.keys[1], key.String())
		fields = pipe.HGetAll(ctx, s.keys[0]+key.String())
		return nil
	})
	if err != nil {
		return entities.Operation{}, false, storeError(err)
	}
	if !active.Val() {
		return entities.Operation{}, false, nil
	}
	var task entities.Operation
	if data := fields.Val()["task"]; data != "" {
		if err := decodeTask([]byte(data), &task); err != nil {
			return task, false, storeError(err)
		}
	}
	if task.Key == uuid.Nil() {
		return task, true, oops.Wrap(entities.ErrOperationBusy)
	}
	return task, true, nil
}

func (s *Store) Remaining(ctx context.Context) (int, error) {
	count, err := s.client.SCard(ctx, s.keys[1]).Result()
	return int(count), storeError(err)
}

func (s *Store) RetryFailed(ctx context.Context, key uuid.UUID) error {
	retried, err := s.update(ctx, key.String(), []string{s.keys[1]},
		func(tx *redis.Tx, record operationRecord, now int64) (bool, error) {
			if record.State != stateFailed {
				return false, nil
			}
			active, err := tx.SIsMember(ctx, s.keys[1], key.String()).Result()
			if err != nil || !active {
				return false, oops.Wrap(err)
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, s.keys[0]+key.String(), "state", stateDeferred, "until", now)
				s.schedule(ctx, pipe, key.String(), record.Lane, now)
				return nil
			})
			return err == nil, oops.Wrap(err)
		})
	if err != nil {
		return storeError(err)
	}
	if !retried {
		return oops.Wrap(entities.ErrNotFound)
	}
	return nil
}
