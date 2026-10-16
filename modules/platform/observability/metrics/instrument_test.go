package metrics

import (
	"sync"
	"testing"
)

func TestConcurrentInstrumentation(t *testing.T) {
	t.Parallel()
	cfg := Config{Path: metricsPath, Service: serviceName, Version: serviceVersion, LatencyBuckets: []float64{0.01, 0.1}}
	telemetry, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	stop, err := telemetry.StartAutometrics(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer stop(nil)
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			var operationErr error
			defer Instrument(PreInstrument(t.Context(), TestConcurrentInstrumentation), &operationErr)
		})
	}
	wg.Wait()
	families, err := telemetry.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != "function_calls_total" {
			continue
		}
		var count float64
		for _, metric := range family.Metric {
			count += metric.GetCounter().GetValue()
		}
		if count != 50 {
			t.Fatalf("recorded calls %v, want 50", count)
		}
		return
	}
	t.Fatal("operation counter absent")
}
