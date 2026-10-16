package schema

import (
	"uuid"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Reservation struct{ ent.Schema }

func (Reservation) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "reservation"}}
}

func (Reservation) Fields() []ent.Field {
	return []ent.Field{
		field.String("reservation_uid").GoType(uuid.UUID{}).ValueScanner(UUIDScanner).SchemaType(map[string]string{
			dialect.Postgres: uuidType,
		}).Unique(),
		field.String("username").MaxRuneLen(80),
		field.String("payment_uid").GoType(uuid.UUID{}).ValueScanner(UUIDScanner).SchemaType(map[string]string{
			dialect.Postgres: uuidType,
		}),
		field.Int("hotel_id").Optional().Nillable(),
		field.Enum("status").Values("PAID", "CANCELED"),
		field.Time("start_date").Optional().Nillable(),
		field.Time("end_date").StorageKey("end_data").Optional().Nillable(),
	}
}

func (Reservation) Edges() []ent.Edge {
	return []ent.Edge{edge.From("hotel", Hotel.Type).Ref("reservations").Field("hotel_id").Unique()}
}

func (Reservation) Indexes() []ent.Index {
	return []ent.Index{index.Fields("username")}
}
