package converters

import (
	"testing"

	"lab2/gateway/internal/clients/contracts"
)

func TestPageEntityEmpty(t *testing.T) {
	t.Parallel()
	got := PageEntity(contracts.HotelPage{Page: 1, PageSize: 10})
	if got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("items = %v, want nonnil empty slice", got.Items)
	}
}
