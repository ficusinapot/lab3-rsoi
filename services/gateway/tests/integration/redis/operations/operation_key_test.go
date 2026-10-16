//go:build integration

package operations

import (
	"crypto/sha256"
	"uuid"
)

func testKey(value any) uuid.UUID {
	if id, ok := value.(uuid.UUID); ok {
		return id
	}
	label := value.(string)
	if label == "" {
		return uuid.Nil()
	}
	if id, err := uuid.Parse(label); err == nil {
		return id
	}
	digest := sha256.Sum256([]byte(label))
	var id uuid.UUID
	copy(id[:], digest[:16])
	id[6] = (id[6] & 0x0f) | 0x80
	id[8] = (id[8] & 0x3f) | 0x80
	return id
}
