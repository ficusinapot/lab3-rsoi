package opstream

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"lab2/gateway/internal/models/entities"
)

func queueConfig() Config {
	return Config{
		Capacity: 1, Concurrency: 1, RetryInterval: 10 * time.Second,
		AttemptTimeout: time.Second, RollbackTimeout: time.Second,
		StorageTimeout:      5 * time.Second,
		CompensationWorkers: 1,
		PollInterval:        10 * time.Second,
		ShutdownTimeout:     20 * time.Second,
	}
}

func TestCapacityAndDeduplication(t *testing.T) {
	t.Parallel()
	q := newQueue(queueConfig())
	if ok, err := q.Reserve(testKey("a")); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, err := q.Reserve(testKey("a")); ok || err != nil {
		t.Fatal(ok, err)
	}
	if _, err := q.Reserve(testKey("b")); !errors.Is(err, entities.ErrQueueFull) {
		t.Fatal(err)
	}
	if _, ok := q.take(testKey("a")); ok {
		t.Fatal("unsent reservation is executable")
	}
	q.Release(testKey("a"))
	if ok, err := q.Reserve(testKey("b")); !ok || err != nil {
		t.Fatal(ok, err)
	}
	owned, _ := q.start(entities.Operation{
		Key: testKey("b"),
	})
	q.finish(owned, entities.NewOperationResult(owned.task, nil))
	if _, exists := q.Pending(testKey("b")); exists {
		t.Fatal("completed task consumes capacity")
	}
}

func TestConcurrentClaim(t *testing.T) {
	t.Parallel()
	q := newQueue(queueConfig())
	_, _ = q.Reserve(testKey("a"))
	task := entities.Operation{
		Kind: entities.OperationCancellation,
		Key:  testKey("a"),
		Steps: []entities.Action{
			entities.NewOperationAction(entities.DecreaseLoyaltyOperation{}),
		},
	}
	now := time.Unix(0, 0)
	q.now = func() time.Time { return now }
	deferOperation(t, q, task)
	now = now.Add(queueConfig().RetryInterval)
	var claims atomic.Int32
	var workers sync.WaitGroup
	for range 12 {
		workers.Go(func() {
			if _, ok := q.take(testKey("a")); ok {
				claims.Add(1)
			}
		})
	}
	workers.Wait()
	if claims.Load() != 1 {
		t.Fatalf("claims = %d, want 1", claims.Load())
	}
	if _, ok := q.start(entities.Operation{
		Key: testKey("a"),
	}); ok {
		t.Fatal("producer restarted an owned attempt")
	}
	if pending, ok := q.Pending(testKey("a")); !ok || !reflect.DeepEqual(pending, task) {
		t.Fatal("producer changed an owned attempt", pending, ok)
	}
	if _, err := q.Reserve(testKey("b")); !errors.Is(err, entities.ErrQueueFull) {
		t.Fatal("running attempt lost reserved capacity", err)
	}
}

func TestStaleResultCannotOverwriteReusedKey(t *testing.T) {
	t.Parallel()
	q := newQueue(queueConfig())
	_, _ = q.Reserve(testKey("a"))
	old, ok := q.start(entities.Operation{
		Key: testKey("a"),
		Steps: []entities.Action{
			entities.NewOperationAction(entities.CancelPaymentOperation{}),
		},
	})
	if !ok {
		t.Fatal("missing old attempt")
	}
	q.Release(testKey("a"))
	_, _ = q.Reserve(testKey("a"))
	fresh := entities.Operation{
		Key: testKey("a"),
		Steps: []entities.Action{
			entities.NewOperationAction(entities.DecreaseLoyaltyOperation{OperationID: "new"}),
		},
	}
	current, ok := q.start(fresh)
	if !ok {
		t.Fatal("missing new attempt")
	}
	if q.finish(old, entities.NewOperationResult(entities.Operation{
		Key: testKey("a"),
	}, nil)) {
		t.Fatal("stale completion accepted")
	}
	old.task.Steps = []entities.Action{entities.NewOperationAction(entities.CancelReservationOperation{})}
	if q.finish(old, entities.NewOperationResult(old.task, entities.ErrUpstreamUnavailable)) {
		t.Fatal("stale failure accepted")
	}
	if pending, ok := q.Pending(testKey("a")); !ok || !reflect.DeepEqual(pending, fresh) {
		t.Fatal("new task overwritten", pending, ok)
	}
	current.task.Steps = nil
	result := entities.NewOperationResult(current.task, nil)
	if !q.finish(current, result) || q.finish(current, result) {
		t.Fatal("completion was not applied exactly once")
	}
	if ok, err := q.Reserve(testKey("b")); !ok || err != nil {
		t.Fatal("completed claim did not release capacity", ok, err)
	}
}

func TestDeferredResultRetainsOutstandingWork(t *testing.T) {
	t.Parallel()
	q := newQueue(queueConfig())
	now := time.Unix(0, 0)
	q.now = func() time.Time { return now }
	_, _ = q.Reserve(testKey("a"))
	owned, ok := q.start(entities.Operation{
		Key: testKey("a"),
		Steps: []entities.Action{
			entities.NewOperationAction(entities.CancelPaymentOperation{}),
			entities.NewOperationAction(entities.CancelReservationOperation{}),
		},
	})
	if !ok {
		t.Fatal("missing attempt")
	}
	owned.task.Steps = owned.task.Steps[1:]
	if !q.finish(owned, entities.NewOperationResult(owned.task, entities.ErrUpstreamUnavailable)) {
		t.Fatal("deferred result rejected")
	}
	if _, ok := q.take(testKey("a")); ok || len(q.readyKeys()) != 0 {
		t.Fatal("deferred work retried before interval")
	}
	now = now.Add(queueConfig().RetryInterval)
	next, ok := q.take(testKey("a"))
	if !ok || entities.HasAction[entities.CancelPaymentOperation](next.task) ||
		!entities.HasAction[entities.CancelReservationOperation](next.task) {
		t.Fatal("lost remaining work", next, ok)
	}
	if owned.number != 1 || next.number != 2 {
		t.Fatal("attempt count did not advance", owned.number, next.number)
	}
	if _, err := q.Reserve(testKey("b")); !errors.Is(err, entities.ErrQueueFull) {
		t.Fatal("deferred task did not retain capacity", err)
	}
}

// deferOperation records a failed initial attempt through the production lifecycle.
func deferOperation(t *testing.T, q *queue, task entities.Operation) {
	t.Helper()
	owned, ok := q.start(task)
	if !ok {
		t.Fatal("missing reserved operation", task.Key)
	}
	if !q.finish(owned, entities.NewOperationResult(task, entities.ErrUpstreamUnavailable)) {
		t.Fatal("initial failure was not deferred", task.Key)
	}
}
