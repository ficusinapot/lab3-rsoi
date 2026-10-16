package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type BookingRequest struct{ ent.Schema }

func (BookingRequest) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "booking_requests"}}
}

func (BookingRequest) Fields() []ent.Field {
	return []ent.Field{
		field.String("operation_id").MaxLen(36).Unique(),
		field.String("username").MaxRuneLen(80),
		field.Bool("canceled").Default(false),
	}
}
