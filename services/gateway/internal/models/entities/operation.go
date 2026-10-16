package entities

import (
	"crypto/sha256"
	"errors"
	"time"
	"uuid"

	"github.com/samber/oops"
)

func NewOperationPanic(value any, stack []byte) error {
	cause, _ := value.(error)
	return oops.With("panic", value, "stack", string(stack)).
		Wrapf(errors.Join(ErrOperationPanic, cause), "operation panicked")
}

type OperationKind uint8

const (
	OperationKindUnknown OperationKind = iota
	OperationCompensation
	OperationCancellation
)

// Operation contains queue metadata and a remaining group of typed operations.
// Executors replace Steps instead of modifying its backing array.
type Operation struct {
	Kind  OperationKind
	Key   uuid.UUID
	Steps []Action
}

func (r Operation) Done() bool { return len(r.Steps) == 0 }

func HasAction[T OperationPayload](operation Operation) bool {
	for _, action := range operation.Steps {
		if _, ok := action.(OperationAction[T]); ok {
			return true
		}
	}
	return false
}

func (r Operation) Validate() error {
	if r.Kind != OperationCompensation && r.Kind != OperationCancellation {
		return oops.Wrapf(ErrInvalidInput, "unknown operation kind")
	}
	if r.Key == uuid.Nil() {
		return oops.Wrapf(ErrInvalidInput, "invalid operation key")
	}
	previous := -1
	for _, action := range r.Steps {
		if action == nil {
			return oops.Wrapf(ErrInvalidInput, "missing operation action")
		}
		index := r.actionIndex(action)
		if index < 0 || index <= previous {
			return oops.Wrapf(ErrInvalidInput, "invalid operation action order")
		}
		previous = index
		if err := action.Validate(); err != nil {
			return oops.Wrap(err)
		}
	}
	return nil
}

// actionIndex defines business ordering, not a persistent action identifier.
func (r Operation) actionIndex(action Action) int {
	switch action.(type) {
	case OperationAction[CancelPaymentOperation]:
		if r.Kind == OperationCompensation {
			return 2
		}
		return 0
	case OperationAction[CancelReservationOperation]:
		return 1
	case OperationAction[DecreaseLoyaltyOperation]:
		if r.Kind == OperationCancellation {
			return 2
		}
	case OperationAction[ResolveCreationOperation]:
		if r.Kind == OperationCompensation {
			return 0
		}
	case OperationAction[RollbackLoyaltyOperation]:
		if r.Kind == OperationCompensation {
			return 3
		}
	}
	return -1
}

type ExecutionMode uint8

const (
	ExecutionUnknown ExecutionMode = iota
	ExecutionInitial
	ExecutionBackground
)

type OperationStatus uint8

const (
	OperationUnknown OperationStatus = iota
	OperationCompleted
	OperationDeferred
	OperationFailed   // Retained for manual retry.
	OperationRejected // Failed initial request; release its queue slot.
)

// OperationResult carries the remaining plan independently of queue ownership.
type OperationResult struct {
	Task   Operation
	Status OperationStatus
	Err    error
}

func NewOperationResult(task Operation, err error) OperationResult {
	status := OperationDeferred
	if task.Done() && err == nil {
		status = OperationCompleted
	}
	return OperationResult{Task: task, Status: status, Err: err}
}

type OperationInfo struct {
	Key     uuid.UUID
	State   string
	Owner   string
	Attempt uint64
	Until   time.Time
	Error   string
	Task    *Operation
}

// OperationKey is stable for retries and separates creation recovery from cancellation.
func OperationKey(kind OperationKind, reservationUID uuid.UUID) uuid.UUID {
	data := append([]byte{byte(kind)}, reservationUID[:]...)
	digest := sha256.Sum256(data)
	var key uuid.UUID
	copy(key[:], digest[:16])
	key[6] = (key[6] & 0x0f) | 0x80 // UUIDv8: application-defined, namespaced digest.
	key[8] = (key[8] & 0x3f) | 0x80
	return key
}
