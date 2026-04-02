package bot

import (
	"fmt"

	"bill-bot/internal/config"

	"github.com/bwmarrin/discordgo"
)

// CategoryGuard checks that the channel belongs to the configured account category.
func CategoryGuard(s *discordgo.Session, cfg *config.Config, channelID string) error {
	ch, err := s.Channel(channelID)
	if err != nil {
		return fmt.Errorf("無法取得頻道資訊")
	}
	if ch.ParentID == "" {
		return fmt.Errorf("此指令只能在「%s」分類下的頻道使用", cfg.AccountCategory)
	}
	parent, err := s.Channel(ch.ParentID)
	if err != nil {
		return fmt.Errorf("無法取得頻道分類資訊")
	}
	if parent.Name != cfg.AccountCategory {
		return fmt.Errorf("此指令只能在「%s」分類下的頻道使用", cfg.AccountCategory)
	}
	return nil
}
