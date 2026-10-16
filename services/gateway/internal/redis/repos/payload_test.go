package repos

import (
	"bytes"
	"encoding/gob"
	"errors"
	"reflect"
	"testing"
	"uuid"

	"lab2/gateway/internal/models/entities"
)

func TestPayloadRejectsInvalidData(t *testing.T) {
	t.Parallel()
	var task entities.Operation
	if err := decodeTask([]byte("invalid gob"), &task); err == nil {
		t.Fatal("invalid payload accepted")
	}
	for _, action := range []storedAction{
		{Type: actionCode(99)},
		{Type: actionCancelPayment, Payload: []byte("invalid gob")},
	} {
		var buffer bytes.Buffer
		if err := gob.NewEncoder(&buffer).Encode(operationPayload{Actions: []storedAction{action}}); err != nil {
			t.Fatal(err)
		}
		if err := decodeTask(buffer.Bytes(), &task); !errors.Is(err, entities.ErrInvalidInput) {
			t.Fatalf("invalid action accepted: %v", err)
		}
	}
}

func TestPayloadRoundTrip(t *testing.T) {
	t.Parallel()
	reservationID, paymentID := uuid.New(), uuid.New()
	creation := entities.NewCreationOperation("user", reservationID, paymentID)
	for _, task := range []entities.Operation{
		creation.ResolveCompensation(),
		creation.Compensation(),
		creation.Group(),
		{
			Kind: entities.OperationCancellation,
			Key:  entities.OperationKey(entities.OperationCancellation, reservationID),
			Steps: []entities.Action{
				entities.NewOperationAction(entities.CancelPaymentOperation{PaymentUID: paymentID}),
				entities.NewOperationAction(entities.CancelReservationOperation{User: "user", ReservationUID: reservationID}),
				entities.NewOperationAction(entities.DecreaseLoyaltyOperation{User: "user", OperationID: "cancel"}),
			},
		},
	} {
		data, err := encodeTask(task)
		if err != nil {
			t.Fatal(err)
		}
		var got entities.Operation
		if err := decodeTask(data, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, task) {
			t.Fatalf("round trip = %+v, want %+v", got, task)
		}
	}
}
