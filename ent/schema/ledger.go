package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Ledger holds the schema definition for the Ledger entity.
type Ledger struct {
	ent.Schema
}

func (Ledger) Fields() []ent.Field {
	return []ent.Field{
		field.String("channel_id").Unique().Immutable(),
		field.String("guild_id").Immutable(),
		field.String("category_id").Optional(),       // 所屬 Category ID
		field.String("pinned_message_id").Optional(), // 置頂摘要訊息 ID
		field.Bool("active").Default(true),
		field.Time("created_at").Immutable().Default(time.Now),
	}
}

func (Ledger) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("expenses", Expense.Type),
		edge.To("payments", Payment.Type),
		edge.To("members", LedgerMember.Type),
	}
}

func (Ledger) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("guild_id"),
		index.Fields("guild_id", "channel_id"),
	}
}
