package schema

import (
	"database/sql"
	"testing"
	"uuid"
)

func TestUUIDScanner(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("049161bb-badd-4fa8-9d90-87c9a82b0668")
	value, err := UUIDScanner.Value(id)
	if err != nil || value != id.String() {
		t.Fatalf("SQL value=%v err=%v", value, err)
	}
	scanner := UUIDScanner.ScanValue()
	if err := scanner.Scan(value); err != nil {
		t.Fatal(err)
	}
	got, err := UUIDScanner.FromValue(scanner)
	if err != nil || got != id {
		t.Fatalf("UUID=%v err=%v", got, err)
	}
	if _, err := UUIDScanner.FromValue(&sql.NullString{String: "invalid", Valid: true}); err == nil {
		t.Fatal("invalid UUID accepted")
	}
	if got, err := UUIDScanner.FromValue(&sql.NullString{}); err != nil || got != uuid.Nil() {
		t.Fatalf("null UUID=%v err=%v", got, err)
	}
}
