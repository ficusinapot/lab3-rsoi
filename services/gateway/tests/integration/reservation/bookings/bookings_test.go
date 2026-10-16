//go:build integration

package bookings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"uuid"

	"lab2/gateway/internal/httpclient"
	"lab2/gateway/internal/models/entities"
	"lab2/gateway/internal/reservation/repos"
)

func TestReservationAdapter(t *testing.T) {
	t.Parallel()
	const hotelID = "049161bb-badd-4fa8-9d90-87c9a82b0668"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/hotels" || r.URL.Query().Get("page") != "1" || r.URL.Query().Get("size") != "10" {
				t.Errorf("unexpected hotels request: %s", r.URL)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"page":1,"pageSize":10,"totalElements":1,"items":[{"hotelUid":"` +
				hotelID + `","name":"Hotel","country":"RU","city":"Moscow","address":"Street","stars":null,"price":100}]}`))
		case http.MethodPost:
			if r.URL.Path != "/reservations" || r.Header.Get("X-User-Name") != "user" {
				t.Errorf("unexpected reservation request: %s", r.URL)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["paymentUid"] == nil ||
				body["hotelUid"] != hotelID {
				t.Errorf("unexpected request body: %v, %v", body, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"reservationUid":"` + uuid.New().String() + `","hotelUid":"` + hotelID +
				`","paymentUid":"` + body["paymentUid"].(string) +
				`","status":"PAID","startDate":"2026-01-01","endDate":"2026-01-04"}`))
		default:
			t.Errorf("unexpected method: %s", r.Method)
		}
	}))
	t.Cleanup(server.Close)
	client := httpclient.New(httpclient.Config{
		Timeout: time.Second, MaxInFlight: 2, MaxIdleConnections: 4,
		MaxIdleConnectionsPerHost: 2, IdleConnectionTimeout: time.Second,
	})
	t.Cleanup(client.CloseIdleConnections)
	breaker := httpclient.NewBreaker(httpclient.BreakerConfig{FailureThreshold: 5, ResetTimeout: time.Second})
	repo, err := repos.New(client, server.URL, breaker)
	if err != nil {
		t.Fatal(err)
	}
	page, size := 1, 10
	items, err := repo.Hotels(t.Context(), entities.PageQuery{Page: &page, Size: &size})
	if err != nil || len(items.Items) != 1 || items.Items[0].Name != "Hotel" || items.Items[0].Stars != nil {
		t.Fatalf("hotels: %+v, %v", items, err)
	}
	paymentID := uuid.New().String()
	row, err := repo.CreateReservation(t.Context(), "user", entities.InternalCreate{
		HotelUID: uuid.MustParse(hotelID), StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:    time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC),
		PaymentUID: uuid.MustParse(paymentID),
	})
	if err != nil || row.PaymentUID.String() != paymentID || row.Status != entities.ReservationPaid {
		t.Fatalf("reservation: %+v, %v", row, err)
	}
}
