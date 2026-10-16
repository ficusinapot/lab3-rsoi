package bookings

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"uuid"

	"go.uber.org/mock/gomock"
	model "lab2/gateway/internal/models/entities"
)

func readRows(count int) []model.InternalReservation {
	rows := make([]model.InternalReservation, count)
	for index := range rows {
		rows[index] = model.InternalReservation{
			ReservationUID: uuid.New(), HotelUID: uuid.New(), PaymentUID: uuid.New(),
			Status: model.ReservationPaid,
		}
	}
	return rows
}

func TestListPreservesOrder(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		reservations := NewMockReservationReader(ctrl)
		payments := NewMockPaymentReader(ctrl)
		rows := readRows(4)
		completed := make(chan uuid.UUID, len(rows))
		release := make([]chan struct{}, len(rows))
		reservations.EXPECT().Reservations(gomock.Any(), "user").Return(rows, nil)
		for index, row := range rows {
			release[index] = make(chan struct{})
			reservations.EXPECT().Hotel(gomock.Any(), row.HotelUID).DoAndReturn(
				func(ctx context.Context, id uuid.UUID) (model.Hotel, error) {
					select {
					case <-release[index]:
						return model.Hotel{HotelUID: id, Name: id.String()}, nil
					case <-ctx.Done():
						return model.Hotel{}, ctx.Err()
					}
				},
			)
			payments.EXPECT().Payment(gomock.Any(), row.PaymentUID).DoAndReturn(
				func(context.Context, uuid.UUID) (model.Payment, error) {
					completed <- row.ReservationUID
					return model.Payment{Status: model.PaymentPaid, Price: index}, nil
				},
			)
		}
		var items []model.Reservation
		var err error
		go func() { items, err = NewQueries(reservations, payments, nil, 4).List(t.Context(), "user") }()
		synctest.Wait()
		for index := len(rows) - 1; index >= 0; index-- {
			close(release[index])
			synctest.Wait()
			if got := <-completed; got != rows[index].ReservationUID {
				t.Fatalf("completed reservation = %s, want %s", got, rows[index].ReservationUID)
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != len(rows) {
			t.Fatalf("reservation count = %d, want %d", len(items), len(rows))
		}
		for index, item := range items {
			if item.ReservationUID != rows[index].ReservationUID {
				t.Errorf("reservation[%d].UID = %s, want %s", index, item.ReservationUID, rows[index].ReservationUID)
			}
			if item.Hotel.HotelUID != rows[index].HotelUID {
				t.Errorf("reservation[%d].hotel UID = %s, want %s", index, item.Hotel.HotelUID, rows[index].HotelUID)
			}
			if item.Payment.Price != index {
				t.Errorf("reservation[%d].price = %d, want %d", index, item.Payment.Price, index)
			}
		}
	})
}

func TestListConcurrencyLimit(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{1, 4} {
		name := "one worker"
		if limit == 4 {
			name = "four workers"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				reservations := NewMockReservationReader(ctrl)
				payments := NewMockPaymentReader(ctrl)
				rows := readRows(20)
				uniqueHotels := len(rows)
				if limit == 4 {
					uniqueHotels = limit
					for index := range rows {
						rows[index].HotelUID = rows[index%uniqueHotels].HotelUID
					}
				}
				reservations.EXPECT().Reservations(gomock.Any(), "user").Return(rows, nil)
				release := make(chan struct{})
				var active, calls atomic.Int32
				var exceeded atomic.Bool
				reservations.EXPECT().Hotel(gomock.Any(), gomock.Any()).Times(uniqueHotels).DoAndReturn(
					func(ctx context.Context, id uuid.UUID) (model.Hotel, error) {
						running := active.Add(1)
						defer active.Add(-1)
						calls.Add(1)
						if int(running) > limit {
							exceeded.Store(true)
						}
						select {
						case <-release:
							return model.Hotel{HotelUID: id}, nil
						case <-ctx.Done():
							return model.Hotel{}, ctx.Err()
						}
					},
				)
				payments.EXPECT().Payment(gomock.Any(), gomock.Any()).Times(len(rows)).Return(model.Payment{}, nil)
				var items []model.Reservation
				var err error
				go func() { items, err = NewQueries(reservations, payments, nil, limit).List(t.Context(), "user") }()
				synctest.Wait()
				if got := active.Load(); int(got) != limit {
					t.Errorf("active requests before release = %d, want %d", got, limit)
				}
				if got := calls.Load(); int(got) != limit {
					t.Errorf("started requests before release = %d, want %d", got, limit)
				}
				close(release)
				synctest.Wait()
				if err != nil {
					t.Fatal(err)
				}
				if len(items) != len(rows) {
					t.Errorf("reservation count = %d, want %d", len(items), len(rows))
				}
				if got := calls.Load(); int(got) != uniqueHotels {
					t.Errorf("hotel requests = %d, want %d", got, uniqueHotels)
				}
				if exceeded.Load() {
					t.Errorf("active requests exceeded limit %d", limit)
				}
				if got := active.Load(); got != 0 {
					t.Errorf("active requests after return = %d, want 0", got)
				}
			})
		})
	}
}

func TestListCancellationBeforeRequests(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	reservations := NewMockReservationReader(ctrl)
	payments := NewMockPaymentReader(ctrl)
	reservations.EXPECT().Reservations(gomock.Any(), "user").Return(readRows(20), nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	items, err := NewQueries(reservations, payments, nil, 4).List(ctx, "user")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want cancellation", err)
	}
	if items != nil {
		t.Fatalf("reservations = %v, want nil", items)
	}
}

func TestListCancellationDuringRequests(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		reservations := NewMockReservationReader(ctrl)
		payments := NewMockPaymentReader(ctrl)
		reservations.EXPECT().Reservations(gomock.Any(), "user").Return(readRows(20), nil)
		var active atomic.Int32
		reservations.EXPECT().Hotel(gomock.Any(), gomock.Any()).Times(4).DoAndReturn(
			func(ctx context.Context, _ uuid.UUID) (model.Hotel, error) {
				active.Add(1)
				defer active.Add(-1)
				<-ctx.Done()
				return model.Hotel{}, ctx.Err()
			},
		)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var items []model.Reservation
		var err error
		go func() { items, err = NewQueries(reservations, payments, nil, 4).List(ctx, "user") }()
		synctest.Wait()
		if got := active.Load(); got != 4 {
			t.Errorf("active requests before cancellation = %d, want 4", got)
		}
		cancel()
		synctest.Wait()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want cancellation", err)
		}
		if items != nil {
			t.Errorf("reservations = %v, want nil", items)
		}
		if got := active.Load(); got != 0 {
			t.Errorf("active requests after cancellation = %d, want 0", got)
		}
	})
}

func TestListFailureCancelsOtherRequests(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		reservations := NewMockReservationReader(ctrl)
		payments := NewMockPaymentReader(ctrl)
		rows := readRows(20)
		cause := errors.New("hotel unavailable")
		release := make(chan struct{})
		var active atomic.Int32
		reservations.EXPECT().Reservations(gomock.Any(), "user").Return(rows, nil)
		reservations.EXPECT().Hotel(gomock.Any(), gomock.Any()).Times(4).DoAndReturn(
			func(ctx context.Context, id uuid.UUID) (model.Hotel, error) {
				active.Add(1)
				defer active.Add(-1)
				if id == rows[0].HotelUID {
					<-release
					return model.Hotel{}, cause
				}
				<-ctx.Done()
				return model.Hotel{}, ctx.Err()
			},
		)
		var items []model.Reservation
		var err error
		go func() { items, err = NewQueries(reservations, payments, nil, 4).List(t.Context(), "user") }()
		synctest.Wait()
		if got := active.Load(); got != 4 {
			t.Errorf("active requests before failure = %d, want 4", got)
		}
		close(release)
		synctest.Wait()
		if !errors.Is(err, cause) {
			t.Fatalf("error = %v, want cause %v", err, cause)
		}
		if items != nil {
			t.Errorf("reservations = %v, want nil", items)
		}
		if got := active.Load(); got != 0 {
			t.Errorf("active requests after failure = %d, want 0", got)
		}
	})
}

func TestListEmpty(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	reservations := NewMockReservationReader(ctrl)
	reservations.EXPECT().Reservations(gomock.Any(), "user").Return(nil, nil)
	items, err := NewQueries(reservations, NewMockPaymentReader(ctrl), nil, 4).List(t.Context(), "user")
	if err != nil {
		t.Fatal(err)
	}
	if items == nil || len(items) != 0 {
		t.Fatalf("reservations = %v, want nonnil empty slice", items)
	}
}

func TestPagination(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		query         model.PageQuery
		upstreamQuery model.PageQuery
		wantPage      int
		wantErr       error
	}{
		{name: "omitted", wantPage: 1},
		{
			name: "zero page", query: model.PageQuery{Page: new(0), Size: new(10)},
			upstreamQuery: model.PageQuery{Page: new(1), Size: new(10)},
		},
		{
			name: "explicit", query: model.PageQuery{Page: new(3), Size: new(5)},
			upstreamQuery: model.PageQuery{Page: new(3), Size: new(5)}, wantPage: 3,
		},
		{name: "negative page", query: model.PageQuery{Page: new(-1)}, wantErr: model.ErrInvalidInput},
		{name: "zero size", query: model.PageQuery{Size: new(0)}, wantErr: model.ErrInvalidInput},
		{name: "oversize", query: model.PageQuery{Size: new(101)}, wantErr: model.ErrInvalidInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			reservations := NewMockReservationReader(ctrl)
			if tc.wantErr == nil {
				page := tc.wantPage
				if tc.query.Page != nil && *tc.query.Page == 0 {
					page = 1
				}
				reservations.EXPECT().Hotels(gomock.Any(), tc.upstreamQuery).
					Return(model.Page{Page: page, Items: []model.Hotel{}}, nil)
			}
			got, err := NewQueries(reservations, nil, nil, 4).Hotels(t.Context(), tc.query)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if got.Page != tc.wantPage {
				t.Errorf("page = %d, want %d", got.Page, tc.wantPage)
			}
			if tc.wantErr == nil && !slices.Equal(got.Items, []model.Hotel{}) {
				t.Errorf("hotels = %v, want empty", got.Items)
			}
		})
	}
}
