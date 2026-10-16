package bookings

//go:generate go tool mockgen -source=../../../models/reservationifc/bookings.go -destination=reservation_mock_test.go -package=bookings -mock_names=Bookings=MockReservations,Canceler=MockReservationCanceler,Reader=MockReservationReader,BookingSource=MockReservationBookingSource
//go:generate go tool mockgen -source=../../../models/paymentifc/payments.go -destination=payment_mock_test.go -package=bookings -mock_names=Canceler=MockPaymentCanceler,Reader=MockPaymentReader
//go:generate go tool mockgen -source=../../../models/loyaltyifc/loyalty.go -destination=loyalty_mock_test.go -package=bookings -mock_names=Client=MockLoyalty,Reader=MockLoyaltyReader,BookingSource=MockLoyaltyBookingSource
