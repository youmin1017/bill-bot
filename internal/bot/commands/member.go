package commands

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"bill-bot/ent"
	"bill-bot/ent/ledger"
	"bill-bot/ent/ledgermember"
	"bill-bot/internal/bot"
	"bill-bot/internal/config"
	"bill-bot/internal/render"
	"bill-bot/internal/service"

	"github.com/bwmarrin/discordgo"
	"github.com/samber/do/v2"
)

// MemberCommand handles /member list|add|remove
type MemberCommand struct {
	cfg    *config.Config
	client *ent.Client
}

func NewMemberCommand(i do.Injector) (*MemberCommand, error) {
	return &MemberCommand{
		cfg:    do.MustInvoke[*config.Config](i),
		client: do.MustInvoke[*ent.Client](i),
	}, nil
}

func (c *MemberCommand) Command() *discordgo.ApplicationCommand {
	userOpts := func() []*discordgo.ApplicationCommandOption {
		opts := make([]*discordgo.ApplicationCommandOption, 5)
		for idx := range opts {
			opts[idx] = &discordgo.ApplicationCommandOption{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        fmt.Sprintf("user%d", idx+1),
				Description: fmt.Sprintf("成員 %d", idx+1),
				Required:    idx == 0,
			}
		}
		return opts
	}

	return &discordgo.ApplicationCommand{
		Name:        "member",
		Description: "管理帳本成員",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "列出所有成員",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "add",
				Description: "加入成員",
				Options:     userOpts(),
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "remove",
				Description: "移除成員",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "要移除的成員",
						Required:    true,
					},
				},
			},
		},
	}
}

func (c *MemberCommand) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	subCmd := i.ApplicationCommandData().Options[0]
	switch subCmd.Name {
	case "list":
		c.handleList(s, i, ctx, l)
	case "add":
		c.handleAdd(s, i, ctx, l, subCmd.Options)
	case "remove":
		c.handleRemove(s, i, ctx, l, subCmd.Options)
	}
}

func (c *MemberCommand) handleList(s *discordgo.Session, i *discordgo.InteractionCreate, ctx context.Context, l *ent.Ledger) {
	members, err := l.QueryMembers().Where(ledgermember.Active(true)).All(ctx)
	if err != nil {
		respondEphemeral(s, i, "無法取得成員清單")
		return
	}

	if len(members) == 0 {
		respondEphemeral(s, i, "目前尚無成員")
		return
	}

	names := make([]string, 0, len(members))
	for _, m := range members {
		names = append(names, m.UserName)
	}

	respond(s, i, fmt.Sprintf("成員（%d 人）：%s", len(members), strings.Join(names, "、")))
}

func (c *MemberCommand) handleAdd(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	ctx context.Context,
	l *ent.Ledger,
	opts []*discordgo.ApplicationCommandInteractionDataOption,
) {
	resolved := i.ApplicationCommandData().Resolved

	// 建立 option map 加速查找
	optMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(opts))
	for _, o := range opts {
		optMap[o.Name] = o
	}

	added := []string{}
	for idx := 1; idx <= 5; idx++ {
		opt, ok := optMap[fmt.Sprintf("user%d", idx)]
		if !ok {
			continue
		}

		uid := opt.UserValue(nil).ID
		uname := uid // fallback

		if resolved != nil {
			// 優先順序：Nick > GlobalName > Username
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
		}

		if err := service.UpsertMember(ctx, c.client, l.ID, uid, uname); err != nil {
			slog.Error("failed to upsert member", "user_id", uid, "error", err)
			continue
		}

		added = append(added, uname)

		allow := int64(discordgo.PermissionViewChannel | discordgo.PermissionSendMessages | discordgo.PermissionReadMessageHistory)
		if err := s.ChannelPermissionSet(i.ChannelID, uid, discordgo.PermissionOverwriteTypeMember, allow, 0); err != nil {
			slog.Warn("failed to set channel permission", "user_id", uid, "error", err)
		}
	}

	if len(added) == 0 {
		respondEphemeral(s, i, "未能新增任何成員")
		return
	}

	_ = render.RefreshPinnedSummary(ctx, c.client, l, s)
	respond(s, i, fmt.Sprintf("已新增成員：%s", strings.Join(added, "、")))
}

func (c *MemberCommand) handleRemove(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	ctx context.Context,
	l *ent.Ledger,
	opts []*discordgo.ApplicationCommandInteractionDataOption,
) {
	if len(opts) == 0 {
		respondEphemeral(s, i, "請指定要移除的成員")
		return
	}

	u := opts[0].UserValue(nil)
	uid := u.ID

	member, err := c.client.LedgerMember.Query().
		Where(
			ledgermember.UserID(uid),
			ledgermember.HasLedgerWith(ledger.ID(l.ID)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondEphemeral(s, i, "找不到該成員")
		} else {
			respondEphemeral(s, i, "資料庫錯誤")
		}
		return
	}

	if err := member.Update().SetActive(false).Exec(ctx); err != nil {
		respondEphemeral(s, i, "移除成員失敗")
		return
	}

	if err := s.ChannelPermissionDelete(i.ChannelID, uid); err != nil {
		slog.Error("member remove: failed to delete channel permission", "user_id", uid, "error", err)
	}

	_ = render.RefreshPinnedSummary(ctx, c.client, l, s)
	respond(s, i, fmt.Sprintf("已移除成員：%s", member.UserName))
}
