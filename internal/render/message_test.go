package render

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/w0rxbend/twi/internal/twitch"
)

func TestTextRowTruncatesWithoutBreakingUTF8(t *testing.T) {
	msg := twitch.ChatMessage{
		Timestamp:   time.Date(2026, 7, 1, 20, 0, 0, 0, time.UTC),
		DisplayName: "viewer",
		Text:        "hello 😀 Привіт chat",
	}

	row := TextRow(msg, 32)

	if !utf8.ValidString(row) {
		t.Fatalf("row is invalid UTF-8: %q", row)
	}
	if !strings.HasSuffix(row, "...") {
		t.Fatalf("row = %q, want ellipsis suffix", row)
	}
}

func TestTextRowUsesFallbackAuthor(t *testing.T) {
	msg := twitch.ChatMessage{
		Timestamp:   time.Date(2026, 7, 1, 20, 0, 0, 0, time.UTC),
		AuthorLogin: "login",
		Text:        "message",
	}

	row := TextRow(msg, 80)

	if !strings.Contains(row, "login") {
		t.Fatalf("row = %q, want fallback author login", row)
	}
}
