package opstream

import (
	"sync"
	"time"
	"uuid"

	"lab2/gateway/internal/models/entities"

	"github.com/samber/oops"
)

type entry struct {
	task        entities.Operation
	ready       bool
	claim       uint64
	attempts    uint64
	nextAttempt time.Time
}

type attempt struct {
	task   entities.Operation
	claim  uint64
	number uint64
}

// queue owns reservations and attempt claims, but never executes business steps.
type queue struct {
	mu            sync.Mutex
	tasks         map[uuid.UUID]entry
	capacity      int
	generation    uint64
	retryInterval time.Duration
	now           func() time.Time
}

func newQueue(cfg Config) *queue {
	return &queue{
		capacity: cfg.Capacity, tasks: make(map[uuid.UUID]entry),
		retryInterval: cfg.RetryInterval, now: time.Now,
	}
}

func (q *queue) Reserve(key uuid.UUID) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, exists := q.tasks[key]; exists {
		return false, nil
	}
	if len(q.tasks) >= q.capacity {
		return false, oops.Wrap(entities.ErrQueueFull)
	}
	q.tasks[key] = entry{}
	return true, nil
}

func (q *queue) Release(key uuid.UUID) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.tasks, key)
}

func (q *queue) Pending(key uuid.UUID) (entities.Operation, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	item, exists := q.tasks[key]
	return item.task, exists
}

func (q *queue) remaining() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tasks)
}

func (q *queue) start(task entities.Operation) (attempt, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	item, exists := q.tasks[task.Key]
	if !exists || item.ready || item.claim != 0 || item.attempts != 0 {
		return attempt{}, false
	}
	q.generation++
	item.task, item.claim, item.attempts = task, q.generation, 1
	q.tasks[task.Key] = item
	return attempt{task: task, claim: item.claim, number: item.attempts}, true
}

func (q *queue) readyKeys() []uuid.UUID {
	q.mu.Lock()
	defer q.mu.Unlock()
	keys := make([]uuid.UUID, 0, len(q.tasks))
	for key, item := range q.tasks {
		if item.ready && !q.now().Before(item.nextAttempt) {
			keys = append(keys, key)
		}
	}
	return keys
}

func (q *queue) take(key uuid.UUID) (attempt, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	item, exists := q.tasks[key]
	if !exists || !item.ready || q.now().Before(item.nextAttempt) {
		return attempt{}, false
	}
	q.generation++
	item.ready, item.claim = false, q.generation
	item.attempts++
	q.tasks[key] = item
	return attempt{task: item.task, claim: item.claim, number: item.attempts}, true
}

// finish rejects results from a released or superseded claim of the same key.
func (q *queue) finish(owned attempt, result entities.OperationResult) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	item, exists := q.tasks[owned.task.Key]
	if !exists || owned.claim == 0 || item.claim != owned.claim || result.Task.Key != owned.task.Key {
		return false
	}
	completed := result.Status == entities.OperationCompleted && result.Task.Done() && result.Err == nil
	if completed || result.Status == entities.OperationRejected {
		delete(q.tasks, owned.task.Key)
		return true
	}
	q.tasks[owned.task.Key] = entry{
		task: result.Task, ready: result.Status == entities.OperationDeferred, attempts: item.attempts,
		nextAttempt: q.now().Add(q.retryInterval),
	}
	return true
}
