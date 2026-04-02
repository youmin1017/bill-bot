package service

import (
	"context"

	"bill-bot/ent"
	"bill-bot/ent/ledger"
)

// GetLedger returns the ledger for a channel, or nil if not found.
func GetLedger(ctx context.Context, client *ent.Client, channelID string) (*ent.Ledger, error) {
	return client.Ledger.Query().
		Where(ledger.ChannelID(channelID)).
		Only(ctx)
}

// CreateLedger creates a new ledger for a channel.
func CreateLedger(ctx context.Context, client *ent.Client, channelID, guildID, categoryID string) (*ent.Ledger, error) {
	return client.Ledger.Create().
		SetChannelID(channelID).
		SetGuildID(guildID).
		SetCategoryID(categoryID).
		Save(ctx)
}
