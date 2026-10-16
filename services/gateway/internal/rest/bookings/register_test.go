package bookings

//go:generate go tool mockgen -source=../../models/coreifc/bookings.go -destination=service_mock_test.go -package=bookings

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/go-chi/chi/v5"
	"go.uber.org/mock/gomock"
	model "lab2/gateway/internal/models/entities"
	"lab2/platform/rest"
)

const (
	loyaltyPath     = "/api/v1/loyalty"
	jsonContentType = "application/json"
)

func TestGatewayContract(t *testing.T) {
	t.Parallel()
	uid := uuid.MustParse("049161bb-badd-4fa8-9d90-87c9a82b0668")
	reservationUID := uuid.MustParse("9b4ba1f7-e5ac-465b-ace4-7b54dec20f9a")
	const testUser = "user"
	const reservationsPath = "/api/v1/reservations"
	const validBody = `{"hotelUid":"049161bb-badd-4fa8-9d90-87c9a82b0668","startDate":"2026-01-01","endDate":"2026-01-04"}`
	for _, tc := range []struct {
		name        string
		method      string
		path        string
		body        string
		user        string
		contentType string
		status      int
		wantBody    string
		expect      func(*MockBookingCommands, *MockBookingQueries)
	}{
		{
			name:     "loyalty",
			method:   http.MethodGet,
			path:     loyaltyPath,
			user:     testUser,
			status:   http.StatusOK,
			wantBody: `{"status":"BRONZE","discount":5,"reservationCount":0}`,
			expect: func(commands *MockBookingCommands, queries *MockBookingQueries) {
				queries.EXPECT().Loyalty(gomock.Any(), testUser).
					Return(model.Loyalty{Status: model.LoyaltyBronze, Discount: 5}, nil)
			},
		},
		{
			name: "loyalty unavailable", method: http.MethodGet, path: loyaltyPath,
			user: testUser, status: http.StatusServiceUnavailable,
			wantBody: `{"message":"Loyalty Service unavailable"}`,
			expect: func(commands *MockBookingCommands, queries *MockBookingQueries) {
				queries.EXPECT().Loyalty(gomock.Any(), testUser).Return(model.Loyalty{}, model.ErrLoyaltyUnavailable)
			},
		},
		{
			name: "critical reservation unavailable", method: http.MethodGet, path: reservationsPath,
			user: testUser, status: http.StatusInternalServerError,
			wantBody: `{"message":"Reservation Service unavailable"}`,
			expect: func(commands *MockBookingCommands, queries *MockBookingQueries) {
				queries.EXPECT().List(gomock.Any(), testUser).Return(nil, model.ErrReservationUnavailable)
			},
		},
		{
			name: "queue full before creation", method: http.MethodPost, path: reservationsPath,
			user: testUser, contentType: jsonContentType, body: validBody, status: http.StatusServiceUnavailable,
			wantBody: `{"message":"retry queue full"}`,
			expect: func(commands *MockBookingCommands, queries *MockBookingQueries) {
				commands.EXPECT().Create(gomock.Any(), gomock.Any()).Return(model.Created{}, model.ErrQueueFull)
			},
		},
		{
			name: "me loyalty fallback", method: http.MethodGet, path: "/api/v1/me",
			user: testUser, status: http.StatusOK, wantBody: `{"reservations":[],"loyalty":{}}`,
			expect: func(commands *MockBookingCommands, queries *MockBookingQueries) {
				queries.EXPECT().Me(gomock.Any(), testUser).Return(model.UserInfo{Reservations: []model.Reservation{}}, nil)
			},
		},

		{
			name:     "hotels pagination",
			method:   http.MethodGet,
			path:     "/api/v1/hotels?page=0&size=1",
			status:   http.StatusOK,
			wantBody: `{"page":0,"pageSize":1,"totalElements":0,"items":[]}`,
			expect: func(commands *MockBookingCommands, queries *MockBookingQueries) {
				queries.EXPECT().Hotels(gomock.Any(), model.PageQuery{Page: new(0), Size: new(1)}).
					Return(model.Page{PageSize: 1, Items: []model.Hotel{}}, nil)
			},
		},
		{
			name:     "empty reservations",
			method:   http.MethodGet,
			path:     reservationsPath,
			user:     testUser,
			status:   http.StatusOK,
			wantBody: `[]`,
			expect: func(commands *MockBookingCommands, queries *MockBookingQueries) {
				queries.EXPECT().List(gomock.Any(), testUser).Return([]model.Reservation{}, nil)
			},
		},
		{
			name:     "user info",
			method:   http.MethodGet,
			path:     "/api/v1/me",
			user:     testUser,
			status:   http.StatusOK,
			wantBody: `{"reservations":[],"loyalty":{"status":"BRONZE","discount":5,"reservationCount":0}}`,
			expect: func(commands *MockBookingCommands, queries *MockBookingQueries) {
				queries.EXPECT().Me(gomock.Any(), testUser).Return(model.UserInfo{
					Reservations: []model.Reservation{},
					Loyalty:      model.Loyalty{Status: model.LoyaltyBronze, Discount: 5},
				}, nil)
			},
		},
		{
			name:   "foreign reservation",
			method: http.MethodGet,
			path:   "/api/v1/reservations/" + reservationUID.String(),
			user:   "stranger",
			status: http.StatusNotFound,
			expect: func(commands *MockBookingCommands, queries *MockBookingQueries) {
				queries.EXPECT().Get(gomock.Any(), reservationUID, "stranger").Return(model.Reservation{}, model.ErrNotFound)
			},
		},
		{
			name:   "cancel",
			method: http.MethodDelete,
			path:   "/api/v1/reservations/" + reservationUID.String(),
			user:   testUser,
			status: http.StatusNoContent,
			expect: func(commands *MockBookingCommands, queries *MockBookingQueries) {
				commands.EXPECT().Cancel(gomock.Any(), model.CancelBooking{
					ReservationUID: reservationUID, User: testUser,
				}).Return(nil)
			},
		},
		{
			name:        "JSON charset",
			method:      http.MethodPost,
			path:        reservationsPath,
			user:        testUser,
			body:        validBody,
			contentType: "application/json; charset=utf-8",
			status:      http.StatusOK,
			wantBody: `{"reservationUid":"9b4ba1f7-e5ac-465b-ace4-7b54dec20f9a",
"hotelUid":"049161bb-badd-4fa8-9d90-87c9a82b0668",
"startDate":"2026-01-01",
"endDate":"2026-01-04",
"discount":5,
"status":"PAID",
"payment":{"status":"PAID",
"price":28500}}`,
			expect: func(commands *MockBookingCommands, queries *MockBookingQueries) {
				start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				end := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
				commands.EXPECT().Create(gomock.Any(), model.CreateBooking{
					User:     testUser,
					HotelUID: uid, StartDate: start, EndDate: end,
				}).Return(model.Created{
					ReservationUID: reservationUID, HotelUID: uid, StartDate: start, EndDate: end,
					Discount: 5, Status: model.ReservationPaid, Payment: model.Payment{Status: model.PaymentPaid, Price: 28500},
				}, nil)
			},
		},
		{
			name:   "missing user",
			method: http.MethodGet,
			path:   loyaltyPath,
			status: http.StatusBadRequest,
		},
		{
			name:   "fractional page",
			method: http.MethodGet,
			path:   "/api/v1/hotels?page=1.5",
			status: http.StatusBadRequest,
		},
		{
			name:   "invalid hotel UUID",
			method: http.MethodPost,
			path:   reservationsPath,
			user:   testUser,
			body:   strings.Replace(validBody, uid.String(), "bad", 1),
			status: http.StatusBadRequest,
		},
		{
			name:   "invalid start date",
			method: http.MethodPost,
			path:   reservationsPath,
			user:   testUser,
			body:   strings.Replace(validBody, "2026-01-01", "bad", 1),
			status: http.StatusBadRequest,
		},
		{
			name:   "malformed JSON",
			method: http.MethodPost,
			path:   reservationsPath,
			user:   testUser,
			body:   `{`,
			status: http.StatusBadRequest,
		},
		{
			name:   "oversized body",
			method: http.MethodPost,
			path:   reservationsPath,
			user:   testUser,
			body:   strings.Repeat(" ", 4096) + validBody,
			status: http.StatusBadRequest,
		},
		{
			name:        "invalid content type",
			method:      http.MethodPost,
			path:        reservationsPath,
			user:        testUser,
			body:        validBody,
			contentType: "text/plain",
			status:      http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			commands := NewMockBookingCommands(ctrl)
			queries := NewMockBookingQueries(ctrl)
			if tc.expect != nil {
				tc.expect(commands, queries)
			}
			router := chi.NewRouter()
			Register(
				router,
				commands, queries,
				slog.New(slog.NewTextHandler(io.Discard, nil)),
				rest.Config{APIPrefix: "/api/v1", MaxBodyBytes: 4096},
			)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.user != "" {
				req.Header.Set("X-User-Name", tc.user)
			}
			contentType := tc.contentType
			if contentType == "" {
				contentType = jsonContentType
			}
			if tc.body != "" {
				req.Header.Set("Content-Type", contentType)
			}
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			if out.Code != tc.status {
				t.Fatalf("status: got %d, want %d; body: %s", out.Code, tc.status, out.Body)
			}
			if tc.status == http.StatusNoContent {
				if out.Body.Len() != 0 {
					t.Fatalf("204 body: %s", out.Body)
				}
				return
			}
			if got := out.Header().Get("Content-Type"); got != jsonContentType {
				t.Fatalf("content type: got %q", got)
			}
			if tc.wantBody != "" {
				var got, want any
				if err := json.Unmarshal(out.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(tc.wantBody), &want); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("body: got %s, want %s", out.Body, tc.wantBody)
				}
			}
		})
	}
}

func TestNestedFallbackAndHealthyZeros(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "empty payment", value: paymentResponse(model.Payment{}), want: `{}`},
		{
			name: "free payment", value: paymentResponse(model.Payment{Status: model.PaymentPaid}),
			want: `{"status":"PAID","price":0}`,
		},
		{name: "empty loyalty", value: loyaltyResponse(model.Loyalty{}), want: `{}`},
		{
			name: "new loyalty", value: loyaltyResponse(model.Loyalty{Status: model.LoyaltyBronze, Discount: 5}),
			want: `{"status":"BRONZE","discount":5,"reservationCount":0}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("JSON = %s, want %s", data, tc.want)
			}
		})
	}
}
