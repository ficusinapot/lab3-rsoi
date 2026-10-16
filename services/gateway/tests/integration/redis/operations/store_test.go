//go:build integration

package operations

import (
	"context"
	"errors"
	"net"
	"reflect"
	"slices"
	"testing"
	"time"
	"uuid"

	"github.com/redis/go-redis/v9"
	"github.com/samber/oops"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"lab2/gateway/internal/core/services/opstream"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/opstreamifc"
	"lab2/gateway/internal/redis/repos"
)

const fixtureUser = "user"

func operationConfig() opstream.Config {
	return opstream.Config{
		Capacity: 4, CompensationCapacity: 2, Concurrency: 4,
		CompensationWorkers: 2, CancellationWorkers: 1, LoyaltyWorkers: 1,
		PollInterval: 10 * time.Millisecond, LeaseDuration: 3 * time.Second,
		StorageTimeout: time.Second, HistoryTTL: time.Hour, RetryInterval: 50 * time.Millisecond,
		AttemptTimeout: time.Second, RollbackTimeout: time.Second, ShutdownTimeout: 5 * time.Second,
	}
}

func redisURL(t *testing.T, container testcontainers.Container) string {
	t.Helper()
	host, err := container.Host(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(t.Context(), "6379/tcp")
	if err != nil {
		t.Fatal(err)
	}
	return "redis://" + net.JoinHostPort(host, port.Port()) + "/0"
}

func newStore(t *testing.T, url, prefix string) *repos.Store {
	t.Helper()
	store, err := repos.New(t.Context(), repos.Config{
		URL: url, Prefix: prefix, PoolSize: 4, DialTimeout: time.Second,
	}, operationConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store
}

func reserve(t *testing.T, store *repos.Store, key uuid.UUID, kind entities.OperationKind) {
	t.Helper()
	ok, err := store.Reserve(t.Context(), key, kind)
	if !ok || err != nil {
		t.Fatalf("reserve %s = %v, %v", key, ok, err)
	}
}

func start(t *testing.T, store *repos.Store, task entities.Operation) opstreamifc.Attempt {
	t.Helper()
	reserve(t, store, testKey(task.Key), task.Kind)
	attempt, ok, err := store.Start(t.Context(), task)
	if !ok || err != nil {
		t.Fatalf("start = %+v, %v, %v", attempt, ok, err)
	}
	return attempt
}

func paymentTask() entities.Operation {
	return entities.Operation{
		Kind: entities.OperationCompensation,
		Key:  testKey(uuid.New().String()),
		Steps: []entities.Action{
			entities.NewOperationAction(entities.CancelPaymentOperation{PaymentUID: uuid.New()}),
		},
	}
}

//nolint:paralleltest // Subtests share a Redis container; recovery cases restart it.
func TestRedisOperations(t *testing.T) {
	t.Parallel()
	container, err := testcontainers.Run(t.Context(), "redis:7.4-alpine",
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithCmd("redis-server", "--appendonly", "yes", "--appendfsync", "always"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("6379/tcp")))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, container)
	url := redisURL(t, container)

	t.Run("separate budgets and duplicate keys", func(t *testing.T) {
		store := newStore(t, url, "{"+uuid.New().String()+"}:")
		for _, key := range []string{"comp-a", "comp-b"} {
			reserve(t, store, testKey(key), entities.OperationCompensation)
		}
		_, compensationErr := store.Reserve(t.Context(), testKey("comp-c"), entities.OperationCompensation)
		if !errors.Is(compensationErr, entities.ErrQueueFull) {
			t.Fatalf("compensation budget = %v", compensationErr)
		}
		for _, key := range []string{"cancel-a", "cancel-b"} {
			reserve(t, store, testKey(key), entities.OperationCancellation)
		}
		_, cancellationErr := store.Reserve(t.Context(), testKey("cancel-c"), entities.OperationCancellation)
		if !errors.Is(cancellationErr, entities.ErrQueueFull) {
			t.Fatalf("normal budget = %v", cancellationErr)
		}
		if ok, err := store.Reserve(t.Context(), testKey("comp-a"), entities.OperationCompensation); ok || err != nil {
			t.Fatalf("duplicate reserve = %v, %v", ok, err)
		}
		if count, err := store.Remaining(t.Context()); count != 4 || err != nil {
			t.Fatal(count, err)
		}
	})

	t.Run("expired lease fences old owner", func(t *testing.T) {
		prefix := "{" + uuid.New().String() + "}:"
		first, second := newStore(t, url, prefix), newStore(t, url, prefix)
		task := paymentTask()
		old := start(t, first, task)
		time.Sleep(operationConfig().LeaseDuration + 20*time.Millisecond)
		keys, err := second.ReadyKeys(t.Context(), "compensation")
		if err != nil || !slices.Contains(keys, task.Key) {
			t.Fatal(keys, err)
		}
		current, ok, err := second.Take(t.Context(), testKey(task.Key))
		if !ok || err != nil || current.Token == old.Token || current.Number != 2 {
			t.Fatal(current, ok, err)
		}
		done := task
		done.Steps = nil
		result := entities.NewOperationResult(done, nil)
		if accepted, err := first.Finish(t.Context(), old, result); accepted || err != nil {
			t.Fatal(accepted, err)
		}
		if accepted, err := second.Finish(t.Context(), current, result); !accepted || err != nil {
			t.Fatal(accepted, err)
		}
		if count, err := second.Remaining(t.Context()); count != 0 || err != nil {
			t.Fatal(count, err)
		}
	})

	t.Run("failed task needs explicit retry", func(t *testing.T) {
		store := newStore(t, url, "{"+uuid.New().String()+"}:")
		task := paymentTask()
		owned := start(t, store, task)
		failed := entities.NewOperationResult(task, entities.ErrInvalidInput)
		failed.Status = entities.OperationFailed
		if ok, err := store.Finish(t.Context(), owned, failed); !ok || err != nil {
			t.Fatal(ok, err)
		}
		keys, err := store.ReadyKeys(t.Context(), "compensation")
		if err != nil || slices.Contains(keys, task.Key) {
			t.Fatal(keys, err)
		}
		if count, err := store.Remaining(t.Context()); count != 1 || err != nil {
			t.Fatal(count, err)
		}
		if err := store.RetryFailed(t.Context(), testKey(task.Key)); err != nil {
			t.Fatal(err)
		}
		keys, err = store.ReadyKeys(t.Context(), "compensation")
		if err != nil || !slices.Contains(keys, task.Key) {
			t.Fatal(keys, err)
		}
		current, ok, err := store.Take(t.Context(), testKey(task.Key))
		if !ok || err != nil {
			t.Fatal(current, ok, err)
		}
		task.Steps = nil
		if ok, err := store.Finish(t.Context(), current, entities.NewOperationResult(task, nil)); !ok || err != nil {
			t.Fatal(ok, err)
		}
	})

	t.Run("rejected initial task releases capacity", func(t *testing.T) {
		store := newStore(t, url, "{"+uuid.New().String()+"}:")
		task := paymentTask()
		owned := start(t, store, task)
		rejected := entities.NewOperationResult(task, entities.ErrPaymentUnavailable)
		rejected.Status = entities.OperationRejected
		if ok, err := store.Finish(t.Context(), owned, rejected); !ok || err != nil {
			t.Fatal(ok, err)
		}
		if count, err := store.Remaining(t.Context()); count != 0 || err != nil {
			t.Fatal(count, err)
		}
		if _, pending, err := store.Pending(t.Context(), task.Key); pending || err != nil {
			t.Fatal(pending, err)
		}
	})

	t.Run("remaining loyalty uses isolated lane", func(t *testing.T) {
		store := newStore(t, url, "{"+uuid.New().String()+"}:")
		task := entities.Operation{
			Kind: entities.OperationCancellation,
			Key:  testKey(uuid.New().String()),
			Steps: []entities.Action{
				entities.NewOperationAction(entities.CancelPaymentOperation{PaymentUID: uuid.New()}),
				entities.NewOperationAction(entities.CancelReservationOperation{User: fixtureUser, ReservationUID: uuid.New()}),
				entities.NewOperationAction(entities.DecreaseLoyaltyOperation{User: fixtureUser, OperationID: "booking:cancel"}),
			},
		}
		owned := start(t, store, task)
		task.Steps = task.Steps[len(task.Steps)-1:]
		accepted, finishErr := store.Finish(
			t.Context(), owned, entities.NewOperationResult(task, entities.ErrLoyaltyUnavailable),
		)
		if !accepted || finishErr != nil {
			t.Fatal(accepted, finishErr)
		}
		time.Sleep(operationConfig().RetryInterval + 20*time.Millisecond)
		keys, err := store.ReadyKeys(t.Context(), "loyalty")
		if err != nil || !slices.Contains(keys, task.Key) {
			t.Fatal(keys, err)
		}
		keys, err = store.ReadyKeys(t.Context(), "cancellation")
		if err != nil || slices.Contains(keys, task.Key) {
			t.Fatal(keys, err)
		}
	})

	t.Run("streams distribute reclaim and acknowledge deliveries", func(t *testing.T) {
		prefix := "{" + uuid.New().String() + "}:"
		first, second := newStore(t, url, prefix), newStore(t, url, prefix)
		options, err := redis.ParseURL(url)
		if err != nil {
			t.Fatal(err)
		}
		raw := redis.NewClient(options)
		t.Cleanup(func() { _ = raw.Close() })
		stream := prefix + "stream:compensation"
		task := paymentTask()
		initial := start(t, first, task)
		if ok, err := first.Finish(t.Context(), initial,
			entities.NewOperationResult(task, entities.ErrPaymentUnavailable)); !ok || err != nil {
			t.Fatal(ok, err)
		}
		if keys, err := first.ReadyKeys(t.Context(), "compensation"); err != nil || len(keys) != 0 {
			t.Fatal("retry delay bypassed", keys, err)
		}
		time.Sleep(operationConfig().RetryInterval + 20*time.Millisecond)
		keys, err := first.ReadyKeys(t.Context(), "compensation")
		if err != nil || !slices.Equal(keys, []uuid.UUID{task.Key}) {
			t.Fatal(keys, err)
		}
		pending, err := raw.XPending(t.Context(), stream, "workers").Result()
		if err != nil || pending.Count != 1 {
			t.Fatal("delivery missing from PEL", pending, err)
		}
		if keys, err := second.ReadyKeys(t.Context(), "compensation"); err != nil || len(keys) != 0 {
			t.Fatal("delivery issued twice", keys, err)
		}
		old, ok, err := first.Take(t.Context(), task.Key)
		if !ok || err != nil {
			t.Fatal(old, ok, err)
		}
		// Simulate a worker dying after Take; Redis owns the unacknowledged delivery.
		time.Sleep(operationConfig().LeaseDuration + 20*time.Millisecond)
		// PEL ownership changes do not invalidate WATCH on the stream itself.
		// The adapter must invalidate this in-flight transaction via the version key.
		err = raw.Watch(t.Context(), func(tx *redis.Tx) error {
			var receiveErr error
			keys, receiveErr = second.ReadyKeys(t.Context(), "compensation")
			if receiveErr != nil {
				return oops.Wrap(receiveErr)
			}
			_, execErr := tx.TxPipelined(t.Context(), func(pipe redis.Pipeliner) error {
				pipe.HSet(t.Context(), prefix+"record:"+task.Key.String(), "error", "stale result")
				return nil
			})
			return oops.Wrap(execErr)
		}, stream, stream+":version")
		if !errors.Is(err, redis.TxFailedErr) {
			t.Fatal("reclaim did not invalidate the stale transaction", err)
		}
		if !slices.Equal(keys, []uuid.UUID{task.Key}) {
			t.Fatal("pending delivery not reclaimed", keys)
		}
		if _, ok, err := first.Take(t.Context(), task.Key); ok || err != nil {
			t.Fatal("old consumer took reclaimed delivery", ok, err)
		}
		current, ok, err := second.Take(t.Context(), task.Key)
		if !ok || err != nil || current.Token == old.Token || current.Number != old.Number+1 {
			t.Fatal(current, ok, err)
		}
		task.Steps = nil
		result := entities.NewOperationResult(task, nil)
		if ok, err := first.Finish(t.Context(), old, result); ok || err != nil {
			t.Fatal("stale result accepted", ok, err)
		}
		if ok, err := second.Finish(t.Context(), current, result); !ok || err != nil {
			t.Fatal(ok, err)
		}
		pending, err = raw.XPending(t.Context(), stream, "workers").Result()
		if err != nil || pending.Count != 0 {
			t.Fatal("completed delivery not acknowledged", pending, err)
		}
		if length, err := raw.XLen(t.Context(), stream).Result(); err != nil || length != 0 {
			t.Fatal("completed entry not removed", length, err)
		}
	})

	t.Run("prepared payload survives Redis restart", func(t *testing.T) {
		prefix := "{" + uuid.New().String() + "}:"
		store := newStore(t, url, prefix)
		task := paymentTask()
		reserve(t, store, testKey(task.Key), task.Kind)
		if err := store.Prepare(t.Context(), task); err != nil {
			t.Fatal(err)
		}
		if err := container.Stop(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		if err := container.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		restarted := newStore(t, redisURL(t, container), prefix)
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			if _, err := restarted.ReadyKeys(ctx, "compensation"); err != nil {
				t.Fatal(err)
			}
			owned, ok, err := restarted.Take(ctx, testKey(task.Key))
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				if !entities.HasAction[entities.CancelPaymentOperation](owned.Task) ||
					!reflect.DeepEqual(owned.Task.Steps, task.Steps) {
					t.Fatalf("recovered task = %+v", owned.Task)
				}
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal("prepared task did not recover")
			case <-ticker.C:
			}
		}
	})

	t.Run("pending stream delivery survives Redis restart", func(t *testing.T) {
		prefix := "{" + uuid.New().String() + "}:"
		store := newStore(t, redisURL(t, container), prefix)
		task := paymentTask()
		initial := start(t, store, task)
		if ok, err := store.Finish(t.Context(), initial,
			entities.NewOperationResult(task, entities.ErrPaymentUnavailable)); !ok || err != nil {
			t.Fatal(ok, err)
		}
		time.Sleep(operationConfig().RetryInterval + 20*time.Millisecond)
		if keys, err := store.ReadyKeys(t.Context(), "compensation"); err != nil || !slices.Contains(keys, task.Key) {
			t.Fatal(keys, err)
		}
		old, ok, err := store.Take(t.Context(), task.Key)
		if !ok || err != nil {
			t.Fatal(old, ok, err)
		}
		if err := container.Stop(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		if err := container.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		restarted := newStore(t, redisURL(t, container), prefix)
		time.Sleep(operationConfig().LeaseDuration + 20*time.Millisecond)
		keys, err := restarted.ReadyKeys(t.Context(), "compensation")
		if err != nil || !slices.Equal(keys, []uuid.UUID{task.Key}) {
			t.Fatal("AOF lost pending delivery", keys, err)
		}
		current, ok, err := restarted.Take(t.Context(), task.Key)
		if !ok || err != nil || current.Token == old.Token || !reflect.DeepEqual(current.Task, task) {
			t.Fatal(current, ok, err)
		}
		task.Steps = nil
		if ok, err := restarted.Finish(t.Context(), current, entities.NewOperationResult(task, nil)); !ok || err != nil {
			t.Fatal(ok, err)
		}
	})
}
