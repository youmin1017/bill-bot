package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Split 記錄每筆帳目中各人應分攤的金額
type Split struct {
	ent.Schema
}

func (Split) Fields() []ent.Field {
	return []ent.Field{
		field.String("user_id"),
		field.Int64("amount"),      // 此人應付金額（分為單位）
		field.Bool("settled").Default(false),
	}
}

func (Split) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("expense", Expense.Type).Ref("splits").Unique().Required(),
	}
}
