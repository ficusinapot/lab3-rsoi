package opstream

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"uuid"

	"github.com/redis/go-redis/v9"
	"github.com/samber/oops"
)

func (s *Store) ReadyKeys(ctx context.Context, lane string) ([]uuid.UUID, error) {
	var keys []uuid.UUID
	for _, name := range []string{laneCompensation, laneCancellation, laneLoyalty} {
		if lane != "" && lane != name {
			continue
		}
		queue, stream := s.laneKeys(name)
		now, err := s.client.Time(ctx).Result()
		if err != nil {
			return nil, storeError(err)
		}
		due, err := s.client.ZRangeArgs(ctx, redis.ZRangeArgs{
			Key: queue, ByScore: true, Start: "-inf", Stop: strconv.FormatInt(now.UnixMilli(), 10),
		}).Result()
		if err != nil {
			return nil, storeError(err)
		}
		for _, key := range due {
			if err := s.publish(ctx, key, name); err != nil {
				return nil, storeError(err)
			}
		}
		var messages []redis.XMessage
		cursor := "0-0"
		for {
			var claimed *redis.XAutoClaimCmd
			_, err = s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				claimed = pipe.XAutoClaim(ctx, &redis.XAutoClaimArgs{
					Stream: stream, Group: consumerGroup, Consumer: s.owner.String(),
					MinIdle: s.cfg.LeaseDuration, Start: cursor, Count: 1,
				})
				pipe.Incr(ctx, stream+":version")
				return nil
			})
			if err != nil {
				return nil, storeError(err)
			}
			messages, cursor, err = claimed.Result()
			if err != nil {
				return nil, storeError(err)
			}
			if cursor == "0-0" || len(messages) != 0 {
				break
			}
		}
		if len(messages) == 0 {
			var read *redis.XStreamSliceCmd
			_, err := s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				read = pipe.XReadGroup(ctx, &redis.XReadGroupArgs{
					Group: consumerGroup, Consumer: s.owner.String(),
					Streams: []string{stream, ">"}, Count: 1, Block: -1,
				})
				pipe.Incr(ctx, stream+":version")
				return nil
			})
			if err != nil && !errors.Is(err, redis.Nil) {
				return nil, storeError(err)
			}
			if streams := read.Val(); len(streams) != 0 {
				messages = streams[0].Messages
			}
		}
		for _, message := range messages {
			value, _ := message.Values["key"].(string)
			key, err := uuid.Parse(value)
			if err != nil {
				return nil, storeError(err)
			}
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (s *Store) publish(ctx context.Context, key, lane string) error {
	queue, stream := s.laneKeys(lane)
	_, err := s.update(ctx, key, []string{stream},
		func(tx *redis.Tx, record operationRecord, now int64) (bool, error) {
			eligible := record.State == stateReserved || record.State == stateRunning || record.State == stateDeferred
			if record.Until > now || record.Lane != lane {
				return false, nil
			}
			var id string
			if eligible && record.Message == "" {
				info, err := tx.XInfoStream(ctx, stream).Result()
				if err != nil {
					return false, oops.Wrap(err)
				}
				milliseconds, sequence, _ := strings.Cut(info.LastGeneratedID, "-")
				last, err := strconv.ParseInt(milliseconds, 10, 64)
				if err != nil {
					return false, oops.Wrap(err)
				}
				counter, err := strconv.ParseUint(sequence, 10, 64)
				if err != nil {
					return false, oops.Wrap(err)
				}
				id = strconv.FormatInt(now, 10) + "-0"
				if last >= now {
					id = milliseconds + "-" + strconv.FormatUint(counter+1, 10)
				}
			}
			_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				if id != "" {
					// The explicit ID lets XADD and its record reference commit together.
					pipe.XAdd(ctx, &redis.XAddArgs{Stream: stream, ID: id, Values: map[string]any{"key": key}})
					pipe.HSet(ctx, s.keys.operationPrefix+key, "message", id)
				}
				pipe.ZRem(ctx, queue, key)
				return nil
			})
			return err == nil, oops.Wrap(err)
		})
	return oops.Wrap(err)
}
