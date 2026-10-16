//go:build integration

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
	"uuid"

	"lab2/reservation/ent/hotel"
	"lab2/reservation/ent/reservation"
	"lab2/reservation/tests/integration/fixtures"
	"lab2/reservation/tests/integration/infrastructure"
	"lab2/reservation/tests/integration/support"
)

//nolint:paralleltest // Autometrics uses process-global registration state.
func TestReservationAPI(t *testing.T) {
	db := infrastructure.NewDatabase(t)
	ctx := t.Context()
	hotelFixture := fixtures.SeedHotel(t, ctx, db.Client)
	hotelUID := hotelFixture.HotelUID
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := db.Client.Reservation.Delete().Where(reservation.HotelIDEQ(hotelFixture.ID)).Exec(cleanup); err != nil {
			t.Error(err)
		}
		if _, err := db.Client.Hotel.Delete().Where(hotel.HotelUIDEQ(hotelUID)).Exec(cleanup); err != nil {
			t.Error(err)
		}
	})
	api := support.NewHTTPClient(infrastructure.NewAPI(t, db))
	prefix := db.Config.HTTP.APIPrefix

	//nolint:paralleltest // API scenarios share database fixtures and metrics.
	t.Run("health hotels and validation", func(t *testing.T) {
		api.Request(t, http.MethodGet, "/manage/health", "", "", http.StatusOK)
		page := api.Request(t, http.MethodGet, prefix+"/hotels", "", "", http.StatusOK)
		if bytes.Contains(page.Body, []byte("$schema")) {
			t.Fatal("unexpected schema field in service response")
		}
		api.Request(t, http.MethodGet, prefix+"/hotels?page=0", "", "", http.StatusBadRequest)
		api.Request(t, http.MethodGet, prefix+"/hotels/"+hotelUID.String(), "", "", http.StatusOK)
		api.Request(t, http.MethodGet, prefix+"/hotels/"+uuid.New().String(), "", "", http.StatusNotFound)
		api.Request(t, http.MethodGet, prefix+"/reservations", "", "", http.StatusBadRequest)
	})

	body, err := json.Marshal(struct {
		HotelUID   string `json:"hotelUid"`
		PaymentUID string `json:"paymentUid"`
		StartDate  string `json:"startDate"`
		EndDate    string `json:"endDate"`
	}{
		HotelUID:   hotelUID.String(),
		PaymentUID: uuid.New().String(),
		StartDate:  "2026-10-01",
		EndDate:    "2026-10-03",
	})
	if err != nil {
		t.Fatal(err)
	}
	created := api.Request(t, http.MethodPost, prefix+"/reservations", "alice", string(body), http.StatusCreated)
	var item struct {
		ReservationUID string `json:"reservationUid"`
		Status         string `json:"status"`
	}
	if err := json.Unmarshal(created.Body, &item); err != nil {
		t.Fatal(err)
	}
	location := created.Header.Get("Location")
	if location != prefix+"/reservations/"+item.ReservationUID || item.Status != "PAID" {
		t.Fatalf("created response: %#v location %s", item, location)
	}
	//nolint:paralleltest // Cancellation follows creation of the same reservation.
	t.Run("ownership and repeated cancellation", func(t *testing.T) {
		api.Request(t, http.MethodGet, location, "bob", "", http.StatusNotFound)
		api.Request(t, http.MethodDelete, location, "bob", "", http.StatusNotFound)
		api.Request(t, http.MethodGet, location, "alice", "", http.StatusOK)
		api.Request(t, http.MethodDelete, location, "alice", "", http.StatusNoContent)
		api.Request(t, http.MethodDelete, location, "alice", "", http.StatusNoContent)
		canceled := api.Request(t, http.MethodGet, location, "alice", "", http.StatusOK)
		if err := json.Unmarshal(canceled.Body, &item); err != nil {
			t.Fatal(err)
		}
		if item.Status != "CANCELED" {
			t.Fatalf("reservation status = %q, want CANCELED", item.Status)
		}
	})
	//nolint:paralleltest // API scenarios share database fixtures and metrics.
	t.Run("documentation and malformed input", func(t *testing.T) {
		api.Request(t, http.MethodPost, prefix+"/reservations", "alice", `{"hotelUid":null}`, http.StatusUnprocessableEntity)
		api.Request(t, http.MethodGet, prefix+db.Config.OpenAPI.DocsPath, "", "", http.StatusOK)
		schema := api.Request(t, http.MethodGet, prefix+db.Config.OpenAPI.SchemaPath+".json", "", "", http.StatusOK)
		if !bytes.Contains(schema.Body, []byte("create-reservation")) {
			t.Fatal("OpenAPI operation is missing")
		}
	})

	//nolint:paralleltest // Metrics include requests from preceding scenarios.
	t.Run("metrics labels and collectors", func(t *testing.T) {
		scrape := api.Request(t, http.MethodGet, db.Config.Metrics.Path, "", "", http.StatusOK).Body
		for _, metric := range []string{
			"http_requests_total",
			"http_request_duration_seconds",
			"reservation_db_connections_in_use",
			"function_calls_total",
		} {
			if !bytes.Contains(scrape, []byte(metric)) {
				t.Errorf("missing metric %s", metric)
			}
		}
		if bytes.Contains(scrape, []byte(item.ReservationUID)) || bytes.Contains(scrape, []byte("alice")) {
			t.Fatal("metrics contain high-cardinality or personal labels")
		}
	})
}
