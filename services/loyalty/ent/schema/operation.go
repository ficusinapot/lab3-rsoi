package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type Operation struct{ ent.Schema }

func (Operation) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "loyalty_operations"}}
}

func (Operation) Fields() []ent.Field {
	return []ent.Field{
		field.String("operation_id").MaxLen(128).Unique(),
		field.String("username").MaxRuneLen(80),
		field.Int("delta"),
	}
}
