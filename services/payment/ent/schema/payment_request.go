package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type PaymentRequest struct{ ent.Schema }

func (PaymentRequest) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "payment_requests"}}
}

func (PaymentRequest) Fields() []ent.Field {
	return []ent.Field{
		field.String("operation_id").MaxLen(36).Unique(), field.Bool("canceled").Default(false),
	}
}
