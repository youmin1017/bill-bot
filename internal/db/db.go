package db

import (
	"context"

	"bill-bot/ent"
	"bill-bot/ent/migrate"
	"bill-bot/internal/config"

	"github.com/samber/do/v2"

	_ "github.com/mattn/go-sqlite3"
)

var Package = do.Package(
	do.Lazy(NewEntClient),
)

func NewEntClient(i do.Injector) (*ent.Client, error) {
	cfg := do.MustInvoke[*config.Config](i)

	client, err := ent.Open("sqlite3", cfg.DBPath+"?_fk=1&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}

	if err := client.Schema.Create(context.Background(), migrate.WithDropColumn(true)); err != nil {
		return nil, err
	}

	return client, nil
}
