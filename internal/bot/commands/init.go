package commands

import (
	"context"
	"log/slog"

	"bill-bot/ent"
	"bill-bot/internal/render"
	"bill-bot/internal/service"

	"github.com/bwmarrin/discordgo"
	"github.com/samber/do/v2"
)

// InitCommand handles /init
type InitCommand struct {
	client *ent.Client
}

func NewInitCommand(i do.Injector) (*InitCommand, error) {
	return &InitCommand{
		client: do.MustInvoke[*ent.Client](i),
	}, nil
}

func (c *InitCommand) Command() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "init",
		Description: "將此頻道初始化為帳本",
	}
}

func (c *InitCommand) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()

	// Skip if ledger already exists
	if _, err := service.GetLedger(ctx, c.client, i.ChannelID); err == nil {
		respondEphemeral(s, i, "此頻道已是帳本，無需重複初始化")
		return
	}

	ch, err := s.Channel(i.ChannelID)
	if err != nil {
		respondEphemeral(s, i, "無法取得頻道資訊")
		return
	}

	l, err := service.CreateLedger(ctx, c.client, i.ChannelID, i.GuildID, ch.ParentID)
	if err != nil {
		slog.Error("INIT: failed to create ledger", "channel_id", i.ChannelID, "error", err)
		respondEphemeral(s, i, "建立帳本失敗")
		return
	}

	if err := render.RefreshPinnedSummary(ctx, c.client, l, s); err != nil {
		slog.Error("INIT: failed to pin summary", "error", err)
	}

	respond(s, i, "帳本已建立！在頻道加入成員即可自動同步至帳本。")
}
