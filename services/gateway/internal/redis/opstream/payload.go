package opstream

import (
	"bytes"
	"encoding/gob"
	"errors"
	"uuid"

	"lab2/gateway/internal/models/entities"

	"github.com/samber/oops"
)

// actionCode belongs to the Redis wire format, not to the operation model.
type actionCode uint8

const (
	actionUnknown           actionCode = 0
	actionResolveCreation   actionCode = 1
	actionCancelPayment     actionCode = 2
	actionCancelReservation actionCode = 3
	actionRollbackLoyalty   actionCode = 4
	actionDecreaseLoyalty   actionCode = 5
)

type storedAction struct {
	Type    actionCode
	Payload []byte
}

type operationPayload struct {
	Kind    entities.OperationKind
	Key     uuid.UUID
	Actions []storedAction
}

func encodeTask(task entities.Operation) ([]byte, error) {
	payload := operationPayload{Kind: task.Kind, Key: task.Key}
	for _, action := range task.Steps {
		code, err := codeForAction(action)
		if err != nil {
			return nil, err
		}
		var data bytes.Buffer
		if err := gob.NewEncoder(&data).Encode(action.Data()); err != nil {
			return nil, oops.Wrapf(err, "encode operation DTO")
		}
		payload.Actions = append(payload.Actions, storedAction{
			Type: code, Payload: data.Bytes(),
		})
	}
	var buffer bytes.Buffer
	err := gob.NewEncoder(&buffer).Encode(payload)
	return buffer.Bytes(), oops.Wrap(err)
}

func decodeTask(data []byte, task *entities.Operation) error {
	var payload operationPayload
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&payload); err != nil {
		return oops.Wrapf(err, "decode operation payload")
	}
	*task = entities.Operation{Kind: payload.Kind, Key: payload.Key}
	for _, action := range payload.Actions {
		dto, err := decodeAction(action.Type, action.Payload)
		if err != nil {
			return err
		}
		task.Steps = append(task.Steps, dto)
	}
	return nil
}

func decodeAction(kind actionCode, data []byte) (entities.Action, error) {
	switch kind {
	case actionCancelPayment:
		return decodeDTO[entities.CancelPaymentOperation](data)
	case actionCancelReservation:
		return decodeDTO[entities.CancelReservationOperation](data)
	case actionResolveCreation:
		return decodeDTO[entities.ResolveCreationOperation](data)
	case actionRollbackLoyalty:
		return decodeDTO[entities.RollbackLoyaltyOperation](data)
	case actionDecreaseLoyalty:
		return decodeDTO[entities.DecreaseLoyaltyOperation](data)
	case actionUnknown:
		return nil, oops.Wrap(entities.ErrInvalidInput)
	default:
		return nil, oops.Wrap(entities.ErrInvalidInput)
	}
}

func decodeDTO[T entities.OperationPayload](data []byte) (entities.Action, error) {
	var dto T
	err := gob.NewDecoder(bytes.NewReader(data)).Decode(&dto)
	if err != nil {
		return nil, oops.Wrapf(errors.Join(entities.ErrInvalidInput, err), "decode operation DTO")
	}
	return entities.OperationAction[T]{Payload: dto}, nil
}

func codeForAction(action entities.Action) (actionCode, error) {
	if action != nil {
		switch action.Data().(type) {
		case entities.ResolveCreationOperation:
			return actionResolveCreation, nil
		case entities.CancelPaymentOperation:
			return actionCancelPayment, nil
		case entities.CancelReservationOperation:
			return actionCancelReservation, nil
		case entities.RollbackLoyaltyOperation:
			return actionRollbackLoyalty, nil
		case entities.DecreaseLoyaltyOperation:
			return actionDecreaseLoyalty, nil
		}
	}
	return actionUnknown, oops.Wrapf(entities.ErrInvalidInput, "unknown stored action type")
}
