//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"lab2/gateway/internal/models/entities"
)

const (
	faultDropAfter          = "drop_after"
	faultHoldAfter          = "hold_after"
	faultHoldBefore         = "hold_before"
	faultUnavailable        = "unavailable"
	loyaltyReservationsPath = "/api/v1/loyalty/reservations"
)

type capturedRequest struct {
	Key  string
	Body []byte
}

type faultRule struct {
	method, path, stage string
	seen                chan capturedRequest
	outcome             chan int
	release             chan struct{}
	once                sync.Once
}

func (r *faultRule) unblock() { r.once.Do(func() { close(r.release) }) }

type faultProxy struct {
	target       string
	server       *httptest.Server
	mu           sync.Mutex
	rule         *faultRule
	paymentPosts atomic.Int32
	root         *testing.T
}

func newFaultProxy(t *testing.T, target string) *faultProxy {
	t.Helper()
	p := &faultProxy{target: target, root: t}
	p.server = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.server.Close)
	return p
}

func (p *faultProxy) arm(t *testing.T, method, path, stage string) *faultRule {
	t.Helper()
	rule := &faultRule{
		method: method, path: path, stage: stage,
		seen: make(chan capturedRequest, 1), outcome: make(chan int, 1), release: make(chan struct{}),
	}
	p.mu.Lock()
	p.rule = rule
	p.mu.Unlock()
	t.Cleanup(rule.unblock)
	return rule
}

func (p *faultProxy) serve(w http.ResponseWriter, incoming *http.Request) {
	data, err := io.ReadAll(incoming.Body)
	if err != nil {
		p.root.Error(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if incoming.Method == http.MethodPost && incoming.URL.Path == "/api/v1/payments" {
		p.paymentPosts.Add(1)
	}
	p.mu.Lock()
	rule := p.rule
	if rule != nil && (rule.method != incoming.Method || rule.path != incoming.URL.Path) {
		rule = nil
	}
	if rule != nil {
		select {
		case <-rule.release:
			p.rule = nil
			rule = nil
		default:
		}
	}
	if rule != nil && rule.stage != faultUnavailable {
		p.rule = nil
	}
	p.mu.Unlock()
	captured := capturedRequest{Key: incoming.Header.Get("Idempotency-Key"), Body: data}
	if rule != nil && rule.stage == faultUnavailable {
		select {
		case rule.seen <- captured:
		default:
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if rule != nil && rule.stage == faultHoldBefore {
		rule.seen <- captured
		<-rule.release
	}
	// Forwarding may outlive the caller to model a request already accepted downstream.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(incoming.Context()), 5*time.Second)
	defer cancel()
	//nolint:gosec // The proxy forwards only to its fixture container address.
	outgoing, err := http.NewRequestWithContext(
		ctx, incoming.Method, p.target+incoming.URL.RequestURI(), bytes.NewReader(data),
	)
	if err != nil {
		p.root.Error(err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	outgoing.Header = incoming.Header.Clone()
	//nolint:gosec // The target is a fixture container; the request cannot select its host.
	response, err := http.DefaultClient.Do(outgoing)
	if err != nil {
		p.root.Error(err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			p.root.Error(err)
		}
	}()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		p.root.Error(err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	if rule != nil {
		if rule.stage == faultHoldBefore {
			rule.outcome <- response.StatusCode
		}
		if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated {
			if rule.stage == faultDropAfter || rule.stage == faultHoldAfter {
				rule.seen <- captured
				if rule.stage == faultHoldAfter {
					<-rule.release
				}
			}
		}
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			p.root.Error(err)
			return
		}
		if err := connection.Close(); err != nil {
			p.root.Error(err)
		}
		return
	}
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	w.WriteHeader(response.StatusCode)
	if _, err := w.Write(body); err != nil && incoming.Context().Err() == nil {
		p.root.Error(err)
	}
}

func awaitCapture(t *testing.T, rule *faultRule) capturedRequest {
	t.Helper()
	select {
	case captured := <-rule.seen:
		return captured
	case <-time.After(10 * time.Second):
		t.Fatal("fault request was not captured")
		return capturedRequest{}
	}
}

type nativeGateway struct {
	root                    *testing.T
	binary, config, address string
	command                 *exec.Cmd
	done                    chan error
	log                     *os.File
}

func newNativeGateway(t *testing.T, s *stack, proxies map[string]*faultProxy) *nativeGateway {
	t.Helper()
	root := os.Getenv("GATEWAY_PROJECT_ROOT")
	dir := t.TempDir()
	binary := filepath.Join(dir, "gateway")
	//nolint:gosec // The build directory is the required, trusted test project root.
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./cmd/gateway")
	build.Dir = filepath.Join(root, "services/gateway")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build gateway: %v\n%s", err, output)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Reads the fixture configuration from the trusted test project root.
	data, err := os.ReadFile(filepath.Join(root, "services/gateway/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := string(data)
	replacements := map[string]string{
		`address: ":8080"`:         `address: "` + address + `"`,
		"redis://localhost:6379/0": s.redisURL,
		"{lab3:operations}:":       "{recovery:" + uuid.New().String() + "}:",
		"lease_duration: 30s":      "lease_duration: 12s",
		"http://localhost:8060":    proxies[paymentServiceName].server.URL,
		"http://localhost:8070":    proxies[reservationServiceName].server.URL,
		"http://localhost:8050":    proxies[loyaltyServiceName].server.URL,
	}
	for from, to := range replacements {
		if !strings.Contains(cfg, from) {
			t.Fatalf("config entry missing: %s", from)
		}
		cfg = strings.ReplaceAll(cfg, from, to)
	}
	config := filepath.Join(dir, "gateway.yaml")
	//nolint:gosec // Writes a fixed filename inside t.TempDir.
	if err := os.WriteFile(config, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Creates a fixed log filename inside t.TempDir.
	log, err := os.Create(filepath.Join(dir, "gateway.log"))
	if err != nil {
		t.Fatal(err)
	}
	n := &nativeGateway{root: t, binary: binary, config: config, address: "http://" + address, log: log}
	t.Cleanup(func() {
		if n.command != nil {
			n.kill(t)
		}
		if err := n.log.Close(); err != nil {
			t.Error(err)
		}
		if t.Failed() {
			data, err := os.ReadFile(log.Name())
			if err != nil {
				t.Error(err)
			} else {
				t.Logf("recovery gateway logs:\n%s", data)
			}
		}
	})
	n.start(t)
	return n
}

func (n *nativeGateway) start(t *testing.T) {
	t.Helper()
	//nolint:gosec // Runs the Gateway binary built by this test inside t.TempDir.
	command := exec.CommandContext(n.root.Context(), n.binary, "--config", n.config)
	command.Stdout, command.Stderr = n.log, n.log
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	n.command, n.done = command, done
	go func() { done <- command.Wait() }()
	await(t, func() bool {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.address+"/manage/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		return response.StatusCode == http.StatusOK
	})
}

func (n *nativeGateway) kill(t *testing.T) {
	t.Helper()
	if err := n.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatal(err)
	}
	select {
	case <-n.done:
	case <-time.After(5 * time.Second):
		t.Fatal("gateway did not exit")
	}
	n.command = nil
}

func (n *nativeGateway) completed(t *testing.T, key string) bool {
	prefix, rawID, ok := strings.Cut(key, ":")
	if ok {
		kind := entities.OperationCompensation
		if prefix == "cancel" {
			kind = entities.OperationCancellation
		}
		key = entities.OperationKey(kind, uuid.MustParse(rawID)).String()
	}

	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	//nolint:gosec // Runs a fixed CLI command of the Gateway binary built by this test.
	output, err := exec.CommandContext(ctx, n.binary, "inspect-operation", "--config", n.config, "--key", key).Output()
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(output, &info); err != nil {
		t.Fatal(err)
	}
	return info.State == "completed"
}

func await(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(25 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("asynchronous recovery timed out")
		case <-ticker.C:
		}
	}
}
