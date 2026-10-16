package schema

import (
	"uuid"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Hotel struct{ ent.Schema }

func (Hotel) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "hotels"}}
}

func (Hotel) Fields() []ent.Field {
	return []ent.Field{
		field.String("hotel_uid").GoType(uuid.UUID{}).ValueScanner(UUIDScanner).SchemaType(map[string]string{
			dialect.Postgres: uuidType,
		}).Unique(),
		field.String("name").MaxLen(255),
		field.String("country").MaxLen(80),
		field.String("city").MaxLen(80),
		field.String("address").MaxLen(255),
		field.Int("stars").Optional().Nillable(),
		field.Int("price"),
	}
}

func (Hotel) Edges() []ent.Edge {
	return []ent.Edge{edge.To("reservations", Reservation.Type)}
}
