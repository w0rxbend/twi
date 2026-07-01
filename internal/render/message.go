package render

import (
	"fmt"
	"strings"

	"github.com/w0rxbend/twi/internal/twitch"
)

func TextRow(msg twitch.ChatMessage, width int) string {
	if width < 24 {
		width = 24
	}

	timeText := msg.Timestamp.Local().Format("15:04")
	author := msg.DisplayName
	if author == "" {
		author = msg.AuthorLogin
	}
	if author == "" {
		author = "unknown"
	}

	prefix := fmt.Sprintf("%s %-14s ", timeText, author)
	available := width - len(prefix)
	if available < 12 {
		available = 12
	}

	text := strings.TrimSpace(msg.Text)
	text = truncateRunes(text, available)
	return prefix + text
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}
