package metrics

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	am "github.com/autometrics-dev/autometrics-go/pkg/autometrics"
	"github.com/autometrics-dev/autometrics-go/prometheus/autometrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/samber/oops"
)

type Config struct {
	Path           string    `mapstructure:"path"`
	Service        string    `mapstructure:"service"`
	Version        string    `mapstructure:"version"`
	LatencyBuckets []float64 `mapstructure:"latency_buckets"`
}

func (c Config) Validate() error {
	invalidIdentity := c.Service == "" || c.Version == ""
	invalidEndpoint := !strings.HasPrefix(c.Path, "/") || len(c.LatencyBuckets) == 0
	if invalidIdentity || invalidEndpoint {
		return oops.Errorf("metrics path, service, version and latency_buckets are required")
	}
	previous := 0.0
	for _, bucket := range c.LatencyBuckets {
		invalidBucket := bucket <= previous || math.IsNaN(bucket) || math.IsInf(bucket, 0)
		if invalidBucket {
			return oops.Errorf("latency buckets must be positive and strictly increasing")
		}
		previous = bucket
	}
	return nil
}

type Metrics struct {
	namespace string
	registry  *prometheus.Registry
	requests  *prometheus.CounterVec
	duration  *prometheus.HistogramVec
}

func New(c Config) (*Metrics, error) {
	if err := c.Validate(); err != nil {
		return nil, oops.Wrapf(err, "validate metrics configuration")
	}
	m := &Metrics{
		namespace: metricNameComponent(c.Service),
		registry:  prometheus.NewRegistry(),
	}
	m.requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: m.namespace,
		Subsystem: "rest_api",
		Name:      "http_requests_total",
		Help:      "Total number of HTTP requests.",
	}, []string{"method", "route", "status"})
	m.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: m.namespace,
		Subsystem: "rest_api",
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request duration in seconds.",
		Buckets:   c.LatencyBuckets,
	}, []string{"method", "route", "status"})
	for _, collector := range []prometheus.Collector{
		m.requests, m.duration,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	} {
		if err := m.registry.Register(collector); err != nil {
			return nil, oops.Wrapf(err, "register metrics collector")
		}
	}
	return m, nil
}

// Autometrics uses process-global state. Start it once in the composition root.
func (m *Metrics) StartAutometrics(c Config) (context.CancelCauseFunc, error) {
	cancel, err := autometrics.Init(
		autometrics.WithRegistry(m.registry),
		autometrics.WithService(c.Service),
		autometrics.WithVersion(c.Version),
		autometrics.WithHistogramBuckets(c.LatencyBuckets),
	)
	if err != nil {
		return nil, oops.Wrapf(err, "initialize autometrics")
	}
	// Keep YAML authoritative over Autometrics' implicit environment defaults.
	am.SetService(c.Service)
	return cancel, nil
}

func (m *Metrics) RegisterDatabase(db *sql.DB) error {
	stats := []struct {
		name  string
		help  string
		value func() float64
	}{
		{
			name: "connections_open", help: "Number of established database connections.",
			value: func() float64 { return float64(db.Stats().OpenConnections) },
		},
		{
			name: "connections_in_use", help: "Number of database connections currently in use.",
			value: func() float64 { return float64(db.Stats().InUse) },
		},
		{
			name: "connections_idle", help: "Number of idle database connections.",
			value: func() float64 { return float64(db.Stats().Idle) },
		},
	}
	for _, stat := range stats {
		collector := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: m.namespace,
			Subsystem: "db",
			Name:      stat.name,
			Help:      stat.help,
		}, stat.value)
		if err := m.registry.Register(collector); err != nil {
			return oops.Wrapf(err, "register database metric collector")
		}
	}
	return nil
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) ObserveHTTPRequest(
	method string,
	route string,
	statusCode int,
	duration time.Duration,
) {
	status := strconv.Itoa(statusCode)
	m.requests.WithLabelValues(method, route, status).Inc()
	m.duration.WithLabelValues(method, route, status).Observe(duration.Seconds())
}
