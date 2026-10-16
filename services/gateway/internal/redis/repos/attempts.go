package repos

import (
	"context"
	"strconv"
	"uuid"

	"github.com/redis/go-redis/v9"
	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/opstreamifc"
)

func (s *Store) Start(ctx context.Context, task entities.Operation) (opstreamifc.Attempt, bool, error) {
	data, err := encodeTask(task)
	if err != nil {
		return opstreamifc.Attempt{}, false, oops.Wrap(err)
	}
	var attempt opstreamifc.Attempt
	started, err := s.update(ctx, task.Key.String(), nil,
		func(tx *redis.Tx, record operationRecord, now int64) (bool, error) {
			if record.State != stateReserved || record.Owner != s.owner.String() || record.Until <= now {
				return false, nil
			}
			token, err := tx.Incr(ctx, s.keys[7]).Result()
			if err != nil {
				return false, oops.Wrap(err)
			}
			until := now + s.cfg.LeaseDuration.Milliseconds()
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, s.keys[0]+task.Key.String(), "task", data, "state", stateRunning,
					"token", token, "number", 1, "until", until)
				s.schedule(ctx, pipe, task.Key.String(), record.Lane, until)
				return nil
			})
			if err == nil {
				attempt = opstreamifc.Attempt{Task: task, Token: strconv.FormatInt(token, 10), Number: 1}
			}
			return err == nil, oops.Wrap(err)
		})
	return attempt, started, storeError(err)
}

func (s *Store) Take(ctx context.Context, key uuid.UUID) (opstreamifc.Attempt, bool, error) {
	var attempt opstreamifc.Attempt
	taken, err := s.update(ctx, key.String(), nil,
		func(tx *redis.Tx, record operationRecord, now int64) (bool, error) {
			eligible := record.State == stateReserved || record.State == stateRunning || record.State == stateDeferred
			if !eligible || record.Until > now {
				return false, nil
			}
			owned, err := s.deliveryOwned(ctx, tx, record)
			if err != nil || !owned {
				return false, oops.Wrap(err)
			}
			if record.Task == "" {
				_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
					s.acknowledge(ctx, pipe, key.String(), record)
					s.remove(ctx, pipe, key.String())
					pipe.HSet(ctx, s.keys[0]+key.String(), "state", stateCompleted, "until", 0)
					pipe.PExpire(ctx, s.keys[0]+key.String(), s.cfg.HistoryTTL)
					return nil
				})
				return false, oops.Wrap(err)
			}
			var task entities.Operation
			if err := decodeTask([]byte(record.Task), &task); err != nil {
				return false, oops.Wrap(err)
			}
			token, err := tx.Incr(ctx, s.keys[7]).Result()
			if err != nil {
				return false, oops.Wrap(err)
			}
			_, stream := s.laneKeys(record.Lane)
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.XClaimJustID(ctx, &redis.XClaimArgs{
					Stream: stream, Group: consumerGroup, Consumer: s.owner.String(), Messages: []string{record.Message},
				})
				pipe.Incr(ctx, stream+":version")
				pipe.HSet(ctx, s.keys[0]+key.String(), "state", stateRunning, "owner", s.owner.String(),
					"token", token, "number", record.Number+1, "until", now+s.cfg.LeaseDuration.Milliseconds())
				return nil
			})
			if err == nil {
				attempt = opstreamifc.Attempt{Task: task, Token: strconv.FormatInt(token, 10), Number: record.Number + 1}
			}
			return err == nil, oops.Wrap(err)
		})
	return attempt, taken, storeError(err)
}

func lane(task entities.Operation) string {
	if task.Kind == entities.OperationCompensation {
		return laneCompensation
	}
	if len(task.Steps) == 1 && entities.HasAction[entities.DecreaseLoyaltyOperation](task) {
		return laneLoyalty
	}
	return laneCancellation
}

func (s *Store) Finish(
	ctx context.Context,
	attempt opstreamifc.Attempt,
	result entities.OperationResult,
) (bool, error) {
	state := stateDeferred
	if result.Status == entities.OperationCompleted && result.Task.Done() && result.Err == nil {
		state = stateCompleted
	}
	if result.Status == entities.OperationFailed || result.Status == entities.OperationRejected {
		state = stateFailed
	}
	data, err := encodeTask(result.Task)
	if err != nil {
		return false, oops.Wrap(err)
	}
	detail := ""
	if result.Err != nil {
		detail = result.Err.Error()
	}
	key := attempt.Task.Key.String()
	accepted, err := s.update(ctx, key, nil,
		func(tx *redis.Tx, record operationRecord, now int64) (bool, error) {
			owned := record.Owner == s.owner.String() && record.Until > now && record.Token == attempt.Token
			if record.State != stateRunning || !owned {
				return false, nil
			}
			if record.Message != "" {
				owned, err := s.deliveryOwned(ctx, tx, record)
				if err != nil || !owned {
					return false, oops.Wrap(err)
				}
			}
			_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				s.acknowledge(ctx, pipe, key, record)
				pipe.HSet(ctx, s.keys[0]+key, "task", data, "state", state, "error", detail)
				switch {
				case state == stateDeferred:
					until := now + s.cfg.RetryInterval.Milliseconds()
					pipe.HSet(ctx, s.keys[0]+key, "until", until, "lane", lane(result.Task))
					s.schedule(ctx, pipe, key, lane(result.Task), until)
				case result.Status == entities.OperationFailed:
					for _, queue := range s.keys[4:7] {
						pipe.ZRem(ctx, queue, key)
					}
					pipe.HSet(ctx, s.keys[0]+key, "until", 0)
				default:
					s.remove(ctx, pipe, key)
					pipe.HSet(ctx, s.keys[0]+key, "until", 0)
					pipe.PExpire(ctx, s.keys[0]+key, s.cfg.HistoryTTL)
				}
				return nil
			})
			return err == nil, oops.Wrap(err)
		})
	return accepted, storeError(err)
}
