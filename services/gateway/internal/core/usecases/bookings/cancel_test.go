package bookings

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"uuid"

	"go.uber.org/mock/gomock"
	model "lab2/gateway/internal/models/entities"
)

func TestCancel(t *testing.T) {
	t.Parallel()
	id, paymentID := uuid.New(), uuid.New()
	ctrl := gomock.NewController(t)
	reservations := NewMockReservations(ctrl)
	payments := NewMockPayments(ctrl)
	loyalty := NewMockLoyalty(ctrl)
	gomock.InOrder(
		reservations.EXPECT().Reservation(gomock.Any(), id, testUser).Return(model.InternalReservation{
			ReservationUID: id, PaymentUID: paymentID, Status: model.ReservationPaid,
		}, nil), payments.EXPECT().CancelPayment(gomock.Any(), paymentID).Return(nil),

		reservations.EXPECT().CancelReservation(gomock.Any(), id, testUser).Return(nil),
		loyalty.EXPECT().ChangeLoyalty(gomock.Any(), testUser, -1, gomock.Any()).Return(nil),
	)
	if err := newTestCommands(reservations, payments, loyalty).Cancel(t.Context(), model.CancelBooking{
		ReservationUID: id, User: testUser,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCancelAlreadyCanceled(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	ctrl := gomock.NewController(t)
	reservations := NewMockReservations(ctrl)
	reservations.EXPECT().Reservation(gomock.Any(), id, testUser).Return(model.InternalReservation{
		ReservationUID: id, Status: model.ReservationCanceled,
	}, nil)
	service := newTestCommands(reservations, NewMockPayments(ctrl), NewMockLoyalty(ctrl))
	if err := service.Cancel(t.Context(), model.CancelBooking{ReservationUID: id, User: testUser}); err != nil {
		t.Fatal(err)
	}
}

func TestCancelStopsAtFailure(t *testing.T) {
	t.Parallel()
	id, paymentID := uuid.New(), uuid.New()
	cause := errors.New("dependency unavailable")
	for _, tc := range []struct {
		name   string
		expect func(*MockReservations, *MockPayments, *MockLoyalty)
	}{
		{
			name: "reservation lookup fails",
			expect: func(reservations *MockReservations, payments *MockPayments, loyalty *MockLoyalty) {
				gomock.InOrder(
					reservations.EXPECT().Reservation(gomock.Any(), id, testUser).Return(model.InternalReservation{
						ReservationUID: id, PaymentUID: paymentID, Status: model.ReservationPaid,
					}, cause),
				)
			},
		},
		{
			name: "reservation cancellation fails",
			expect: func(reservations *MockReservations, payments *MockPayments, loyalty *MockLoyalty) {
				gomock.InOrder(
					reservations.EXPECT().Reservation(gomock.Any(), id, testUser).Return(model.InternalReservation{
						ReservationUID: id, PaymentUID: paymentID, Status: model.ReservationPaid,
					}, nil),
					payments.EXPECT().CancelPayment(gomock.Any(), paymentID).Return(nil),
					reservations.EXPECT().CancelReservation(gomock.Any(), id, testUser).Return(cause),
				)
			},
		},
		{
			name: "payment cancellation fails",
			expect: func(reservations *MockReservations, payments *MockPayments, loyalty *MockLoyalty) {
				gomock.InOrder(
					reservations.EXPECT().Reservation(gomock.Any(), id, testUser).Return(model.InternalReservation{
						ReservationUID: id, PaymentUID: paymentID, Status: model.ReservationPaid,
					}, nil), payments.EXPECT().CancelPayment(gomock.Any(), paymentID).Return(cause),
				)
			},
		},
		{
			name: "loyalty decrement fails",
			expect: func(reservations *MockReservations, payments *MockPayments, loyalty *MockLoyalty) {
				gomock.InOrder(
					reservations.EXPECT().Reservation(gomock.Any(), id, testUser).Return(model.InternalReservation{
						ReservationUID: id, PaymentUID: paymentID, Status: model.ReservationPaid,
					}, nil), payments.EXPECT().CancelPayment(gomock.Any(), paymentID).Return(nil),

					reservations.EXPECT().CancelReservation(gomock.Any(), id, testUser).Return(nil),
					loyalty.EXPECT().ChangeLoyalty(gomock.Any(), testUser, -1, gomock.Any()).Return(cause),
				)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			reservations := NewMockReservations(ctrl)
			payments := NewMockPayments(ctrl)
			loyalty := NewMockLoyalty(ctrl)
			tc.expect(reservations, payments, loyalty)
			err := newTestCommands(reservations, payments, loyalty).Cancel(t.Context(), model.CancelBooking{
				ReservationUID: id, User: testUser,
			})
			if !errors.Is(err, cause) {
				t.Fatalf("error = %v, want cause %v", err, cause)
			}
		})
	}
}

func TestCancelRejectsBlankUser(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	service := newTestCommands(NewMockReservations(ctrl), NewMockPayments(ctrl), NewMockLoyalty(ctrl))
	if err := service.Cancel(t.Context(), model.CancelBooking{
		ReservationUID: uuid.New(), User: " ",
	}); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("error = %v, want invalid input", err)
	}
}

func TestCancelQueuesLoyaltyAndDeduplicatesConcurrentCalls(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	id, paymentID := uuid.New(), uuid.New()
	reservations, payments, loyalty := NewMockReservations(ctrl), NewMockPayments(ctrl), NewMockLoyalty(ctrl)
	row := model.InternalReservation{ReservationUID: id, PaymentUID: paymentID, Status: model.ReservationPaid}
	var calls atomic.Int32
	reservations.EXPECT().Reservation(gomock.Any(), id, testUser).Times(12).DoAndReturn(
		func(context.Context, uuid.UUID, string) (model.InternalReservation, error) {
			item := row
			if calls.Add(1) > 1 {
				item.Status = model.ReservationCanceled
			}
			return item, nil
		})
	gomock.InOrder(
		payments.EXPECT().CancelPayment(gomock.Any(), paymentID).Return(nil),
		reservations.EXPECT().CancelReservation(gomock.Any(), id, testUser).Return(nil),
		loyalty.EXPECT().ChangeLoyalty(gomock.Any(), testUser, -1, id.String()+":cancel").
			Return(model.ErrUpstreamUnavailable),
	)
	service := newTestCommands(reservations, payments, loyalty)
	var workers sync.WaitGroup
	for range 12 {
		workers.Go(func() {
			if err := service.Cancel(t.Context(), model.CancelBooking{ReservationUID: id, User: testUser}); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	task, pending, _ := service.stream.PendingOperation(t.Context(), model.OperationKey(model.OperationCancellation, id))
	if !pending || !model.HasAction[model.DecreaseLoyaltyOperation](task) ||
		model.HasAction[model.CancelPaymentOperation](task) ||
		model.HasAction[model.CancelReservationOperation](task) {
		t.Fatalf("queued task = %+v, pending = %t", task, pending)
	}
}

func TestFullQueueStopsCancellationBeforePayment(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	id := uuid.New()
	reservations := NewMockReservations(ctrl)
	reservations.EXPECT().Reservation(gomock.Any(), id, testUser).Return(model.InternalReservation{
		ReservationUID: id, Status: model.ReservationPaid,
	}, nil)
	service := newTestCommands(reservations, NewMockPayments(ctrl), NewMockLoyalty(ctrl))
	for range 256 {
		reserved, err := service.stream.ReserveOperation(t.Context(), uuid.New(), model.OperationCompensation)
		if err != nil || !reserved {
			t.Fatalf("reserve: reserved=%t, error=%v", reserved, err)
		}
	}
	if err := service.Cancel(t.Context(), model.CancelBooking{
		ReservationUID: id, User: testUser,
	}); !errors.Is(err, model.ErrQueueFull) {
		t.Fatal(err)
	}
}

func TestCancelWaitsForCreationBeforeReadingReservation(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	reservations := NewMockReservations(ctrl)
	service := newTestCommands(reservations, NewMockPayments(ctrl), NewMockLoyalty(ctrl))
	id, paymentID := uuid.New(), uuid.New()
	operation := model.NewCreationOperation(testUser, id, paymentID)
	reserved, err := service.stream.ReserveOperation(t.Context(), operation.Group().Key, model.OperationCompensation)
	if err != nil || !reserved {
		t.Fatal(reserved, err)
	}
	if err := service.stream.Prepare(t.Context(), operation.Compensation()); err != nil {
		t.Fatal(err)
	}
	input := model.CancelBooking{User: testUser, ReservationUID: id}
	if err := service.Cancel(t.Context(), input); !errors.Is(err, model.ErrOperationBusy) {
		t.Fatal("unfinished creation did not block cancellation", err)
	}
	// Creation may have rolled back. The retry must read the current row, not a prior PAID snapshot.
	if err := service.stream.ReleaseOperation(t.Context(), operation.Group().Key); err != nil {
		t.Fatal(err)
	}
	reservations.EXPECT().Reservation(gomock.Any(), id, testUser).Return(model.InternalReservation{
		ReservationUID: id, PaymentUID: paymentID, Status: model.ReservationCanceled,
	}, nil)
	if err := service.Cancel(t.Context(), input); err != nil {
		t.Fatal(err)
	}
}
