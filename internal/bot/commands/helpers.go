package commands

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func respond(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	})
}

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
