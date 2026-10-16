package metrics

import (
	"context"
	"sync"

	am "github.com/autometrics-dev/autometrics-go/pkg/autometrics"
	"github.com/autometrics-dev/autometrics-go/prometheus/autometrics"
)

var instrumentMu sync.Mutex

// PreInstrument serializes Autometrics v1.1.0's shared, unsynchronized PRNG.
// The measured operation runs outside the lock.
func PreInstrument(ctx context.Context, operation any) context.Context {
	instrumentMu.Lock()
	defer instrumentMu.Unlock()
	// The library infers wrapper names instead of operation names. Disable its
	// optional concurrency gauge so setup/recording use the same operation labels.
	ctx = autometrics.PreInstrument(am.SetTrackConcurrentCalls(ctx, false))
	if ctx == nil {
		return nil
	}
	return am.SetCallInfo(ctx, am.ReflectFunctionModuleName(operation))
}

// Instrument also guards the library's shared caller map during recording.
func Instrument(ctx context.Context, err *error) {
	instrumentMu.Lock()
	defer instrumentMu.Unlock()
	autometrics.Instrument(ctx, err)
}
