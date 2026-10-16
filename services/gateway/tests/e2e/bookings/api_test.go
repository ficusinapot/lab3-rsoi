//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"testing"
	"time"
)

const (
	bookingEndDate         = "2026-01-04"
	bookingStartDate       = "2026-01-01"
	reservationServiceName = "reservation"
	paymentServiceName     = "payment"
	hotelUID               = "049161bb-badd-4fa8-9d90-87c9a82b0668"
	paidStatus             = "PAID"
	canceledStatus         = "CANCELED"
)

func request(t *testing.T, method, address, user string, input, output any, want ...int) []byte {
	t.Helper()
	var data bytes.Buffer
	if input != nil {
		if err := json.NewEncoder(&data).Encode(input); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, address, &data)
	if err != nil {
		t.Fatal(err)
	}
	if user != "" {
		req.Header.Set("X-User-Name", user)
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(want, res.StatusCode) {
		t.Fatalf("%s %s: got %d want %v body %s", method, address, res.StatusCode, want, body)
	}
	if output != nil {
		if err := json.Unmarshal(body, output); err != nil {
			t.Fatal(err)
		}
	}
	if res.StatusCode == http.StatusNoContent && len(body) != 0 {
		t.Fatalf("204 response has a body: %s", body)
	}
	return body
}

func loyalty(t *testing.T, s *stack, user string, count, discount int, status string) {
	t.Helper()
	var out loyaltyResult
	request(t, http.MethodGet, s.gateway+"/api/v1/loyalty", user, nil, &out, http.StatusOK)
	if out.ReservationCount != count || out.Discount != discount || out.Status != status {
		t.Fatalf("unexpected loyalty %+v", out)
	}
}

//nolint:paralleltest // Subtests share containers and failure scenarios stop services.
func TestBookingLifecycle(t *testing.T) {
	t.Parallel()
	s := newStack(t)
	t.Cleanup(http.DefaultClient.CloseIdleConnections)
	t.Run("service endpoints", func(t *testing.T) {
		for _, address := range s.urls {
			for _, path := range []string{"/manage/health", "/metrics", "/api/v1/docs", "/api/v1/openapi.json"} {
				request(t, http.MethodGet, address+path, "", nil, nil, http.StatusOK)
			}
		}
		var hotels hotelPage
		request(t, http.MethodGet, s.gateway+"/api/v1/hotels?page=1&size=1", "", nil, &hotels, http.StatusOK)
		if len(hotels.Items) != 1 || hotels.Items[0].HotelUID.String() != hotelUID {
			t.Fatalf("hotels: got %+v, want seed hotel %s", hotels, hotelUID)
		}
	})
	t.Run("booking and cancellation", func(t *testing.T) {
		user := t.Name()
		input := createRequest{HotelUID: hotelUID, StartDate: bookingStartDate, EndDate: bookingEndDate}
		loyalty(t, s, user, 0, 5, "BRONZE")
		var created createdResult
		body := request(t, http.MethodPost, s.gateway+"/api/v1/reservations", user, input, &created, http.StatusOK)
		assertPublicReservation(t, body)
		if created.Discount != 5 || created.Payment != (paymentResult{Status: paidStatus, Price: 28500}) {
			t.Fatalf("created: got %+v, want 5%% discount and paid price 28500", created)
		}
		path := s.gateway + "/api/v1/reservations/" + created.ReservationUID.String()
		var item reservationResult
		body = request(t, http.MethodGet, path, user, nil, &item, http.StatusOK)
		assertPublicReservation(t, body)
		if item.Status != paidStatus || item.Hotel.FullAddress != "Россия, Москва, Неглинная ул., 4" {
			t.Fatalf("reservation: got %+v", item)
		}
		request(t, http.MethodDelete, path, user, nil, nil, http.StatusNoContent)
		request(t, http.MethodGet, path, user, nil, &item, http.StatusOK)
		if item.Status != canceledStatus || item.Payment.Status != canceledStatus {
			t.Fatalf("canceled reservation: got %+v", item)
		}
		request(t, http.MethodDelete, path, user, nil, nil, http.StatusNoContent)
		loyalty(t, s, user, 0, 5, "BRONZE")
		var me userResult
		request(t, http.MethodGet, s.gateway+"/api/v1/me", user, nil, &me, http.StatusOK)
		if len(me.Reservations) != 1 || me.Loyalty.ReservationCount != 0 {
			t.Fatalf("me: got %+v", me)
		}
		var list []reservationResult
		request(t, http.MethodGet, s.gateway+"/api/v1/reservations", user, nil, &list, http.StatusOK)
		if len(list) != 1 || list[0].Status != canceledStatus {
			t.Fatalf("reservations: got %+v", list)
		}
	})
	t.Run("ownership", func(t *testing.T) {
		user := t.Name()
		var created createdResult
		request(t, http.MethodPost, s.gateway+"/api/v1/reservations", user,
			createRequest{HotelUID: hotelUID, StartDate: bookingStartDate, EndDate: bookingEndDate}, &created, http.StatusOK)
		path := s.gateway + "/api/v1/reservations/" + created.ReservationUID.String()
		request(t, http.MethodGet, path, user+" stranger", nil, nil, http.StatusNotFound)
		request(t, http.MethodDelete, path, user+" stranger", nil, nil, http.StatusNotFound)
		var item reservationResult
		request(t, http.MethodGet, path, user, nil, &item, http.StatusOK)
		if item.Status != paidStatus || item.Payment.Status != paidStatus {
			t.Fatalf("foreign cancellation changed %+v", item)
		}
		loyalty(t, s, user, 1, 5, "BRONZE")
	})
	t.Run("loyalty boundaries", func(t *testing.T) {
		user := t.Name()
		input := createRequest{HotelUID: hotelUID, StartDate: bookingStartDate, EndDate: bookingEndDate}
		ids := make([]string, 0, 20)
		for i := range 20 {
			var out createdResult
			request(t, http.MethodPost, s.gateway+"/api/v1/reservations", user, input, &out, http.StatusOK)
			discount := 5
			if i >= 10 {
				discount = 7
			}
			if out.Discount != discount || out.Payment.Price != 30000*(100-discount)/100 {
				t.Fatalf("booking %d: got %+v, want discount %d", i+1, out, discount)
			}
			ids = append(ids, out.ReservationUID.String())
			switch i + 1 {
			case 9:
				loyalty(t, s, user, 9, 5, "BRONZE")
			case 10:
				loyalty(t, s, user, 10, 7, "SILVER")
			case 19:
				loyalty(t, s, user, 19, 7, "SILVER")
			case 20:
				loyalty(t, s, user, 20, 10, "GOLD")
			}
		}
		request(t, http.MethodDelete, s.gateway+"/api/v1/reservations/"+ids[19], user, nil, nil, http.StatusNoContent)
		loyalty(t, s, user, 19, 7, "SILVER")
		for i := 18; i >= 9; i-- {
			request(t, http.MethodDelete, s.gateway+"/api/v1/reservations/"+ids[i], user, nil, nil, http.StatusNoContent)
		}
		loyalty(t, s, user, 9, 5, "BRONZE")
	})
	t.Run("invalid input", func(t *testing.T) {
		user := t.Name()
		request(t, http.MethodPost, s.gateway+"/api/v1/reservations", user,
			createRequest{HotelUID: hotelUID, StartDate: bookingStartDate, EndDate: bookingStartDate},
			nil, http.StatusBadRequest)
		loyalty(t, s, user, 0, 5, "BRONZE")
		request(t, http.MethodGet, s.gateway+"/api/v1/loyalty", "", nil, nil, http.StatusBadRequest)
	})
	t.Run("partial failures", func(t *testing.T) { partialFailures(t, s) })
}

func assertPublicReservation(t *testing.T, body []byte) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["paymentUid"]; exists {
		t.Fatalf("internal paymentUid in reservation response: %s", body)
	}
}
