package opstream

import (
	"context"
	"strconv"
	"time"
	"uuid"

	"github.com/samber/oops"
	"lab2/gateway/internal/models/entities"
)

func (s *Store) Inspect(ctx context.Context, key uuid.UUID) (entities.OperationInfo, error) {
	values, err := s.client.HMGet(ctx, s.keys.operationPrefix+key.String(),
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
