package commands

import (
	"context"
	"fmt"
	"strings"

	"bill-bot/ent"
	"bill-bot/internal/bot"
	"bill-bot/internal/config"
	"bill-bot/internal/render"
	"bill-bot/internal/service"

	"github.com/bwmarrin/discordgo"
	"github.com/samber/do/v2"
)

// SettleCommand handles /settle
type SettleCommand struct {
	cfg    *config.Config
	client *ent.Client
}

func NewSettleCommand(i do.Injector) (*SettleCommand, error) {
	return &SettleCommand{
		cfg:    do.MustInvoke[*config.Config](i),
		client: do.MustInvoke[*ent.Client](i),
	}, nil
}

func (c *SettleCommand) Command() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "settle",
		Description: "記錄還款",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "to",
				Description: "還款給誰",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "amount",
				Description: "金額（例：120 或 120.50）",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "note",
				Description: "備注",
				Required:    false,
			},
		},
	}
}

func (c *SettleCommand) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := bot.CategoryGuard(s, c.cfg, i.ChannelID); err != nil {
		respondEphemeral(s, i, ""+err.Error())
		return
	}

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

	om := optionMap(i.ApplicationCommandData().Options)

	toUser := om["to"].UserValue(nil)
	toID := toUser.ID
	toName := toUser.Username

	resolved := i.ApplicationCommandData().Resolved
	if resolved != nil && resolved.Members != nil {
		if m, ok := resolved.Members[toID]; ok && m.Nick != "" {
			toName = m.Nick
		}
	}

	amtStr := om["amount"].StringValue()
	amtCents, err := parseAmount(amtStr)
	if err != nil {
		respondEphemeral(s, i, ""+err.Error())
		return
	}

	fromID := i.Member.User.ID
	fromName := i.Member.User.Username
	if i.Member.Nick != "" {
		fromName = i.Member.Nick
	}

	note := ""
	if noteOpt, ok := om["note"]; ok {
		note = noteOpt.StringValue()
	}

	_, err = c.client.Payment.Create().
		SetLedgerID(l.ID).
		SetFromUserID(fromID).
		SetFromUserName(fromName).
		SetToUserID(toID).
		SetToUserName(toName).
		SetAmount(amtCents).
		SetNote(note).
		Save(ctx)
	if err != nil {
		respondEphemeral(s, i, "記錄還款失敗")
		return
	}

	// Refresh pinned summary
	_ = render.RefreshPinnedSummary(ctx, c.client, l, s)

	msg := fmt.Sprintf("已記錄：%s 還款 %s 給 %s", fromName, formatAmount(amtCents), toName)
	if note != "" {
		msg += fmt.Sprintf("（%s）", note)
	}
	respond(s, i, msg)
}

// BalanceCommand handles /balance
type BalanceCommand struct {
	cfg    *config.Config
	client *ent.Client
}

func NewBalanceCommand(i do.Injector) (*BalanceCommand, error) {
	return &BalanceCommand{
		cfg:    do.MustInvoke[*config.Config](i),
		client: do.MustInvoke[*ent.Client](i),
	}, nil
}

func (c *BalanceCommand) Command() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "balance",
		Description: "查看目前結算情況",
	}
}

func (c *BalanceCommand) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := bot.CategoryGuard(s, c.cfg, i.ChannelID); err != nil {
		respondEphemeral(s, i, ""+err.Error())
		return
	}

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

	settlements, err := service.ComputeSettlements(ctx, c.client, l.ID)
	if err != nil {
		respondEphemeral(s, i, "計算結算失敗")
		return
	}

	// Get name map
	members, err := l.QueryMembers().All(ctx)
	if err != nil {
		respondEphemeral(s, i, "無法取得成員清單")
		return
	}
	nameMap := map[string]string{}
	for _, m := range members {
		nameMap[m.UserID] = m.UserName
	}

	if len(settlements) == 0 {
		respond(s, i, "所有帳目已結清！")
		return
	}

	resolveName := func(uid string) string {
		if name := nameMap[uid]; name != "" {
			return name
		}
		return "<@" + uid + ">"
	}

	var sb strings.Builder
	sb.WriteString("**結算方案（最少轉帳）**\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
	for _, st := range settlements {
		fmt.Fprintf(&sb, "%s 　→　%s　**%s**\n",
			resolveName(st.From), resolveName(st.To), formatAmount(st.Amount))
	}

	respond(s, i, sb.String())
}
