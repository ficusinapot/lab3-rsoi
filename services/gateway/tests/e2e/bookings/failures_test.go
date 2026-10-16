//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
	"uuid"

	"github.com/testcontainers/testcontainers-go"
)

func partialFailures(t *testing.T, s *stack) {
	t.Helper()
	for _, tc := range []struct {
		name            string
		service         string
		create          bool
		wantReservation string
	}{
		{name: "reservation unavailable during cancellation", service: reservationServiceName, wantReservation: paidStatus},
		{name: "payment unavailable during cancellation", service: paymentServiceName, wantReservation: paidStatus},
		{name: "payment unavailable during creation", service: paymentServiceName, create: true, wantReservation: paidStatus},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := "failure " + uuid.New().String()
			input := createRequest{HotelUID: hotelUID, StartDate: bookingStartDate, EndDate: bookingEndDate}
			var created createdResult
			request(t, http.MethodPost, s.gateway+"/api/v1/reservations", user, input, &created, http.StatusOK)
			before := reservationCount(t, s, user)
			restore := stopService(t, s, tc.service)
			path := s.gateway + "/api/v1/reservations/" + created.ReservationUID.String()
			if tc.create {
				request(t, http.MethodPost, s.gateway+"/api/v1/reservations", user,
					input, nil, http.StatusBadGateway, http.StatusGatewayTimeout)
			} else {
				request(t, http.MethodDelete, path, user, nil, nil,
					http.StatusInternalServerError, http.StatusBadGateway, http.StatusGatewayTimeout)
			}
			if tc.service == paymentServiceName {
				var fallback map[string]json.RawMessage
				request(t, http.MethodGet, path, user, nil, &fallback, http.StatusOK)
				if string(fallback["payment"]) != "{}" {
					t.Fatalf("payment fallback = %s", fallback["payment"])
				}
				var me userResult
				request(t, http.MethodGet, s.gateway+"/api/v1/me", user, nil, &me, http.StatusOK)
				if len(me.Reservations) != 1 || me.Reservations[0].Status != paidStatus {
					t.Fatal(me)
				}
			}

			restore()
			var item reservationResult
			request(t, http.MethodGet, path, user, nil, &item, http.StatusOK)
			if item.Status != tc.wantReservation || item.Payment.Status != paidStatus {
				t.Fatalf("after failure: got %+v, want reservation %s and payment PAID", item, tc.wantReservation)
			}
			if after := reservationCount(t, s, user); after != before {
				t.Fatalf("reservation count: got %d, want %d", after, before)
			}
			loyalty(t, s, user, 1, 5, "BRONZE")
		})
	}
	t.Run("loyalty cancellation eventually applies once", func(t *testing.T) {
		user := "retry " + uuid.New().String()
		input := createRequest{HotelUID: hotelUID, StartDate: bookingStartDate, EndDate: bookingEndDate}
		var created createdResult
		request(t, http.MethodPost, s.gateway+"/api/v1/reservations", user, input, &created, http.StatusOK)
		restore := stopService(t, s, loyaltyServiceName)
		var me map[string]json.RawMessage
		request(t, http.MethodGet, s.gateway+"/api/v1/me", user, nil, &me, http.StatusOK)
		if string(me["loyalty"]) != "{}" {
			t.Fatalf("loyalty fallback = %s", me["loyalty"])
		}
		var failure struct {
			Message string `json:"message"`
		}
		request(t, http.MethodGet, s.gateway+"/api/v1/loyalty", user, nil, &failure, http.StatusServiceUnavailable)
		if failure.Message != "Loyalty Service unavailable" {
			t.Fatal(failure)
		}
		request(t, http.MethodPost, s.gateway+"/api/v1/reservations", user, input, &failure, http.StatusServiceUnavailable)
		if failure.Message != "Loyalty Service unavailable" {
			t.Fatal(failure)
		}
		path := s.gateway + "/api/v1/reservations/" + created.ReservationUID.String()
		for range 3 {
			request(t, http.MethodDelete, path, user, nil, nil, http.StatusNoContent)
		}
		var item reservationResult
		request(t, http.MethodGet, path, user, nil, &item, http.StatusOK)
		if item.Status != canceledStatus || item.Payment.Status != canceledStatus {
			t.Fatal(item)
		}
		restore()
		deadline := time.Now().Add(30 * time.Second)
		for {
			var current loyaltyResult
			request(t, http.MethodGet, s.gateway+"/api/v1/loyalty", user, nil, &current,
				http.StatusOK, http.StatusServiceUnavailable)
			if current.Status != "" && current.ReservationCount == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("queued decrement did not converge", current)
			}
			time.Sleep(100 * time.Millisecond)
		}
		request(t, http.MethodDelete, path, user, nil, nil, http.StatusNoContent)
		loyalty(t, s, user, 0, 5, "BRONZE")
	})
}

func reservationCount(t *testing.T, s *stack, user string) int {
	t.Helper()
	var count int
	query := s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM reservation WHERE username=$1", user)
	if err := query.Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func stopService(t *testing.T, s *stack, name string) func() {
	t.Helper()
	client, err := testcontainers.NewDockerClientWithOpts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	container := s.containers[name]
	// Pause models an unavailable service without changing Docker network endpoints.
	if err := client.ContainerPause(t.Context(), container.GetContainerID()); err != nil {
		t.Fatal(err)
	}
	restarted := false
	restore := func() {
		if restarted {
			return
		}
		restarted = true
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := client.ContainerUnpause(ctx, container.GetContainerID()); err != nil {
			t.Fatal(err)
		}

		client := &http.Client{Timeout: time.Second}
		defer client.CloseIdleConnections()
		for ctx.Err() == nil {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.addresses[name]+"/manage/health", http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			res, err := client.Do(req)
			if err == nil {
				if err := res.Body.Close(); err != nil {
					t.Fatal(err)
				}
				if res.StatusCode == http.StatusOK {
					return
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal(ctx.Err())
	}
	t.Cleanup(restore)
	return restore
}
