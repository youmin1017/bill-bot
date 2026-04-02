# Claude Code Prompt: bill-bot Discord Split Bot

## Project Overview

Build a Discord split-bill bot in Go named `bill-bot`.  
The bot can be added to multiple Discord Servers and operates **only inside channels that belong to a Category named `記帳`** (configurable).  
Each text channel inside that category acts as an independent ledger (account book).

---

## Tech Stack

- **Language**: Go 1.22+
- **Discord**: `github.com/bwmarrin/discordgo`
- **ORM / Schema**: `entgo.io/ent`
- **Database**: SQLite (`modernc.org/sqlite` — pure Go, no CGO)
- **DI**: `github.com/samber/do/v2`
- **Config**: `github.com/spf13/viper` + embedded default `config.yaml`
- **Module name**: `bill-bot`

---

## Project Structure

```
bill-bot/
├── cmd/bot/main.go
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   └── config.yaml          # embedded default config
│   ├── db/
│   │   └── db.go                # ent client + SQLite, DI provider
│   ├── bot/
│   │   ├── app.go               # bot.App: session lifecycle, GuildCreate handler
│   │   ├── middleware.go        # category guard middleware
│   │   ├── register.go          # register all slash commands on startup
│   │   └── commands/
│   │       ├── add.go           # /add
│   │       ├── balance.go       # /balance
│   │       ├── pay.go           # /pay
│   │       ├── history.go       # /history
│   │       ├── settle.go        # /settle
│   │       ├── init.go          # /init
│   │       └── member.go        # /member list
│   ├── service/
│   │   ├── ledger.go
│   │   ├── expense.go
│   │   ├── member.go            # member upsert / list helpers
│   │   └── balance.go           # minimum settlement algorithm
│   └── render/
│       └── summary.go           # pinned message formatter
└── ent/
    └── schema/
        ├── ledger.go
        ├── ledger_member.go     # NEW
        ├── expense.go
        ├── split.go
        └── payment.go
```

---

## Config Pattern

Follow this exact pattern for `internal/config/config.go`:

```go
package config

import (
    "bytes"
    _ "embed"
    "strings"
    "github.com/samber/do/v2"
    "github.com/spf13/viper"
)

type Config struct {
    BotToken        string `mapstructure:"botToken"`
    AccountCategory string `mapstructure:"accountCategory"` // default: "記帳"
    DBPath          string `mapstructure:"dbPath"`           // default: "bill-bot.db"
}

var Package = do.Package(
    do.Lazy(NewConfig),
)

//go:embed config.yaml
var defaultConfig []byte

func NewConfig(i do.Injector) (*Config, error) {
    // ... same viper pattern as provided
}
```

`config.yaml` defaults:

```yaml
botToken: ""
accountCategory: "記帳"
dbPath: "bill-bot.db"
```

---

## Main Pattern

```go
package main

import (
    "bill-bot/internal/bot"
    "bill-bot/internal/config"
    "bill-bot/internal/db"
    "github.com/samber/do/v2"
)

func main() {
    i := do.New()
    config.Package(i)
    db.Package(i)
    bot.Package(i)
    do.MustInvoke[*bot.App](i)
    select {} // block forever, bot runs in goroutines
}
```

---

## DI Package Pattern

Every internal package must expose a `Package` var using `do.Package(do.Lazy(...))`.  
Example for `internal/db/db.go`:

```go
var Package = do.Package(
    do.Lazy(NewEntClient),
)

func NewEntClient(i do.Injector) (*ent.Client, error) {
    cfg := do.MustInvoke[*config.Config](i)
    // open SQLite with ent, run AutoMigrate
}
```

`bot.Package` should register all command providers via DI and expose `bot.App`.

---

## Ent Schema

### `Ledger`

| Field               | Type      | Notes                       |
| ------------------- | --------- | --------------------------- |
| `channel_id`        | string    | unique, immutable           |
| `guild_id`          | string    | immutable                   |
| `category_id`       | string    | optional                    |
| `pinned_message_id` | string    | optional, updated after pin |
| `active`            | bool      | default true                |
| `created_at`        | time.Time | immutable                   |

Edges: `→ expenses []Expense`, `→ payments []Payment`, `→ members []LedgerMember`  
Index: `(guild_id)`, `(guild_id, channel_id)`

### `LedgerMember` _(NEW)_

Represents an explicit member of a ledger. This is the source of truth for "who is in this account book".

| Field       | Type      | Notes                            |
| ----------- | --------- | -------------------------------- |
| `user_id`   | string    | Discord user ID                  |
| `user_name` | string    | snapshot, updated on each upsert |
| `active`    | bool      | default true; false = removed    |
| `joined_at` | time.Time | immutable                        |

Edges: `← ledger Ledger`  
Index: `(ledger_id, user_id)` unique

### `Expense`

| Field         | Type      | Notes                         |
| ------------- | --------- | ----------------------------- |
| `amount`      | int64     | in cents, e.g. NT$120 → 12000 |
| `currency`    | string    | default "TWD"                 |
| `description` | string    |                               |
| `payer_id`    | string    | Discord user ID               |
| `payer_name`  | string    | snapshot at creation time     |
| `type`        | enum      | `"split"` or `"for"`          |
| `deleted`     | bool      | soft delete, default false    |
| `created_at`  | time.Time | immutable                     |

Edges: `← ledger Ledger`, `→ splits []Split`

### `Split`

| Field       | Type   | Notes         |
| ----------- | ------ | ------------- |
| `user_id`   | string |               |
| `user_name` | string | snapshot      |
| `amount`    | int64  | in cents      |
| `settled`   | bool   | default false |

Edges: `← expense Expense`

### `Payment`

| Field            | Type      | Notes     |
| ---------------- | --------- | --------- |
| `from_user_id`   | string    |           |
| `from_user_name` | string    | snapshot  |
| `to_user_id`     | string    |           |
| `to_user_name`   | string    | snapshot  |
| `amount`         | int64     | in cents  |
| `note`           | string    | optional  |
| `created_at`     | time.Time | immutable |

Edges: `← ledger Ledger`

---

## Slash Commands

All commands are Discord **Application Commands (Slash Commands)** using `discordgo`.  
Every command handler must first pass a **category guard**: check that the channel's `ParentID` belongs to a category whose name matches `Config.AccountCategory`. If not, respond ephemerally with an error message.

### `/init [users...]`

Initialize the current channel as a ledger.

- Create a `Ledger` record for `(guild_id, channel_id)`
- Automatically add the command invoker to `LedgerMember`
- If `users` are mentioned, add each of them to `LedgerMember` as well
- Send and pin a summary message, save `pinned_message_id`
- Respond: "✅ 此頻道已初始化為分帳帳本"
- If the ledger already exists (auto-initialized by `CHANNEL_CREATE`), respond ephemerally: "⚠️ 此頻道已經是帳本，無需重複初始化"

### `/member list`

List all active members in this ledger. Respond ephemerally.

```
👥 帳本成員（共 3 人）：
  • Alice（加入：2025/03/01）
  • Bob（加入：2025/03/01）
  • Carol（加入：2025/03/15）
```

> 成員新增／移除請直接在 Discord 頻道設定中管理，Bot 會自動同步。

### `/add split <amount> <description> [users...]`

Payer = command invoker. Split the amount equally among mentioned users (+ payer themselves).

- `amount` is an integer in major currency unit (e.g. 120 = NT$120), store as `amount * 100`
- If no users are mentioned → split equally among **all `LedgerMember` with `active = true`** (including payer)
- If users are mentioned → split among those users + payer; **automatically upsert each mentioned user into `LedgerMember`** if not already present
- The payer is also **automatically upserted** into `LedgerMember` if not already present
- Create `Expense{type:"split"}` + one `Split` per person
- After saving, refresh the pinned summary message

### `/add for <amount> <description> <user>`

Payer = command invoker. The full amount is owed by the single mentioned user.

- Create `Expense{type:"for"}` + one `Split{user: mentioned, amount: full}`
- **Automatically upsert** the payer and the mentioned user into `LedgerMember` if not already present
- After saving, refresh the pinned summary message

### `/balance`

Show net balance for every member in this ledger channel.  
Also show the **minimum settlement plan** (see service/balance.go).  
Respond ephemerally.

### `/pay <user> <amount>`

Record a manual repayment from invoker → mentioned user.  
Create a `Payment` record. Refresh pinned summary.

### `/history [limit]`

Show the last N expenses (default 10) in this channel. Respond ephemerally.

Each expense is displayed in the following format:

```
#12　2025/03/31　Alice　NT$300.00　晚餐
　　 👥 Alice、Bob、Carol　各 NT$100.00

#11　2025/03/28　Bob　NT$120.00　飲料
　　 👥 Bob、Alice　各 NT$60.00

#10　2025/03/25　Alice　NT$500.00　Carol 的計程車
　　 👥 Carol　NT$500.00（for）
```

- Show payer, amount, description, and the list of participants with their individual share
- Mark `for` type expenses explicitly so it's clear why only one person is listed
- Participants are taken from the `Split` records of that expense (snapshot at creation time), not the current member list

````

### `/settle`

Mark all splits in this ledger as settled. Requires confirmation (use a Discord component button: "確認結清 / 取消").

---

## Service: Member Upsert (`service/member.go`)

Provide a reusable helper used by `/add` commands to keep `LedgerMember` in sync:

```go
// UpsertMember ensures a user exists as an active LedgerMember.
// If the record exists but active=false, it is reactivated and user_name updated.
// If the record does not exist, it is created.
func UpsertMember(ctx context.Context, client *ent.Client, ledgerID int, userID, userName string) error
````

This must be called inside the same database transaction as the `Expense` creation wherever relevant.

---

## Service: Minimum Settlement Algorithm (`service/balance.go`)

Implement the **greedy debt simplification** algorithm:

1. Compute net balance per user: `net[u] = total_paid_by_u - total_owed_by_u`
2. Separate into creditors (net > 0) and debtors (net < 0)
3. Use two-pointer greedy to match debtors to creditors, minimizing number of transactions
4. Return `[]Settlement{From, To, Amount}`

---

## Pinned Summary Message Format

The bot maintains **one pinned message** per ledger channel, updated after every mutation:

```
📒 **室友帳** 帳本摘要
━━━━━━━━━━━━━━━━━━━━
👥 成員：Alice、Bob、Carol、Dave

💰 總支出：NT$1,250.00

📊 結算方案（最少轉帳）：
  Alice 　→　Bob　　NT$300.00
  Carol　→　Alice　NT$150.00

✅ 已結清成員：Dave

最後更新：2025/03/31 14:22
━━━━━━━━━━━━━━━━━━━━
輸入 /add split|for 新增帳目　/member add 新增成員
```

---

## Category Guard Middleware

```go
// middleware.go
func CategoryGuard(s *discordgo.Session, cfg *config.Config, channelID string) error {
    ch, err := s.Channel(channelID)
    // check ch.ParentID → fetch parent → compare name to cfg.AccountCategory
    // return descriptive error if not matching
}
```

All command handlers call this before any business logic.

---

## Error Handling & Responses

- All command responses use `InteractionResponseData` with `Flags: discordgo.MessageFlagsEphemeral` for error/info messages
- Public (non-ephemeral) messages only for: new expense notifications (@mentions), pinned summary updates
- All amounts displayed as `NT$X,XXX.XX` format

---

## Notes

- Use `modernc.org/sqlite` (no CGO required)
- Run `client.Schema.Create(ctx)` on startup for auto migration
- Store all monetary values as `int64` cents to avoid float precision issues
- Snapshot `user_name` at write time (Discord display names can change)
- Handle `CHANNEL_DELETE` gateway event: mark `Ledger.active = false`

## Gateway Events

### `CHANNEL_CREATE`

```go
s.AddHandler(func(s *discordgo.Session, e *discordgo.ChannelCreate) {
    // 1. Ignore if channel type != discordgo.ChannelTypeGuildText (type 0)
    // 2. Fetch parent category via s.Channel(e.ParentID)
    // 3. If parent category name != cfg.AccountCategory → ignore
    // 4. Call ledger service to create Ledger{channel_id, guild_id, category_id, active:true}
    // 5. Extract initial members from PermissionOverwrites (see helper below)
    // 6. Send and pin initial summary message, save pinned_message_id
    // 7. Send a one-time public message in the channel:
    //    "📒 帳本已自動建立！在頻道加入成員即可自動同步至帳本。"
})
```

- If a `Ledger` already exists for this `channel_id` (e.g. race condition), skip silently

### `CHANNEL_UPDATE`

```go
s.AddHandler(func(s *discordgo.Session, e *discordgo.ChannelUpdate) {
    // 1. Ignore if not a tracked ledger channel (look up by channel_id)
    // 2. Extract current members from new PermissionOverwrites (see helper below)
    // 3. Fetch existing LedgerMember list from DB
    // 4. Diff: users in new overwrites but not in DB → UpsertMember(active: true)
    //         users in DB but not in new overwrites → SetActive(false)
    // 5. Refresh pinned summary message
})
```

### `CHANNEL_DELETE`

```go
s.AddHandler(func(s *discordgo.Session, e *discordgo.ChannelDelete) {
    // Mark Ledger.active = false for this channel_id
})
```

### Helper: Extract Members from PermissionOverwrites

Provide a reusable helper used by both `CHANNEL_CREATE` and `CHANNEL_UPDATE`:

```go
// extractMemberIDs returns the list of user IDs that have explicit permission overwrites
// on the channel. Filters to type == 1 (user overwrite) only, excluding roles (type == 0).
// Bot users do NOT appear here because the bot accesses via role overwrite (type == 0),
// so no additional filtering is needed.
func extractMemberIDs(overwrites []*discordgo.PermissionOverwrite) []string {
    var ids []string
    for _, o := range overwrites {
        if o.Type == discordgo.PermissionOverwriteTypeMember { // type == 1
            ids = append(ids, o.ID)
        }
    }
    return ids
}
```

- `type == 0` = role overwrite (e.g. `@everyone`, Bot role) → **ignore**
- `type == 1` = individual user overwrite → **these are the ledger members**
- Bot accesses the channel via its **role** permission on the `記帳` category, so it will appear as `type == 0` and is naturally excluded
- `LedgerMember` is the **single source of truth** for who belongs to a ledger; never infer membership from historical transactions alone
