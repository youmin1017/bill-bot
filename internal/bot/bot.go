package bot

import (
	"context"
	"log/slog"

	"bill-bot/ent"
	"bill-bot/internal/config"
	"bill-bot/internal/render"
	"bill-bot/internal/service"

	"github.com/bwmarrin/discordgo"
	"github.com/samber/do/v2"
	"github.com/samber/lo"
)

// Handler is a slash command handler.
type Handler interface {
	Command() *discordgo.ApplicationCommand
	Handle(s *discordgo.Session, i *discordgo.InteractionCreate)
}

// Bot wraps a discordgo session and manages command registration.
type Bot struct {
	Session  *discordgo.Session
	cfg      *config.Config
	client   *ent.Client
	handlers map[string]Handler
}

var Package = do.Package(
	do.Lazy(NewBot),
)

func NewBot(i do.Injector) (*Bot, error) {
	cfg := do.MustInvoke[*config.Config](i)
	client := do.MustInvoke[*ent.Client](i)

	s, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		return nil, err
	}

	return &Bot{
		Session:  s,
		cfg:      cfg,
		client:   client,
		handlers: make(map[string]Handler),
	}, nil
}

// Register adds a command handler.
func (b *Bot) Register(h Handler) {
	b.handlers[h.Command().Name] = h
}

// Start opens the websocket connection, registers commands, and listens.
func (b *Bot) Start() error {
	b.Session.AddHandler(b.onInteraction)
	b.Session.AddHandler(b.onChannelCreate)
	b.Session.AddHandler(b.onChannelDelete)

	if err := b.Session.Open(); err != nil {
		return err
	}

	cmds := lo.MapToSlice(b.handlers, func(_ string, h Handler) *discordgo.ApplicationCommand {
		return h.Command()
	})

	if _, err := b.Session.ApplicationCommandBulkOverwrite(b.Session.State.User.ID, "", cmds); err != nil {
		slog.Error("failed to bulk register commands", "error", err)
	}

	return nil
}

// Stop closes the session.
func (b *Bot) Stop() {
	_ = b.Session.Close()
}

func (b *Bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	name := i.ApplicationCommandData().Name
	h, ok := b.handlers[name]
	if !ok {
		return
	}
	h.Handle(s, i)
}

func (b *Bot) onChannelCreate(s *discordgo.Session, e *discordgo.ChannelCreate) {
	if e.Type != discordgo.ChannelTypeGuildText {
		return
	}
	if e.ParentID == "" {
		return
	}
	parent, err := s.Channel(e.ParentID)
	if err != nil || parent.Name != b.cfg.AccountCategory {
		return
	}
	ctx := context.Background()
	// Skip if ledger already exists (race condition guard)
	if _, err := service.GetLedger(ctx, b.client, e.ID); err == nil {
		return
	}
	l, err := service.CreateLedger(ctx, b.client, e.ID, e.GuildID, e.ParentID)
	if err != nil {
		slog.Error("CHANNEL_CREATE: failed to create ledger", "channel_id", e.ID, "error", err)
		return
	}
	if err := render.RefreshPinnedSummary(ctx, b.client, l, s); err != nil {
		slog.Error("CHANNEL_CREATE: failed to pin summary", "error", err)
	}
	_, _ = s.ChannelMessageSend(e.ID, "帳本已自動建立！在頻道加入成員即可自動同步至帳本。")
}

func (b *Bot) onChannelDelete(s *discordgo.Session, e *discordgo.ChannelDelete) {
	ctx := context.Background()
	l, err := service.GetLedger(ctx, b.client, e.ID)
	if err != nil {
		return
	}
	if err := b.client.Ledger.UpdateOne(l).SetActive(false).Exec(ctx); err != nil {
		slog.Error("CHANNEL_DELETE: failed to mark ledger inactive", "channel_id", e.ID, "error", err)
	}
}
