//go:build integration

package operations

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/redis/repos"
)

func TestConcurrentTransactions(t *testing.T) {
	t.Parallel()
	container, err := testcontainers.Run(t.Context(), "redis:7.4-alpine",
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("6379/tcp")))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, container)
	url := redisURL(t, container)

	t.Run("concurrent reservations respect both budgets", func(t *testing.T) {
		t.Parallel()
		prefix := "{" + uuid.New().String() + "}:"
		stores := []*repos.Store{newStore(t, url, prefix), newStore(t, url, prefix)}
		type result struct {
			kind     entities.OperationKind
			accepted bool
			err      error
		}
		results := make(chan result, 32)
		begin := make(chan struct{})
		for i := range 32 {
			go func() {
				<-begin
				kind := entities.OperationCancellation
				if i%2 == 0 {
					kind = entities.OperationCompensation
				}
				accepted, err := stores[i%len(stores)].Reserve(t.Context(), uuid.New(), kind)
				results <- result{kind: kind, accepted: accepted, err: err}
			}()
		}
		close(begin)
		accepted := map[entities.OperationKind]int{}
		for range 32 {
			result := <-results
			if result.err != nil && !errors.Is(result.err, entities.ErrQueueFull) {
				t.Error(result.err)
			}
			if result.accepted {
				accepted[result.kind]++
			}
		}
		if accepted[entities.OperationCompensation] != 2 || accepted[entities.OperationCancellation] != 2 {
			t.Fatal("concurrent admission exceeded budgets", accepted)
		}
		if count, err := stores[0].Remaining(t.Context()); err != nil || count != 4 {
			t.Fatal(count, err)
		}
	})

	t.Run("concurrent publication and completion do not duplicate messages", func(t *testing.T) {
		t.Parallel()
		prefix := "{" + uuid.New().String() + "}:"
		first, second := newStore(t, url, prefix), newStore(t, url, prefix)
		for range 2 {
			task := paymentTask()
			initial := start(t, first, task)
			if ok, err := first.Finish(t.Context(), initial,
				entities.NewOperationResult(task, entities.ErrPaymentUnavailable)); !ok || err != nil {
				t.Fatal(ok, err)
			}
		}
		time.Sleep(operationConfig().RetryInterval + 20*time.Millisecond)
		type delivery struct {
			store *repos.Store
			keys  []uuid.UUID
			err   error
		}
		deliveries := make(chan delivery, 2)
		begin := make(chan struct{})
		for _, store := range []*repos.Store{first, second} {
			go func() {
				<-begin
				keys, err := store.ReadyKeys(t.Context(), "compensation")
				deliveries <- delivery{store: store, keys: keys, err: err}
			}()
		}
		close(begin)
		seen := map[uuid.UUID]bool{}
		for range 2 {
			delivery := <-deliveries
			if delivery.err != nil || len(delivery.keys) != 1 {
				t.Fatal(delivery.keys, delivery.err)
			}
			key := delivery.keys[0]
			if seen[key] {
				t.Fatal("duplicate stream delivery", key)
			}
			seen[key] = true
			attempt, ok, err := delivery.store.Take(t.Context(), key)
			if !ok || err != nil {
				t.Fatal(attempt, ok, err)
			}
			done := attempt.Task
			done.Steps = nil
			var workers sync.WaitGroup
			results := make(chan bool, 2)
			failures := make(chan error, 2)
			for range 2 {
				workers.Go(func() {
					accepted, err := delivery.store.Finish(t.Context(), attempt, entities.NewOperationResult(done, nil))
					results <- accepted
					failures <- err
				})
			}
			workers.Wait()
			if first, second := <-results, <-results; first == second {
				t.Fatal("completion was not exclusive", first, second)
			}
			for range 2 {
				if err := <-failures; err != nil {
					t.Fatal(err)
				}
			}
		}
		options, err := redis.ParseURL(url)
		if err != nil {
			t.Fatal(err)
		}
		raw := redis.NewClient(options)
		t.Cleanup(func() { _ = raw.Close() })
		if length, err := raw.XLen(t.Context(), prefix+"stream:compensation").Result(); err != nil || length != 0 {
			t.Fatal("publication left duplicate messages", length, err)
		}
		if count, err := first.Remaining(t.Context()); err != nil || count != 0 {
			t.Fatal(count, err)
		}
	})

	t.Run("queue works without scripting permissions", func(t *testing.T) {
		t.Parallel()
		options, err := redis.ParseURL(url)
		if err != nil {
			t.Fatal(err)
		}
		raw := redis.NewClient(options)
		t.Cleanup(func() { _ = raw.Close() })
		user := uuid.New().String()
		err = raw.Do(t.Context(), "ACL", "SETUSER", user, "on", ">test", "~*", "+@all", "-eval", "-evalsha").Err()
		if err != nil {
			t.Fatal(err)
		}
		restrictedURL := strings.Replace(url, "redis://", "redis://"+user+":test@", 1)
		store := newStore(t, restrictedURL, "{"+uuid.New().String()+"}:")
		task := paymentTask()
		initial := start(t, store, task)
		if ok, err := store.Finish(t.Context(), initial,
			entities.NewOperationResult(task, entities.ErrPaymentUnavailable)); !ok || err != nil {
			t.Fatal(ok, err)
		}
		time.Sleep(operationConfig().RetryInterval + 20*time.Millisecond)
		if keys, err := store.ReadyKeys(t.Context(), "compensation"); err != nil || len(keys) != 1 {
			t.Fatal(keys, err)
		}
		attempt, ok, err := store.Take(t.Context(), task.Key)
		if !ok || err != nil {
			t.Fatal(attempt, ok, err)
		}
		task.Steps = nil
		if ok, err := store.Finish(t.Context(), attempt, entities.NewOperationResult(task, nil)); !ok || err != nil {
			t.Fatal(ok, err)
		}
	})
}
