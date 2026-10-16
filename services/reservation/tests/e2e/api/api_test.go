//go:build e2e

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"lab2/reservation/tests/e2e/fixtures"
	"lab2/reservation/tests/e2e/infrastructure"
	"lab2/reservation/tests/e2e/support"
)

type reservationResponse struct {
	ReservationUID string `json:"reservationUid"`
	Status         string `json:"status"`
	StartDate      string `json:"startDate"`
	EndDate        string `json:"endDate"`
}

//nolint:paralleltest // The test builds and starts a dedicated container stack.
func TestReservationAPI(t *testing.T) {
	stack := infrastructure.NewReservationStack(t)
	fixtures.SeedHotel(t, stack.DatabaseURL)
	api := support.NewHTTPClient(stack.BaseURL)

	//nolint:paralleltest // These scenarios share the parent container stack.
	t.Run("service endpoints", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			path   string
			status int
		}{
			{name: "health", path: "/manage/health", status: http.StatusOK},
			{name: "hotels", path: "/api/v1/hotels", status: http.StatusOK},
			{name: "invalid page", path: "/api/v1/hotels?page=0", status: http.StatusBadRequest},
			{name: "documentation", path: "/api/v1/docs", status: http.StatusOK},
			{name: "OpenAPI", path: "/api/v1/openapi.json", status: http.StatusOK},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				api.Request(t, http.MethodGet, tc.path, "", nil, tc.status)
			})
		}
	})

	//nolint:paralleltest // This scenario creates and cancels the same reservation sequentially.
	t.Run("booking lifecycle and ownership", func(t *testing.T) {
		body, err := json.Marshal(struct {
			HotelUID   string `json:"hotelUid"`
			PaymentUID string `json:"paymentUid"`
			StartDate  string `json:"startDate"`
			EndDate    string `json:"endDate"`
		}{
			HotelUID:   fixtures.TestHotelUID,
			PaymentUID: "00000000-0000-0000-0000-000000000001",
			StartDate:  "2030-01-01",
			EndDate:    "2030-01-03",
		})
		if err != nil {
			t.Fatal(err)
		}
		created := api.Request(t, http.MethodPost, "/api/v1/reservations", "e2e-user", body, http.StatusCreated)
		var reservation reservationResponse
		if err := json.Unmarshal(created, &reservation); err != nil {
			t.Fatal(err)
		}
		if reservation.ReservationUID == "" || reservation.Status != "PAID" {
			t.Fatalf("created reservation = %+v, want a UID and PAID status", reservation)
		}
		if reservation.StartDate != "2030-01-01" || reservation.EndDate != "2030-01-03" {
			t.Fatalf("reservation dates = %s to %s, want 2030-01-01 to 2030-01-03",
				reservation.StartDate, reservation.EndDate)
		}
		path := "/api/v1/reservations/" + reservation.ReservationUID
		api.Request(t, http.MethodGet, path, "other-user", nil, http.StatusNotFound)
		api.Request(t, http.MethodDelete, path, "other-user", nil, http.StatusNotFound)
		owned := api.Request(t, http.MethodGet, path, "e2e-user", nil, http.StatusOK)
		var current reservationResponse
		if err := json.Unmarshal(owned, &current); err != nil {
			t.Fatal(err)
		}
		if current != reservation {
			t.Fatalf("retrieved reservation = %+v, want %+v", current, reservation)
		}
		deleted := api.Request(t, http.MethodDelete, path, "e2e-user", nil, http.StatusNoContent)
		if len(deleted) != 0 {
			t.Fatalf("DELETE body = %q, want empty", deleted)
		}
		canceled := api.Request(t, http.MethodGet, path, "e2e-user", nil, http.StatusOK)
		if err := json.Unmarshal(canceled, &current); err != nil {
			t.Fatal(err)
		}
		reservation.Status = "CANCELED"
		if current != reservation {
			t.Fatalf("canceled reservation = %+v, want %+v", current, reservation)
		}
	})
}
