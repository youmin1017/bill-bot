package render

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bill-bot/ent"
	"bill-bot/ent/ledgermember"
	"bill-bot/internal/service"

	"github.com/bwmarrin/discordgo"
)

func formatAmount(cents int64) string {
	if cents < 0 {
		return "-" + formatAmount(-cents)
	}
	whole := cents / 100
	frac := cents % 100
	return fmt.Sprintf("NT$%s.%02d", formatInt(whole), frac)
}

func formatInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	result := ""
	for idx, c := range s {
		if idx > 0 && (len(s)-idx)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

// BuildSummary builds the pinned summary message content.
func BuildSummary(ctx context.Context, client *ent.Client, l *ent.Ledger, s *discordgo.Session) (string, error) {
	// Get channel name for title
	channelName := l.ChannelID
	if s != nil {
		if ch, err := s.Channel(l.ChannelID); err == nil {
			channelName = ch.Name
		}
	}

	// Get active members via ledger edge
	members, err := l.QueryMembers().Where(ledgermember.Active(true)).All(ctx)
	if err != nil {
		return "", err
	}

	memberNames := make([]string, 0, len(members))
	for _, m := range members {
		memberNames = append(memberNames, m.UserName)
	}

	// Get total expense
	expenses, err := l.QueryExpenses().All(ctx)
	if err != nil {
		return "", err
	}
	var total int64
	for _, e := range expenses {
		if !e.Deleted {
			total += e.Amount
		}
	}

	// Get settlements
	settlements, err := service.ComputeSettlements(ctx, client, l.ID)
	if err != nil {
		return "", err
	}

	// Build user name map from members
	nameMap := map[string]string{}
	for _, m := range members {
		nameMap[m.UserID] = m.UserName
	}

	// Get settled users
	settledUsers, err := service.SettledUsers(ctx, client, l.ID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%s** 帳本摘要\n", channelName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")

	if len(memberNames) > 0 {
		sb.WriteString(fmt.Sprintf("成員：%s\n", strings.Join(memberNames, "、")))
	} else {
		sb.WriteString("成員：（尚無成員）\n")
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("總支出：%s\n", formatAmount(total)))

	if len(settlements) > 0 {
		sb.WriteString("\n結算方案（最少轉帳）：\n")
		resolve := func(uid string) string {
			if name := nameMap[uid]; name != "" {
				return name
			}
			return "<@" + uid + ">"
		}
		for _, st := range settlements {
			sb.WriteString(fmt.Sprintf("  %s 　→　%s　%s\n", resolve(st.From), resolve(st.To), formatAmount(st.Amount)))
		}
	}

	if len(settledUsers) > 0 {
		settledNames := make([]string, 0, len(settledUsers))
		for _, uid := range settledUsers {
			n := nameMap[uid]
			if n == "" {
				n = uid
			}
			settledNames = append(settledNames, n)
		}
		sb.WriteString(fmt.Sprintf("\n已結清成員：%s\n", strings.Join(settledNames, "、")))
	}

	sb.WriteString(fmt.Sprintf("\n最後更新：%s\n", time.Now().Format("2006/01/02 15:04")))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString("輸入 /add split|for 新增帳目　/member list 查看成員")

	return sb.String(), nil
}

// RefreshPinnedSummary updates the pinned summary message for a ledger.
func RefreshPinnedSummary(ctx context.Context, client *ent.Client, l *ent.Ledger, s *discordgo.Session) error {
	content, err := BuildSummary(ctx, client, l, s)
	if err != nil {
		return err
	}

	if l.PinnedMessageID != "" {
		_, err = s.ChannelMessageEdit(l.ChannelID, l.PinnedMessageID, content)
		if err == nil {
			return nil
		}
		// If edit fails (message deleted), fall through to create new one
	}

	msg, err := s.ChannelMessageSend(l.ChannelID, content)
	if err != nil {
		return err
	}
	if err := s.ChannelMessagePin(l.ChannelID, msg.ID); err != nil {
		return err
	}
	return client.Ledger.UpdateOne(l).SetPinnedMessageID(msg.ID).Exec(ctx)
}
