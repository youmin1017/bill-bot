package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Payment 記錄還款事件
type Payment struct {
	ent.Schema
}

func (Payment) Fields() []ent.Field {
	return []ent.Field{
		field.String("from_user_id"),
		field.String("from_user_name"),
		field.String("to_user_id"),
		field.String("to_user_name"),
		field.Int64("amount"),
		field.String("note").Optional(),
		field.Time("created_at").Immutable().Default(time.Now),
	}
}

func (Payment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ledger", Ledger.Type).Ref("payments").Unique().Required(),
	}
}
