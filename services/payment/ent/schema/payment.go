package schema

import (
	"uuid"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Payment struct{ ent.Schema }

func (Payment) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "payment"}}
}

func (Payment) Fields() []ent.Field {
	return []ent.Field{
		field.String("payment_uid").GoType(uuid.UUID{}).ValueScanner(UUIDScanner).SchemaType(map[string]string{
			dialect.Postgres: uuidType,
		}),
		field.Enum("status").Values("PAID", "CANCELED"), field.Int("price"),
	}
}

func (Payment) Indexes() []ent.Index {
	return []ent.Index{index.Fields("payment_uid").Unique()}
}
