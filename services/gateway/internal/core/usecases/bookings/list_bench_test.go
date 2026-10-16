package bookings

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/samber/oops"
	model "lab2/gateway/internal/models/entities"
)

func BenchmarkList(b *testing.B) {
	for _, count := range []int{1, 10, 50} {
		for _, limit := range []int{1, 4} {
			b.Run(fmt.Sprintf("rows=%d/workers=%d", count, limit), func(b *testing.B) {
				reservations := &benchmarkReservations{rows: readRows(count)}
				service := NewQueries(reservations, benchmarkPayments{}, nil, limit)
				b.ReportAllocs()
				for b.Loop() {
					if _, err := service.List(b.Context(), "user"); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(reservations.peak.Load()), "peak_calls")
			})
		}
	}
}

type benchmarkReservations struct {
	rows   []model.InternalReservation
	active atomic.Int32
	peak   atomic.Int32
}

func (r *benchmarkReservations) Reservations(context.Context, string) ([]model.InternalReservation, error) {
	return r.rows, nil
}

func (r *benchmarkReservations) Hotel(ctx context.Context, id uuid.UUID) (model.Hotel, error) {
	running := r.active.Add(1)
	defer r.active.Add(-1)
	for previous := r.peak.Load(); running > previous; previous = r.peak.Load() {
		if r.peak.CompareAndSwap(previous, running) {
			break
		}
	}
	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return model.Hotel{}, oops.Wrapf(ctx.Err(), "benchmark hotel lookup canceled")
	case <-timer.C:
		return model.Hotel{HotelUID: id}, nil
	}
}

func (*benchmarkReservations) Hotels(context.Context, model.PageQuery) (model.Page, error) {
	panic("unexpected Hotels call in BenchmarkList")
}

func (*benchmarkReservations) Reservation(context.Context, uuid.UUID, string) (model.InternalReservation, error) {
	panic("unexpected Reservation call in BenchmarkList")
}

type benchmarkPayments struct{}

func (benchmarkPayments) Payment(context.Context, uuid.UUID) (model.Payment, error) {
	return model.Payment{}, nil
}
