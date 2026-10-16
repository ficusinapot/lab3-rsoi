//go:build e2e

package support

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type HTTPClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPClient(baseURL string) HTTPClient {
	return HTTPClient{baseURL: strings.TrimRight(baseURL, "/"), client: &http.Client{Timeout: 10 * time.Second}}
}

func (c HTTPClient) Request(t *testing.T, method, path, user string, body []byte, wantStatus int) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, c.baseURL+path, strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	if user != "" {
		req.Header.Set("X-User-Name", user)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: status %d want %d: %s", method, path, resp.StatusCode, wantStatus, data)
	}
	return data
}
