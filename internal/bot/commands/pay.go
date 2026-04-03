package commands

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"bill-bot/ent"
	"bill-bot/ent/expense"
	"bill-bot/ent/payment"
	"bill-bot/internal/config"
	"bill-bot/internal/render"
	"bill-bot/internal/service"

	"github.com/bwmarrin/discordgo"
	"github.com/samber/do/v2"
)

type PayCommand struct {
	cfg    *config.Config
	client *ent.Client
}

func NewPayCommand(i do.Injector) (*PayCommand, error) {
	return &PayCommand{
		cfg:    do.MustInvoke[*config.Config](i),
		client: do.MustInvoke[*ent.Client](i),
	}, nil
}

func (c *PayCommand) Command() *discordgo.ApplicationCommand {
	userOpts := func(required bool) []*discordgo.ApplicationCommandOption {
		opts := make([]*discordgo.ApplicationCommandOption, 5)
		for idx := range opts {
			opts[idx] = &discordgo.ApplicationCommandOption{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        fmt.Sprintf("user%d", idx+1),
				Description: fmt.Sprintf("分攤成員 %d", idx+1),
				Required:    required && idx == 0,
			}
		}
		return opts
	}

	splitOpts := []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "amount",
			Description: "金額（例：120 或 120.50）",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "description",
			Description: "帳目說明",
			Required:    true,
		},
	}
	splitOpts = append(splitOpts, userOpts(false)...)

	forOpts := []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "amount",
			Description: "金額（例：120 或 120.50）",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "description",
			Description: "帳目說明",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "user",
			Description: "受益人",
			Required:    true,
		},
	}

	removeOpts := []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "id",
			Description: "帳目編號（建立時回報的 #ID）",
			Required:    true,
		},
	}

	return &discordgo.ApplicationCommand{
		Name:        "pay",
		Description: "新增或刪除帳目",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "split",
				Description: "大家平分一筆費用",
				Options:     splitOpts,
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "for",
				Description: "代墊給某人的費用",
				Options:     forOpts,
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "remove",
				Description: "刪除一筆帳目",
				Options:     removeOpts,
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "查看帳目記錄",
			},
		},
	}
}

func parseAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("無效金額：%s", s)
	}
	if f <= 0 {
		return 0, fmt.Errorf("金額必須大於 0")
	}
	return int64(math.Round(f * 100)), nil
}

func (c *PayCommand) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()

	l, err := service.GetLedger(ctx, c.client, i.ChannelID)
	if err != nil {
		if ent.IsNotFound(err) {
			respondEphemeral(s, i, "此頻道尚未初始化為帳本，請先執行 /init")
		} else {
			respondEphemeral(s, i, "資料庫錯誤")
		}
		return
	}

	subCmd := i.ApplicationCommandData().Options[0]
	switch subCmd.Name {
	case "split":
		c.handleSplit(s, i, ctx, l, subCmd.Options)
	case "for":
		c.handleFor(s, i, ctx, l, subCmd.Options)
	case "remove":
		c.handleRemove(s, i, ctx, l, subCmd.Options)
	case "list":
		c.handleList(s, i, ctx, l)
	}
}

func optionMap(opts []*discordgo.ApplicationCommandInteractionDataOption) map[string]*discordgo.ApplicationCommandInteractionDataOption {
	m := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opts))
	for _, o := range opts {
		m[o.Name] = o
	}
	return m
}

// resolveDisplayName 依優先順序取得顯示名稱：Nick > GlobalName > Username > fallback(uid)
func resolveDisplayName(uid string, resolved *discordgo.ApplicationCommandInteractionDataResolved) string {
	uname := uid
	if resolved == nil {
		return uname
	}
	if u, ok := resolved.Users[uid]; ok {
		if u.GlobalName != "" {
			uname = u.GlobalName
		} else {
			uname = u.Username
		}
	}
	if resolved.Members != nil {
		if m, ok := resolved.Members[uid]; ok && m.Nick != "" {
			uname = m.Nick
		}
	}
	return uname
}

// memberDisplayName 從 *discordgo.Member 取得顯示名稱：Nick > GlobalName > Username
func memberDisplayName(m *discordgo.Member) string {
	if m.Nick != "" {
		return m.Nick
	}
	if m.User != nil {
		if m.User.GlobalName != "" {
			return m.User.GlobalName
		}
		return m.User.Username
	}
	return ""
}

func (c *PayCommand) handleSplit(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	ctx context.Context,
	l *ent.Ledger,
	opts []*discordgo.ApplicationCommandInteractionDataOption,
) {
	om := optionMap(opts)

	amtStr := om["amount"].StringValue()
	amtCents, err := parseAmount(amtStr)
	if err != nil {
		respondEphemeral(s, i, err.Error())
		return
	}

	desc := om["description"].StringValue()

	resolved := i.ApplicationCommandData().Resolved
	type splitUser struct {
		id   string
		name string
	}
	var users []splitUser

	for idx := 1; idx <= 5; idx++ {
		opt, ok := om[fmt.Sprintf("user%d", idx)]
		if !ok {
			continue
		}
		uid := opt.UserValue(nil).ID
		users = append(users, splitUser{id: uid, name: resolveDisplayName(uid, resolved)})
	}

	// 未指定成員時，使用帳本全部活躍成員
	if len(users) == 0 {
		members, err := l.QueryMembers().All(ctx)
		if err != nil {
			respondEphemeral(s, i, "無法取得成員清單")
			return
		}
		if len(members) == 0 {
			respondEphemeral(s, i, "帳本尚無成員，請先使用 /member add 新增成員")
			return
		}
		for _, m := range members {
			if m.Active {
				users = append(users, splitUser{id: m.UserID, name: m.UserName})
			}
		}
	}

	// 取得付款人名稱
	payerID := i.Member.User.ID
	payerName := memberDisplayName(i.Member)
	if payerName == "" {
		payerName = payerID
	}

	// 確保付款人在分攤清單中
	payerInList := false
	for _, u := range users {
		if u.id == payerID {
			payerInList = true
			break
		}
	}
	if !payerInList {
		users = append(users, splitUser{id: payerID, name: payerName})
	}

	// 平均分攤，餘數逐一加到前幾位
	n := int64(len(users))
	base := amtCents / n
	remainder := amtCents % n

	exp, err := c.client.Expense.Create().
		SetLedgerID(l.ID).
		SetAmount(amtCents).
		SetDescription(desc).
		SetPayerID(payerID).
		SetType("split").
		Save(ctx)
	if err != nil {
		respondEphemeral(s, i, "建立帳目失敗")
		return
	}

	for idx, u := range users {
		share := base
		if int64(idx) < remainder {
			share++
		}
		if err := c.client.Split.Create().
			SetExpenseID(exp.ID).
			SetUserID(u.id).
			SetAmount(share).
			Exec(ctx); err != nil {
			respondEphemeral(s, i, "建立分帳明細失敗")
			return
		}
		_ = service.UpsertMember(ctx, c.client, l.ID, u.id, u.name)
	}

	_ = render.RefreshPinnedSummary(ctx, c.client, l, s)
	respond(s, i, fmt.Sprintf("#%d 已記錄：%s 代墊 %s（%s），%d 人平分",
		exp.ID, payerName, formatAmount(amtCents), desc, len(users)))
}

func (c *PayCommand) handleFor(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	ctx context.Context,
	l *ent.Ledger,
	opts []*discordgo.ApplicationCommandInteractionDataOption,
) {
	om := optionMap(opts)

	amtStr := om["amount"].StringValue()
	amtCents, err := parseAmount(amtStr)
	if err != nil {
		respondEphemeral(s, i, err.Error())
		return
	}

	desc := om["description"].StringValue()

	resolved := i.ApplicationCommandData().Resolved
	beneficiaryID := om["user"].UserValue(nil).ID
	beneficiaryName := resolveDisplayName(beneficiaryID, resolved)

	// 取得付款人名稱
	payerID := i.Member.User.ID
	payerName := memberDisplayName(i.Member)
	if payerName == "" {
		payerName = payerID
	}

	exp, err := c.client.Expense.Create().
		SetLedgerID(l.ID).
		SetAmount(amtCents).
		SetDescription(desc).
		SetPayerID(payerID).
		SetType("for").
		Save(ctx)
	if err != nil {
		respondEphemeral(s, i, "建立帳目失敗")
		return
	}

	if err := c.client.Split.Create().
		SetExpenseID(exp.ID).
		SetUserID(beneficiaryID).
		SetAmount(amtCents).
		Exec(ctx); err != nil {
		respondEphemeral(s, i, "建立分帳明細失敗")
		return
	}

	_ = service.UpsertMember(ctx, c.client, l.ID, payerID, payerName)
	_ = service.UpsertMember(ctx, c.client, l.ID, beneficiaryID, beneficiaryName)

	_ = render.RefreshPinnedSummary(ctx, c.client, l, s)
	respond(s, i, fmt.Sprintf("#%d 已記錄：%s 代墊 %s 給 %s",
		exp.ID, payerName, formatAmount(amtCents), beneficiaryName))
}

func (c *PayCommand) handleRemove(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	ctx context.Context,
	l *ent.Ledger,
	opts []*discordgo.ApplicationCommandInteractionDataOption,
) {
	om := optionMap(opts)
	expenseID := int(om["id"].IntValue())

	exp, err := l.QueryExpenses().
		Where(expense.ID(expenseID), expense.Deleted(false)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondEphemeral(s, i, fmt.Sprintf("找不到帳目 #%d", expenseID))
		} else {
			respondEphemeral(s, i, "資料庫錯誤")
		}
		return
	}

	if err := c.client.Expense.UpdateOne(exp).SetDeleted(true).Exec(ctx); err != nil {
		respondEphemeral(s, i, "刪除失敗")
		return
	}

	_ = render.RefreshPinnedSummary(ctx, c.client, l, s)
	respond(s, i, fmt.Sprintf("#%d 已刪除（%s）", exp.ID, exp.Description))
}

func (c *PayCommand) handleList(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	ctx context.Context,
	l *ent.Ledger,
) {
	members, err := l.QueryMembers().All(ctx)
	if err != nil {
		respondEphemeral(s, i, "無法取得成員清單")
		return
	}
	memberName := make(map[string]string, len(members))
	for _, m := range members {
		memberName[m.UserID] = m.UserName
	}

	expenses, err := l.QueryExpenses().
		Where(expense.Deleted(false)).
		WithSplits().
		Order(expense.ByCreatedAt()).
		All(ctx)
	if err != nil {
		respondEphemeral(s, i, "無法取得帳目記錄")
		return
	}

	payments, err := l.QueryPayments().
		Order(payment.ByCreatedAt()).
		All(ctx)
	if err != nil {
		respondEphemeral(s, i, "無法取得還款記錄")
		return
	}

	if len(expenses) == 0 && len(payments) == 0 {
		respond(s, i, "目前尚無任何帳目記錄")
		return
	}

	type entry struct {
		createdAt time.Time
		line      string
	}
	var entries []entry

	for _, e := range expenses {
		var detail string
		if e.Type == "split" {
			detail = fmt.Sprintf("平分 %d 人", len(e.Edges.Splits))
		} else if len(e.Edges.Splits) > 0 {
			uid := e.Edges.Splits[0].UserID
			name := memberName[uid]
			if name == "" {
				name = uid
			}
			detail = "給 " + name
		}
		payerName := memberName[e.PayerID]
		if payerName == "" {
			payerName = e.PayerID
		}
		line := fmt.Sprintf("`%s` #%d %s 代墊 **%s**（%s）%s",
			e.CreatedAt.Format("01/02"),
			e.ID,
			payerName,
			formatAmount(e.Amount),
			e.Description,
			detail,
		)
		entries = append(entries, entry{e.CreatedAt, line})
	}

	for _, p := range payments {
		note := ""
		if p.Note != "" {
			note = "（" + p.Note + "）"
		}
		line := fmt.Sprintf("`%s` %s 還款 **%s** 給 %s%s",
			p.CreatedAt.Format("01/02"),
			p.FromUserName,
			formatAmount(p.Amount),
			p.ToUserName,
			note,
		)
		entries = append(entries, entry{p.CreatedAt, line})
	}

	sort.Slice(entries, func(a, b int) bool {
		return entries[a].createdAt.Before(entries[b].createdAt)
	})

	total := len(entries)
	const maxBodyLen = 1700

	lines := make([]string, len(entries))
	for idx, e := range entries {
		lines[idx] = e.line + "\n"
	}

	bodyLen := 0
	startIdx := len(lines)
	for startIdx > 0 && bodyLen+len(lines[startIdx-1]) <= maxBodyLen {
		startIdx--
		bodyLen += len(lines[startIdx])
	}

	var header string
	if startIdx == 0 {
		header = fmt.Sprintf("**帳目記錄**（共 %d 筆）\n━━━━━━━━━━━━━━━━━━━━\n", total)
	} else {
		shown := total - startIdx
		header = fmt.Sprintf("**帳目記錄**（最近 %d／共 %d 筆）\n━━━━━━━━━━━━━━━━━━━━\n", shown, total)
	}

	respond(s, i, header+strings.Join(lines[startIdx:], ""))
}
