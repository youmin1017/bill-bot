package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Expense holds the schema definition for the Expense entity.
type Expense struct {
	ent.Schema
}

func (Expense) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("amount"),                        // 以分為單位，避免浮點數問題（NT$12.50 = 1250）
		field.String("currency").Default("TWD"),
		field.String("description"),
		field.String("payer_id"),                     // Discord user_id
		field.Enum("type").Values("split", "for"),
		field.Bool("deleted").Default(false),
		field.Time("created_at").Immutable().Default(time.Now),
	}
}

func (Expense) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ledger", Ledger.Type).Ref("expenses").Unique().Required(),
		edge.To("splits", Split.Type),
	}
}
