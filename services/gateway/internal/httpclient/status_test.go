package httpclient

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"lab2/gateway/internal/models/entities"

	"github.com/samber/oops"
)

func TestCheckStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		status   int
		expected int
		wantErr  error
	}{
		{name: "ok", status: 200, expected: 200},
		{name: "created", status: 201, expected: 201},
		{name: "no content", status: 204, expected: 204},
		{name: "bad request", status: 400, expected: 200, wantErr: entities.ErrInvalidInput},
		{name: "unprocessable entity", status: 422, expected: 201, wantErr: entities.ErrInvalidInput},
		{name: "not found", status: 404, expected: 200, wantErr: entities.ErrNotFound},
		{name: "server error", status: 500, expected: 204, wantErr: entities.ErrUpstreamUnavailable},
		{name: "redirect", status: 302, expected: 200, wantErr: entities.ErrUpstreamUnavailable},
		{name: "unexpected success", status: 200, expected: 201, wantErr: entities.ErrUpstreamUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := CheckStatus(&http.Response{StatusCode: tc.status}, tc.expected)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CheckStatus() error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				failure, ok := oops.AsOops(err)
				if !ok || failure.Context()["upstream_status"] != tc.status {
					t.Fatalf("CheckStatus() error lacks upstream_status %d: %v", tc.status, err)
				}
			}
		})
	}
}

func TestCheckStatusMissingResponse(t *testing.T) {
	t.Parallel()
	err := CheckStatus(nil, http.StatusOK)
	if !errors.Is(err, entities.ErrUpstreamUnavailable) {
		t.Fatalf("CheckStatus(nil) error = %v, want upstream unavailable", err)
	}
	if !strings.Contains(err.Error(), "missing upstream response") {
		t.Fatalf("CheckStatus(nil) error lacks missing response diagnostic: %v", err)
	}
}
