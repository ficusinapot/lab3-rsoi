package schema

import (
	"database/sql"
	"database/sql/driver"
	"uuid"

	"entgo.io/ent/schema/field"
	"github.com/samber/oops"
)

const uuidType = "uuid"

// UUIDScanner stores standard-library UUIDs as PostgreSQL UUID text.
var UUIDScanner = field.ValueScannerFunc[uuid.UUID, *sql.NullString]{
	V: func(id uuid.UUID) (driver.Value, error) { return id.String(), nil },
	S: func(value *sql.NullString) (uuid.UUID, error) {
		if !value.Valid {
			return uuid.Nil(), nil
		}
		id, err := uuid.Parse(value.String)
		return id, oops.Wrapf(err, "decode UUID")
	},
}
