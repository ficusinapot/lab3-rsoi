package bookings

import (
	"io"
	"log/slog"
	"time"

	"go.uber.org/mock/gomock"

	"lab2/gateway/internal/core/services/opstream"
	"lab2/gateway/internal/core/usecases/operations"
	"lab2/gateway/internal/models/loyaltyifc"
	"lab2/gateway/internal/models/paymentifc"
	"lab2/gateway/internal/models/reservationifc"
)

func newTestCommands(r reservationifc.Bookings, p paymentifc.Payments, l loyaltyifc.Client) *testCommands {
	if mock, ok := l.(*MockLoyalty); ok {
		mock.EXPECT().ResolveCreation(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(false, nil)
	}
	cfg := opstream.Config{
		Capacity: 256, Concurrency: 4, RetryInterval: 10 * time.Second,
		AttemptTimeout: 5 * time.Second, RollbackTimeout: 5 * time.Second,
		StorageTimeout:      5 * time.Second,
		CompensationWorkers: 1,
		PollInterval:        10 * time.Second,
		ShutdownTimeout:     20 * time.Second,
	}
	processor := operations.New(operations.NewExecutor(operations.NewActions(r, p, l)))
	stream := opstream.NewMemory(cfg, processor, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &testCommands{
		Commands: NewCommands(r, l, operations.NewGroups(stream, cfg.RollbackTimeout, r, p, l)),
		stream:   stream,
	}
}

type testCommands struct {
	*Commands
	stream *opstream.Stream
}
