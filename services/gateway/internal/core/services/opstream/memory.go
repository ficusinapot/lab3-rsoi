package opstream

import (
	"context"
	"strconv"
	"uuid"

	"github.com/samber/oops"

	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/opstreamifc"
)

// memoryStore is used by isolated use case and worker tests; production uses Redis.
type memoryStore struct{ queue *queue }

func (m memoryStore) Reserve(_ context.Context, key uuid.UUID, _ entities.OperationKind) (bool, error) {
	return m.queue.Reserve(key)
}

func (m memoryStore) Release(_ context.Context, key uuid.UUID) error {
	m.queue.Release(key)
	return nil
}

func (m memoryStore) Pending(_ context.Context, key uuid.UUID) (entities.Operation, bool, error) {
	task, ok := m.queue.Pending(key)
	return task, ok, nil
}

func (m memoryStore) Prepare(_ context.Context, task entities.Operation) error {
	m.queue.mu.Lock()
	defer m.queue.mu.Unlock()
	item, ok := m.queue.tasks[task.Key]
	if !ok || item.claim != 0 || item.ready {
		return entities.ErrOperationBusy
	}
	item.task = task
	m.queue.tasks[task.Key] = item
	return nil
}

func (m memoryStore) Start(_ context.Context, task entities.Operation) (opstreamifc.Attempt, bool, error) {
	a, ok := m.queue.start(task)
	return opstreamifc.Attempt{Task: a.task, Token: strconv.FormatUint(a.claim, 10), Number: a.number}, ok, nil
}

func (m memoryStore) ReadyKeys(_ context.Context, _ string) ([]uuid.UUID, error) {
	return m.queue.readyKeys(), nil
}

func (m memoryStore) Take(_ context.Context, key uuid.UUID) (opstreamifc.Attempt, bool, error) {
	a, ok := m.queue.take(key)
	return opstreamifc.Attempt{Task: a.task, Token: strconv.FormatUint(a.claim, 10), Number: a.number}, ok, nil
}

func (m memoryStore) Finish(
	_ context.Context, a opstreamifc.Attempt, r entities.OperationResult,
) (bool, error) {
	token, err := strconv.ParseUint(a.Token, 10, 64)
	if err != nil {
		return false, oops.Wrap(err)
	}
	return m.queue.finish(attempt{task: a.Task, claim: token, number: a.Number}, r), nil
}
func (m memoryStore) Remaining(_ context.Context) (int, error) { return m.queue.remaining(), nil }
