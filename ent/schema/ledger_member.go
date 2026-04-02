package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// LedgerMember holds the schema definition for the LedgerMember entity.
type LedgerMember struct {
	ent.Schema
}

func (LedgerMember) Fields() []ent.Field {
	return []ent.Field{
		field.String("user_id"),
		field.String("user_name"),
		field.Bool("active").Default(true),
		field.Time("joined_at").Immutable().Default(time.Now),
	}
}

func (LedgerMember) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ledger", Ledger.Type).Ref("members").Unique().Required(),
	}
}

func (LedgerMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id").Edges("ledger").Unique(),
	}
}
