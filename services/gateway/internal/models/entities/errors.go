package entities

import "errors"

var (
	ErrQueueUnavailable = errors.New("operation store unavailable")
	ErrOperationBusy    = errors.New("operation is already owned")

	ErrLoyaltyUnavailable     = errors.New("loyalty Service unavailable")
	ErrReservationUnavailable = errors.New("reservation Service unavailable")
	ErrPaymentUnavailable     = errors.New("payment Service unavailable")
	ErrQueueFull              = errors.New("retry queue full")
	ErrInvalidInput           = errors.New("invalid input")
	ErrNotFound               = errors.New("not found")
	ErrUpstreamUnavailable    = errors.New("upstream unavailable")
	ErrUpstreamTimeout        = errors.New("upstream timeout")
	ErrOperationPanic         = errors.New("operation panicked")
)
