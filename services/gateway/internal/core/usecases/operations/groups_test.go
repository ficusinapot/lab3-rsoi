package operations

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/models/operationifc"
)

func TestGroupPreparationAndFinish(t *testing.T) {
	t.Parallel()
	cause := errors.New("prepare failed")
	for _, tc := range []struct {
		name     string
		prepared bool
	}{
		{name: "first plan failure releases reservation"},
		{name: "later plan failure compensates updated identities", prepared: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			creation := entities.NewCreationOperation("user", uuid.New(), uuid.New())
			current := creation.Group()
			if tc.prepared {
				current = creation.Compensation()
			}
			key := current.Key
			creation.ReservationUID = uuid.New()
			next := creation.ResolveCompensation()
			stream := &groupStream{prepareErr: cause}
			groups := NewGroups(stream, time.Second, nil, nil, nil)
			ctx, cancel := context.WithCancel(t.Context())
			err := groups.prepare(ctx, &current, next)
			if !errors.Is(err, cause) || current.Key != key || current.Done() == tc.prepared {
				t.Fatalf("prepared plan = %+v, error = %v", current, err)
			}
			cancel()
			groups.finish(ctx, &current, &err)
			if !errors.Is(err, cause) || stream.released == tc.prepared || stream.executed != tc.prepared {
				t.Fatalf("finish error = %v, released = %t, executed = %t", err, stream.released, stream.executed)
			}
			if tc.prepared {
				data, decodeErr := entities.OperationData[entities.ResolveCreationOperation](stream.task.Steps[0])
				if decodeErr != nil || data.ReservationUID != creation.ReservationUID {
					t.Fatalf("updated recovery DTO = %+v, error = %v", data, decodeErr)
				}
				if stream.contextErr != nil || !stream.deadline {
					t.Fatalf("rollback context error = %v, deadline = %t", stream.contextErr, stream.deadline)
				}
			}
		})
	}
}

func TestGroupSuccessfulFinishReleasesWithoutCompensation(t *testing.T) {
	t.Parallel()
	cause := errors.New("release failed")
	stream := &groupStream{releaseErr: cause}
	groups := NewGroups(stream, time.Second, nil, nil, nil)
	task := entities.NewCreationOperation("user", uuid.New(), uuid.New()).Compensation()
	var err error
	groups.finish(t.Context(), &task, &err)
	if !stream.released || stream.executed || !errors.Is(err, cause) {
		t.Fatalf("finish error = %v, released = %t, executed = %t", err, stream.released, stream.executed)
	}
}

// groupStream records lifecycle calls without executing downstream operations.
type groupStream struct {
	prepareErr error
	releaseErr error
	released   bool
	executed   bool
	contextErr error
	deadline   bool
	task       entities.Operation
}

func (*groupStream) ReserveOperation(context.Context, uuid.UUID, entities.OperationKind) (bool, error) {
	return true, nil
}

func (s *groupStream) ReleaseOperation(context.Context, uuid.UUID) error {
	s.released = true
	return s.releaseErr
}

func (*groupStream) PendingOperation(context.Context, uuid.UUID) (entities.Operation, bool, error) {
	return entities.Operation{}, false, nil
}

func (s *groupStream) Prepare(context.Context, entities.Operation) error { return s.prepareErr }

func (s *groupStream) Execute(ctx context.Context, task entities.Operation) entities.OperationResult {
	s.executed, s.task, s.contextErr = true, task, ctx.Err()
	_, s.deadline = ctx.Deadline()
	return entities.NewOperationResult(entities.Operation{}, nil)
}

func TestCreationReadFailureReleasesWithoutCompensation(t *testing.T) {
	t.Parallel()
	cause := errors.New("quote failed")
	stream := &groupStream{}
	groups := NewGroups(stream, time.Second, nil, nil, nil)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err := groups.Create(ctx, "user", func(operationifc.Creation) error {
		cancel()
		return cause
	})
	if !errors.Is(err, cause) || !stream.released || stream.executed {
		t.Fatalf("creation error = %v, released = %t, executed = %t", err, stream.released, stream.executed)
	}
}

func TestCreationPanicPreservesRecovery(t *testing.T) {
	t.Parallel()
	cause := errors.New("creation panicked")
	for _, armed := range []bool{false, true} {
		name := "before writes"
		if armed {
			name = "after recovery plan was stored"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			stream := &groupStream{}
			groups := NewGroups(stream, time.Second, nil, nil, nil)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			err := groups.Create(ctx, "user", func(session operationifc.Creation) error {
				if armed {
					creation := session.(*creation)
					if err := groups.prepare(ctx, &creation.task, creation.operation.Compensation()); err != nil {
						return err
					}
				}
				cancel()
				panic(cause)
			})
			if !errors.Is(err, entities.ErrOperationPanic) || !errors.Is(err, cause) {
				t.Fatal("panic cause lost", err)
			}
			if stream.released == armed || stream.executed != armed {
				t.Fatalf("panic recovery: released=%v executed=%v", stream.released, stream.executed)
			}
			if armed && (stream.contextErr != nil || !stream.deadline || stream.task.Done()) {
				t.Fatal("recovery lost its plan or independent timeout", stream.task, stream.contextErr)
			}
		})
	}
}
