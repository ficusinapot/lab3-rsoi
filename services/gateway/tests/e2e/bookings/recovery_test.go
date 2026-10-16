//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"uuid"
)

func recoveryInput() createRequest {
	return createRequest{HotelUID: hotelUID, StartDate: bookingStartDate, EndDate: bookingEndDate}
}

func createRecoveryBooking(t *testing.T, gateway, user string) createdResult {
	t.Helper()
	var booking createdResult
	request(t, http.MethodPost, gateway+"/api/v1/reservations", user, recoveryInput(), &booking, http.StatusOK)
	return booking
}

func assertRecoveryBooking(t *testing.T, gateway, user, id, status string) {
	t.Helper()
	var booking reservationResult
	request(t, http.MethodGet, gateway+"/api/v1/reservations/"+id, user, nil, &booking, http.StatusOK)
	if booking.Status != status || booking.Payment.Status != status {
		t.Fatalf("booking = %+v", booking)
	}
}

func interruptedCreate(t *testing.T, gateway, user string) <-chan error {
	t.Helper()
	data, err := json.Marshal(recoveryInput())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway+"/api/v1/reservations", bytes.NewReader(data))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Name", user)
	done := make(chan error, 1)
	go func() {
		defer cancel()
		response, err := http.DefaultClient.Do(req)
		if err == nil {
			err = fmt.Errorf("interrupted request returned HTTP %d", response.StatusCode)
			if closeErr := response.Body.Close(); closeErr != nil {
				err = closeErr
			}
		}
		done <- err
	}()
	return done
}

func waitInterrupted(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("interrupted request unexpectedly succeeded")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("interrupted request did not finish")
	}
}

//nolint:paralleltest // Subtests share proxies and restart one native Gateway.
func TestRecovery(t *testing.T) {
	t.Parallel()
	s := newStack(t)
	proxies := map[string]*faultProxy{
		paymentServiceName:     newFaultProxy(t, s.addresses[paymentServiceName]),
		reservationServiceName: newFaultProxy(t, s.addresses[reservationServiceName]),
		loyaltyServiceName:     newFaultProxy(t, s.addresses[loyaltyServiceName]),
	}
	gateway := newNativeGateway(t, s, proxies)
	observed := &stack{
		gateway: gateway.address,
		db:      s.db,
	}

	t.Run("cancel waits for the visible reservation to finish creation", func(t *testing.T) {
		user := "concurrent-create-" + uuid.New().String()
		rule := proxies[loyaltyServiceName].arm(t, http.MethodPost, loyaltyReservationsPath, faultHoldBefore)
		done := interruptedCreate(t, gateway.address, user)
		_ = awaitCapture(t, rule)
		var rows []struct {
			ReservationUID uuid.UUID `json:"reservationUid"`
			Status         string    `json:"status"`
		}
		request(t, http.MethodGet, gateway.address+"/api/v1/reservations", user, nil, &rows, http.StatusOK)
		if len(rows) != 1 || rows[0].Status != "PAID" {
			t.Fatal("pending booking not visible", rows)
		}
		id := rows[0].ReservationUID.String()
		bookingURL := gateway.address + "/api/v1/reservations/" + id
		request(t, http.MethodDelete, bookingURL, user, nil, nil, http.StatusServiceUnavailable)
		rule.unblock()
		waitInterrupted(t, done)
		// The proxy drops the committed increment response. ResolveCreation must retain the paid booking.
		await(t, func() bool { return gateway.completed(t, "create:"+id) })
		assertRecoveryBooking(t, gateway.address, user, id, "PAID")
		if count := reservationCount(t, observed, user); count != 1 {
			t.Fatal("creation count", count)
		}
		request(t, http.MethodDelete, bookingURL, user, nil, nil, http.StatusNoContent)
		assertRecoveryBooking(t, gateway.address, user, id, "CANCELED")
		await(t, func() bool { return gateway.completed(t, "cancel:"+id) })
		loyalty(t, observed, user, 0, 5, "BRONZE")
	})

	for _, service := range []string{paymentServiceName, reservationServiceName, loyaltyServiceName} {
		t.Run("lost committed response from "+service, func(t *testing.T) {
			user := "recovery-" + uuid.New().String()
			baseline := createRecoveryBooking(t, gateway.address, user)
			path := "/api/v1/payments"
			if service == reservationServiceName {
				path = "/api/v1/reservations"
			}
			if service == loyaltyServiceName {
				path = loyaltyReservationsPath
			}
			rule := proxies[service].arm(t, http.MethodPost, path, faultDropAfter)
			before := proxies[paymentServiceName].paymentPosts.Load()
			want := []int{http.StatusInternalServerError}
			if service == paymentServiceName {
				want = []int{http.StatusBadGateway}
			}
			if service == loyaltyServiceName {
				want = []int{http.StatusServiceUnavailable, http.StatusOK}
			}
			request(t, http.MethodPost, gateway.address+"/api/v1/reservations", user, recoveryInput(), nil, want...)
			captured := awaitCapture(t, rule)
			if proxies[paymentServiceName].paymentPosts.Load()-before != 1 {
				t.Fatal("Payment POST was repeated")
			}
			if service == loyaltyServiceName {
				id := strings.TrimSuffix(captured.Key, ":create")
				assertRecoveryBooking(t, gateway.address, user, id, paidStatus)
				loyalty(t, observed, user, 2, 5, "BRONZE")
				await(t, func() bool { return gateway.completed(t, "create:"+id) })
			} else {
				var body struct {
					PaymentUID     uuid.UUID `json:"paymentUid"`
					ReservationUID uuid.UUID `json:"reservationUid"`
				}
				if err := json.Unmarshal(captured.Body, &body); err != nil {
					t.Fatal(err)
				}
				var payment paymentResult
				request(t, http.MethodGet, s.addresses[paymentServiceName]+"/api/v1/payments/"+body.PaymentUID.String(),
					"", nil, &payment, http.StatusOK)
				if payment.Status != canceledStatus {
					t.Fatal(payment)
				}
				if service == reservationServiceName {
					assertRecoveryBooking(t, gateway.address, user, body.ReservationUID.String(), canceledStatus)
				}
				loyalty(t, observed, user, 1, 5, "BRONZE")
			}
			assertRecoveryBooking(t, gateway.address, user, baseline.ReservationUID.String(), paidStatus)
		})
	}

	for _, stage := range []string{faultHoldAfter, faultHoldBefore} {
		t.Run("crash around Loyalty commit "+stage, func(t *testing.T) {
			user := "crash-" + uuid.New().String()
			createRecoveryBooking(t, gateway.address, user)
			rule := proxies[loyaltyServiceName].arm(t, http.MethodPost, loyaltyReservationsPath, stage)
			done := interruptedCreate(t, gateway.address, user)
			captured := awaitCapture(t, rule)
			id := strings.TrimSuffix(captured.Key, ":create")
			gateway.kill(t)
			waitInterrupted(t, done)
			if stage == faultHoldAfter {
				rule.unblock()
			}
			gateway.start(t)
			await(t, func() bool { return gateway.completed(t, "create:"+id) })
			if stage == faultHoldAfter {
				assertRecoveryBooking(t, gateway.address, user, id, paidStatus)
				loyalty(t, observed, user, 2, 5, "BRONZE")
			} else {
				assertRecoveryBooking(t, gateway.address, user, id, canceledStatus)
				rule.unblock()
				select {
				case status := <-rule.outcome:
					if status != http.StatusBadRequest {
						t.Fatalf("late POST status = %d", status)
					}
				case <-time.After(10 * time.Second):
					t.Fatal("late POST did not finish")
				}
				loyalty(t, observed, user, 1, 5, "BRONZE")
			}
		})
	}

	t.Run("deferred cancellation survives restart", func(t *testing.T) {
		user := "cancel-" + uuid.New().String()
		booking := createRecoveryBooking(t, gateway.address, user)
		rule := proxies[loyaltyServiceName].arm(t, http.MethodDelete, loyaltyReservationsPath, faultUnavailable)
		id := booking.ReservationUID.String()
		request(t, http.MethodDelete, gateway.address+"/api/v1/reservations/"+id, user, nil, nil, http.StatusNoContent)
		assertRecoveryBooking(t, gateway.address, user, id, canceledStatus)
		loyalty(t, observed, user, 1, 5, "BRONZE")
		gateway.kill(t)
		gateway.start(t)
		rule.unblock()
		await(t, func() bool { return gateway.completed(t, "cancel:"+id) })
		loyalty(t, observed, user, 0, 5, "BRONZE")
	})
}
