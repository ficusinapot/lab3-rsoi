//go:build integration

package support

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type HTTPClient struct {
	handler http.Handler
}

type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func NewHTTPClient(handler http.Handler) HTTPClient {
	return HTTPClient{handler: handler}
}

func (c HTTPClient) Request(t *testing.T, method, path, user string, body string, wantStatus int) Response {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if user != "" {
		req.Header.Set("X-User-Name", user)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	c.handler.ServeHTTP(response, req)
	result := response.Result()
	defer func() {
		if err := result.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	data, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Code != wantStatus {
		t.Fatalf("%s %s: status %d want %d: %s", method, path, response.Code, wantStatus, data)
	}
	return Response{StatusCode: response.Code, Header: response.Header(), Body: data}
}
