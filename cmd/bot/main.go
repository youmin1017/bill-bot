package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"bill-bot/internal/bot"
	"bill-bot/internal/bot/commands"
	"bill-bot/internal/config"
	"bill-bot/internal/db"

	"github.com/samber/do/v2"
)

func main() {
	i := do.New()

	config.Package(i)
	db.Package(i)
	bot.Package(i)

	// Register command providers
	do.Provide(i, commands.NewPayCommand)
	do.Provide(i, commands.NewMemberCommand)
	do.Provide(i, commands.NewSettleCommand)
	do.Provide(i, commands.NewBalanceCommand)

	// Resolve the bot
	b := do.MustInvoke[*bot.Bot](i)

	// Register command handlers
	b.Register(do.MustInvoke[*commands.PayCommand](i))
	b.Register(do.MustInvoke[*commands.MemberCommand](i))
	b.Register(do.MustInvoke[*commands.SettleCommand](i))
	b.Register(do.MustInvoke[*commands.BalanceCommand](i))

	if err := b.Start(); err != nil {
		slog.Error("failed to start bot", "error", err)
		os.Exit(1)
	}
	defer b.Stop()

	slog.Info("Bill-bot is running. Press CTRL+C to exit.")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("Shutting down...")
}
