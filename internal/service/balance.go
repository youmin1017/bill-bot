package service

import (
	"context"
	"sort"

	"bill-bot/ent"
	"bill-bot/ent/expense"
	"bill-bot/ent/ledger"
	"bill-bot/ent/payment"
	"bill-bot/ent/split"
)

type Settlement struct {
	From   string
	To     string
	Amount int64
}

// ComputeSettlements returns the minimum set of transactions to settle all debts.
func ComputeSettlements(ctx context.Context, client *ent.Client, ledgerID int) ([]Settlement, error) {
	net, err := NetBalances(ctx, client, ledgerID)
	if err != nil {
		return nil, err
	}

	type userBalance struct {
		userID string
		amount int64
	}

	var creditors, debtors []userBalance
	for uid, bal := range net {
		if bal > 0 {
			creditors = append(creditors, userBalance{uid, bal})
		} else if bal < 0 {
			debtors = append(debtors, userBalance{uid, -bal})
		}
	}

	// Sort for deterministic output
	sort.Slice(creditors, func(i, j int) bool { return creditors[i].amount > creditors[j].amount })
	sort.Slice(debtors, func(i, j int) bool { return debtors[i].amount > debtors[j].amount })

	var settlements []Settlement
	i, j := 0, 0
	for i < len(creditors) && j < len(debtors) {
		c := &creditors[i]
		d := &debtors[j]
		amt := min(d.amount, c.amount)
		settlements = append(settlements, Settlement{
			From:   d.userID,
			To:     c.userID,
			Amount: amt,
		})
		c.amount -= amt
		d.amount -= amt
		if c.amount == 0 {
			i++
		}
		if d.amount == 0 {
			j++
		}
	}
	return settlements, nil
}

// NetBalances returns net balance per user for a ledger.
// Positive = owed money by others; Negative = owes money to others.
func NetBalances(ctx context.Context, client *ent.Client, ledgerID int) (map[string]int64, error) {
	net := map[string]int64{}

	expenses, err := client.Expense.Query().
		Where(
			expense.HasLedgerWith(ledger.ID(ledgerID)),
			expense.Deleted(false),
		).
		WithSplits().
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range expenses {
		net[e.PayerID] += e.Amount
		for _, s := range e.Edges.Splits {
			net[s.UserID] -= s.Amount
		}
	}

	payments, err := client.Payment.Query().
		Where(payment.HasLedgerWith(ledger.ID(ledgerID))).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range payments {
		net[p.FromUserID] += p.Amount
		net[p.ToUserID] -= p.Amount
	}

	return net, nil
}

// SettledUsers returns user IDs whose net balance is 0 and have participated in splits.
func SettledUsers(ctx context.Context, client *ent.Client, ledgerID int) ([]string, error) {
	net, err := NetBalances(ctx, client, ledgerID)
	if err != nil {
		return nil, err
	}

	// Find users who appear in splits (have participated)
	splits, err := client.Split.Query().
		Where(split.HasExpenseWith(
			expense.HasLedgerWith(ledger.ID(ledgerID)),
			expense.Deleted(false),
		)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	participated := map[string]bool{}
	for _, s := range splits {
		participated[s.UserID] = true
	}

	var settled []string
	for uid := range participated {
		if net[uid] == 0 {
			settled = append(settled, uid)
		}
	}
	return settled, nil
}
