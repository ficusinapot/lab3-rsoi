package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type Loyalty struct{ ent.Schema }

func (Loyalty) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "loyalty"}}
}

func (Loyalty) Fields() []ent.Field {
	return []ent.Field{
		field.String("username").MaxRuneLen(80).Unique(), field.Int("reservation_count").Default(0),
		field.Enum("status").Values("BRONZE", "SILVER", "GOLD").Default("BRONZE"), field.Int("discount"),
	}
}
