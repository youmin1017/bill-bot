package service

import (
	"context"

	"bill-bot/ent"
	"bill-bot/ent/ledger"
	"bill-bot/ent/ledgermember"
)

// UpsertMember ensures a user exists as an active LedgerMember.
// If the record exists but active=false, it is reactivated and user_name updated.
// If the record does not exist, it is created.
func UpsertMember(ctx context.Context, client *ent.Client, ledgerID int, userID, userName string) error {
	existing, err := client.LedgerMember.Query().
		Where(
			ledgermember.UserID(userID),
			ledgermember.HasLedgerWith(ledger.ID(ledgerID)),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return err
	}
	if existing != nil {
		if !existing.Active || existing.UserName != userName {
			return existing.Update().
				SetActive(true).
				SetUserName(userName).
				Exec(ctx)
		}
		return nil
	}
	return client.LedgerMember.Create().
		SetLedgerID(ledgerID).
		SetUserID(userID).
		SetUserName(userName).
		Exec(ctx)
}
