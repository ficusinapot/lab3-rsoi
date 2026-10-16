package metrics

import (
	"net/http"
	"testing"
	"time"
)

const (
	metricsPath    = "/metrics"
	serviceName    = "reservation"
	serviceVersion = "test"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		cfg   Config
		valid bool
	}{
		{name: "valid", cfg: Config{
			Path: metricsPath, Service: serviceName, Version: serviceVersion, LatencyBuckets: []float64{0.01, 0.1},
		}, valid: true},
		{name: "empty", cfg: Config{}},
		{name: "unordered buckets", cfg: Config{
			Path: metricsPath, Service: serviceName, Version: serviceVersion, LatencyBuckets: []float64{0.1, 0.01},
		}},
		{name: "zero bucket", cfg: Config{
			Path: metricsPath, Service: serviceName, Version: serviceVersion, LatencyBuckets: []float64{0},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.cfg.Validate(); (err == nil) != tc.valid {
				t.Fatalf("validation error=%v", err)
			}
		})
	}
}

func TestObserveHTTPRequest(t *testing.T) {
	t.Parallel()
	m, err := New(Config{
		Path:           metricsPath,
		Service:        serviceName,
		Version:        serviceVersion,
		LatencyBuckets: []float64{0.01, 0.1},
	})
	if err != nil {
		t.Fatal(err)
	}
	const route = "/bookings/{id}"
	for range 2 {
		m.ObserveHTTPRequest(
			http.MethodGet,
			route,
			http.StatusNoContent,
			time.Millisecond,
		)
	}
	families, err := m.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, family := range families {
		if family.GetName() != "reservation_rest_api_http_requests_total" {
			continue
		}
		found = true
		if len(family.Metric) != 1 {
			t.Fatalf("route series=%d", len(family.Metric))
		}
		metric := family.Metric[0]
		if metric.GetCounter().GetValue() != 2 {
			t.Fatalf("request count=%v", metric.GetCounter().GetValue())
		}
		for _, label := range metric.Label {
			if label.GetName() == "route" && label.GetValue() != route {
				t.Fatalf("route label=%q", label.GetValue())
			}
		}
	}
	if !found {
		t.Fatal("request counter absent")
	}
}

func TestRuntimeCollectors(t *testing.T) {
	t.Parallel()
	telemetry, err := New(Config{
		Path: metricsPath, Service: serviceName, Version: serviceVersion, LatencyBuckets: []float64{0.01, 0.1},
	})
	if err != nil {
		t.Fatal(err)
	}
	families, err := telemetry.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool, len(families))
	for _, family := range families {
		names[family.GetName()] = true
	}
	for _, name := range []string{"go_goroutines", "process_cpu_seconds_total"} {
		if !names[name] {
			t.Fatalf("missing runtime collector %s", name)
		}
	}
}
