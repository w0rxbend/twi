package app

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/w0rxbend/twi/internal/config"
	"github.com/w0rxbend/twi/internal/render"
	"github.com/w0rxbend/twi/internal/twitch"
)

// RunMock prints a deterministic mock chat snapshot. It is intentionally
// simple until the Bubble Tea shell lands in the next implementation slice.
func RunMock(w io.Writer, cfg config.Config) error {
	channel := "mock"
	if len(cfg.DefaultChannels) > 0 {
		channel = cfg.DefaultChannels[0]
	}

	messages := []twitch.ChatMessage{
		{
			ID:          "mock-1",
			Channel:     channel,
			Timestamp:   time.Date(2026, 7, 1, 20, 0, 0, 0, time.UTC),
			AuthorLogin: "twi_bot",
			DisplayName: "twi_bot",
			AuthorColor: "#9146ff",
			Text:        "Mock chat is ready. Bubble Tea rendering lands next.",
			Type:        twitch.MessageTypeChat,
		},
		{
			ID:          "mock-2",
			Channel:     channel,
			Timestamp:   time.Date(2026, 7, 1, 20, 0, 1, 0, time.UTC),
			AuthorLogin: "viewer",
			DisplayName: "viewer",
			AuthorColor: "#00d1ff",
			Text:        "@twi_bot typed-in reveal will use normalized fragments.",
			Type:        twitch.MessageTypeChat,
		},
	}

	fmt.Fprintf(w, "twi mock chat #%s\n", channel)
	fmt.Fprintf(w, "animation=%s images=%s avatar=%s\n\n", cfg.Features.AnimationMode, cfg.Features.ImageMode, cfg.Features.AvatarMode)
	for _, msg := range messages {
		fmt.Fprintln(w, render.TextRow(msg, 96))
	}
	fmt.Fprintln(w, strings.Repeat("-", 72))
	fmt.Fprintln(w, "Run without --mock after Twitch transport is implemented.")
	return nil
}
