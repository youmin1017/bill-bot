package db

import (
	"context"
	"database/sql"
	"fmt"

	"bill-bot/ent"
	"bill-bot/ent/migrate"
	"bill-bot/internal/config"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/samber/do/v2"

	_ "modernc.org/sqlite"
)

var Package = do.Package(
	do.Lazy(NewEntClient),
)

func NewEntClient(i do.Injector) (*ent.Client, error) {
	cfg := do.MustInvoke[*config.Config](i)

	sqlDB, err := sql.Open("sqlite", cfg.DBPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("failed opening sqlite: %w", err)
	}

	drv := entsql.OpenDB(dialect.SQLite, sqlDB)
	client := ent.NewClient(ent.Driver(drv))

	if err := client.Schema.Create(context.Background(), migrate.WithDropColumn(true)); err != nil {
		return nil, err
	}

	return client, nil
}
