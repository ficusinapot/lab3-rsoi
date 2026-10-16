package bookings

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"
	"uuid"

	"go.uber.org/mock/gomock"
	"lab2/gateway/internal/core/services/opstream"
	"lab2/gateway/internal/core/usecases/operations"
	model "lab2/gateway/internal/models/entities"
)

func TestCreationCompensationContinuesThroughWorker(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		r, p, l := NewMockReservations(ctrl), NewMockPayments(ctrl), NewMockLoyalty(ctrl)
		hotelID, paymentID, id := uuid.New(), uuid.New(), uuid.New()
		r.EXPECT().Hotel(gomock.Any(), hotelID).Return(model.Hotel{Price: 100}, nil)
		l.EXPECT().Loyalty(gomock.Any(), testUser).Return(model.Loyalty{Discount: 5}, nil)
		p.EXPECT().CreatePayment(gomock.Any(), paymentCreateMatcher(95)).
			Return(model.InternalPayment{PaymentUID: paymentID}, nil)
		r.EXPECT().CreateReservation(gomock.Any(), testUser, gomock.Any()).
			Return(model.InternalReservation{ReservationUID: id}, nil)
		l.EXPECT().ChangeLoyalty(gomock.Any(), testUser, 1, id.String()+":create").
			Return(model.ErrUpstreamUnavailable)
		gomock.InOrder(
			r.EXPECT().CancelReservation(gomock.Any(), id, testUser).Return(model.ErrUpstreamUnavailable),
			p.EXPECT().CancelPayment(gomock.Any(), paymentID).Return(nil),
			r.EXPECT().CancelReservation(gomock.Any(), id, testUser).Return(nil),
		)
		cfg := opstream.Config{
			Capacity: 1, Concurrency: 1, RetryInterval: 10 * time.Second,
			AttemptTimeout: 5 * time.Second, RollbackTimeout: 5 * time.Second,
			StorageTimeout:      5 * time.Second,
			CompensationWorkers: 1,
			PollInterval:        10 * time.Second,
			ShutdownTimeout:     20 * time.Second,
		}
		l.EXPECT().ChangeLoyalty(gomock.Any(), testUser, -1, gomock.Any()).Return(nil)
		l.EXPECT().ResolveCreation(gomock.Any(), testUser, id).Return(false, nil)
		processor := operations.New(operations.NewActions(r, p, l))
		stream := opstream.NewMemory(cfg, processor, slog.New(slog.NewTextHandler(io.Discard, nil)))
		service := NewCommands(r, l, operations.NewGroups(stream, cfg.RollbackTimeout, r, p, l))
		start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
		_, err := service.Create(t.Context(), model.CreateBooking{
			User:     testUser,
			HotelUID: hotelID, StartDate: start, EndDate: start.AddDate(0, 0, 1),
		})
		if !errors.Is(err, model.ErrUpstreamUnavailable) {
			t.Fatalf("create error = %v", err)
		}
		_, err = stream.ReserveOperation(t.Context(), testKey("probe"), model.OperationCompensation)
		if !errors.Is(err, model.ErrQueueFull) {
			t.Fatalf("unfinished compensation must occupy capacity: %v", err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go stream.Run(ctx)
		synctest.Wait()
		time.Sleep(cfg.RetryInterval)
		synctest.Wait()
		reserved, err := stream.ReserveOperation(t.Context(), testKey("probe"), model.OperationCompensation)
		if !reserved || err != nil {
			t.Fatalf("completed compensation must release capacity: %v, %v", reserved, err)
		}
		_ = stream.ReleaseOperation(t.Context(), testKey("probe"))
		cancel()
		synctest.Wait()
	})
}

func TestReadFallbackOnlyForAvailabilityFailures(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{
		model.ErrUpstreamUnavailable, model.ErrUpstreamTimeout,
		model.ErrInvalidInput, model.ErrNotFound, context.Canceled, errors.New("unexpected failure"),
	} {
		t.Run(cause.Error(), func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			row := readRows(1)[0]
			r, p := NewMockReservations(ctrl), NewMockPayments(ctrl)
			r.EXPECT().Reservations(gomock.Any(), testUser).Return([]model.InternalReservation{row}, nil)
			r.EXPECT().Hotel(gomock.Any(), row.HotelUID).Return(model.Hotel{HotelUID: row.HotelUID}, nil)
			p.EXPECT().Payment(gomock.Any(), row.PaymentUID).Return(model.Payment{}, cause)
			got, err := NewQueries(r, p, nil, 4).List(t.Context(), testUser)
			fallback := errors.Is(cause, model.ErrUpstreamUnavailable) || errors.Is(cause, model.ErrUpstreamTimeout)
			if fallback {
				if err != nil || len(got) != 1 || got[0].ReservationUID != row.ReservationUID ||
					got[0].Payment != (model.Payment{}) {
					t.Fatalf("fallback = %+v, error = %v", got, err)
				}
			} else if !errors.Is(err, cause) {
				t.Fatalf("error = %v, want %v", err, cause)
			}
		})
	}
}

func TestCompensationSurvivesRequestCancellation(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	r, p, l := NewMockReservations(ctrl), NewMockPayments(ctrl), NewMockLoyalty(ctrl)
	hotelID, paymentID, id := uuid.New(), uuid.New(), uuid.New()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r.EXPECT().Hotel(gomock.Any(), hotelID).Return(model.Hotel{Price: 100}, nil)
	l.EXPECT().Loyalty(gomock.Any(), testUser).Return(model.Loyalty{Discount: 5}, nil)
	p.EXPECT().CreatePayment(gomock.Any(), paymentCreateMatcher(95)).
		Return(model.InternalPayment{PaymentUID: paymentID}, nil)
	r.EXPECT().CreateReservation(gomock.Any(), testUser, gomock.Any()).
		Return(model.InternalReservation{ReservationUID: id}, nil)
	l.EXPECT().ChangeLoyalty(gomock.Any(), testUser, 1, id.String()+":create").DoAndReturn(
		func(context.Context, string, int, string) error { cancel(); return model.ErrUpstreamUnavailable })
	compensationErr := errors.New("rollback reservation failed")
	checkContext := func(ctx context.Context) {
		if ctx.Err() != nil {
			t.Errorf("rollback uses canceled request: %v", ctx.Err())
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("rollback lacks timeout")
		}
	}
	gomock.InOrder(
		r.EXPECT().CancelReservation(gomock.Any(), id, testUser).DoAndReturn(
			func(ctx context.Context, _ uuid.UUID, _ string) error { checkContext(ctx); return compensationErr }),
		p.EXPECT().CancelPayment(gomock.Any(), paymentID).DoAndReturn(
			func(ctx context.Context, _ uuid.UUID) error { checkContext(ctx); return nil }),
	)
	l.EXPECT().ChangeLoyalty(gomock.Any(), testUser, -1, gomock.Any()).Return(nil)
	service := newTestCommands(r, p, l)
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	_, err := service.Create(ctx, model.CreateBooking{
		User:     testUser,
		HotelUID: hotelID, StartDate: start, EndDate: start.AddDate(0, 0, 1),
	})
	if !errors.Is(err, compensationErr) || !errors.Is(err, model.ErrUpstreamUnavailable) {
		t.Fatal(err)
	}
}

func TestCancelReservationFailureKeepsPendingStep(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	r, p, l := NewMockReservations(ctrl), NewMockPayments(ctrl), NewMockLoyalty(ctrl)
	id, paymentID := uuid.New(), uuid.New()
	r.EXPECT().Reservation(gomock.Any(), id, testUser).Times(2).Return(model.InternalReservation{
		ReservationUID: id, PaymentUID: paymentID, Status: model.ReservationPaid,
	}, nil)
	gomock.InOrder(
		p.EXPECT().CancelPayment(gomock.Any(), paymentID).Return(nil),
		r.EXPECT().CancelReservation(gomock.Any(), id, testUser).Return(model.ErrUpstreamUnavailable),
	)
	service := newTestCommands(r, p, l)
	if err := service.Cancel(t.Context(), model.CancelBooking{
		ReservationUID: id, User: testUser,
	}); !errors.Is(err, model.ErrUpstreamUnavailable) {
		t.Fatal(err)
	}
	if err := service.Cancel(t.Context(), model.CancelBooking{
		ReservationUID: id, User: testUser,
	}); !errors.Is(err, model.ErrReservationUnavailable) {
		t.Fatal(err)
	}
	task, pending, _ := service.stream.PendingOperation(t.Context(), model.OperationKey(model.OperationCancellation, id))
	if !pending || !model.HasAction[model.CancelReservationOperation](task) ||
		model.HasAction[model.CancelPaymentOperation](task) ||
		!model.HasAction[model.DecreaseLoyaltyOperation](task) {
		t.Fatal(task, pending)
	}
}
