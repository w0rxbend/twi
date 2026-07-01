package app

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/w0rxbend/twi/internal/config"
	"github.com/w0rxbend/twi/internal/twitch"
)

func TestRunMockRendersInitialShellForNonInteractiveOutput(t *testing.T) {
	cfg := config.Default()
	cfg.DefaultChannels = []string{"example"}

	var out bytes.Buffer
	if err := RunMock(&out, cfg); err != nil {
		t.Fatalf("RunMock returned error: %v", err)
	}

	view := out.String()
	for _, want := range []string{
		"#example",
		"connected",
		"Mock chat is ready in the Bubble Tea shell.",
		"Message #example",
		"q quit",
		"ctrl+c quit",
		"no network",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("initial view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Run without --mock") {
		t.Fatalf("initial view still contains old static snapshot text:\n%s", view)
	}
}

func TestMockShellQuitsOnQAndCtrlC(t *testing.T) {
	model := newMockShellModel("example", config.Default())

	for name, msg := range map[string]tea.KeyMsg{
		"q":      {Type: tea.KeyRunes, Runes: []rune{'q'}},
		"ctrl+c": {Type: tea.KeyCtrlC},
	} {
		t.Run(name, func(t *testing.T) {
			_, cmd := model.Update(msg)
			if cmd == nil {
				t.Fatal("Update returned nil command, want tea.Quit")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("Update command produced %T, want tea.QuitMsg", cmd())
			}
		})
	}
}

func TestMockShellWindowSizeKeepsViewWithinHeight(t *testing.T) {
	model := newMockShellModel("example", config.Default())

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 64, Height: 12})
	view := updated.View()

	if got, want := lineCount(view), 12; got != want {
		t.Fatalf("view line count = %d, want %d:\n%s", got, want, view)
	}
}

func TestMockShellFocusHelpAndComposerInput(t *testing.T) {
	model := newMockShellModel("example", config.Default())

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(mockShellModel)
	if got, want := model.focus, mockFocusComposer; got != want {
		t.Fatalf("focus after tab = %v, want %v", got, want)
	}
	if !strings.Contains(model.View(), "focus=composer") {
		t.Fatalf("composer focus marker missing:\n%s", model.View())
	}

	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'h', 'i'}},
		{Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
	} {
		updated, cmd := model.Update(msg)
		if cmd != nil {
			t.Fatalf("composer input returned command for %#v", msg)
		}
		model = updated.(mockShellModel)
	}
	if got, want := model.composerText, "hi q"; got != want {
		t.Fatalf("composer text = %q, want %q", got, want)
	}
	if !strings.Contains(model.View(), "hi q") {
		t.Fatalf("composer view missing typed text:\n%s", model.View())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = updated.(mockShellModel)
	if !model.helpExpanded {
		t.Fatal("helpExpanded = false, want true")
	}
	if !strings.Contains(model.View(), "pgup/pgdn") {
		t.Fatalf("expanded help missing page key hint:\n%s", model.View())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(mockShellModel)
	if got, want := model.focus, mockFocusChat; got != want {
		t.Fatalf("focus after second tab = %v, want %v", got, want)
	}
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q from chat focus returned nil command, want tea.Quit")
	}
}

func TestMockShellPageKeysScrollViewport(t *testing.T) {
	model := newMockShellModel("example", config.Default())
	model.messages = numberedMockMessages("example", 12)

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 72, Height: 12})
	model = updated.(mockShellModel)
	bottom := model.View()
	if !strings.Contains(bottom, "message-11") {
		t.Fatalf("bottom viewport missing latest message:\n%s", bottom)
	}
	if strings.Contains(bottom, "message-00") {
		t.Fatalf("bottom viewport unexpectedly contains oldest message:\n%s", bottom)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(mockShellModel)
	scrolled := model.View()
	if !strings.Contains(scrolled, "message-04") {
		t.Fatalf("page up viewport missing previous page message:\n%s", scrolled)
	}
	if model.scrollOffset == 0 {
		t.Fatal("scrollOffset = 0 after page up, want non-zero")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(mockShellModel)
	if !strings.Contains(model.View(), "message-00") {
		t.Fatalf("second page up viewport missing oldest message:\n%s", model.View())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(mockShellModel)
	if model.scrollOffset == 0 {
		t.Fatal("scrollOffset after one page down = 0, want still scrolled")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(mockShellModel)
	if model.scrollOffset != 0 {
		t.Fatalf("scrollOffset after second page down = %d, want 0", model.scrollOffset)
	}
	if !strings.Contains(model.View(), "message-11") {
		t.Fatalf("second page down viewport missing latest message:\n%s", model.View())
	}
}

func TestMockShellNarrowLayoutStaysWithinBounds(t *testing.T) {
	model := newMockShellModel("example", config.Default())
	model.composerText = "hello 😀 表"
	model.messages = append(model.messages, twitch.ChatMessage{
		ID:          "wide",
		Channel:     "example",
		Timestamp:   time.Date(2026, 7, 2, 20, 0, 10, 0, time.UTC),
		AuthorLogin: "wide",
		DisplayName: "wide",
		Text:        "emoji 😀 and CJK 表 stay inside",
		Type:        twitch.MessageTypeChat,
	})

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 24, Height: 8})
	view := updated.View()

	if got, want := lineCount(view), 8; got != want {
		t.Fatalf("narrow view line count = %d, want %d:\n%s", got, want, view)
	}
	for i, line := range strings.Split(strings.TrimSuffix(view, "\n"), "\n") {
		if got, want := lipglossWidth(line), 24; got > want {
			t.Fatalf("line %d width = %d, want <= %d:\n%s", i+1, got, want, view)
		}
	}
	for _, notWant := range []string{"animation=", "images=", "mock source ready"} {
		if strings.Contains(view, notWant) {
			t.Fatalf("narrow view contains nonessential status text %q:\n%s", notWant, view)
		}
	}
	for _, want := range []string{"#example connected", "hello"} {
		if !strings.Contains(view, want) {
			t.Fatalf("narrow view missing %q:\n%s", want, view)
		}
	}
}

func TestMockShellTinyWidthsDoNotExceedWindowWidth(t *testing.T) {
	for width := 1; width <= 5; width++ {
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			model := newMockShellModel("example", config.Default())
			model.composerText = "😀表"

			updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: 8})
			view := updated.View()

			if got, want := lineCount(view), 8; got != want {
				t.Fatalf("tiny view line count = %d, want %d:\n%s", got, want, view)
			}
			for i, line := range strings.Split(strings.TrimSuffix(view, "\n"), "\n") {
				if got := lipglossWidth(line); got > width {
					t.Fatalf("line %d width = %d, want <= %d:\n%s", i+1, got, width, view)
				}
			}
		})
	}
}

func lineCount(value string) int {
	value = strings.TrimSuffix(value, "\n")
	if value == "" {
		return 0
	}
	return strings.Count(value, "\n") + 1
}

func numberedMockMessages(channel string, count int) []twitch.ChatMessage {
	startedAt := time.Date(2026, 7, 2, 20, 0, 0, 0, time.UTC)
	messages := make([]twitch.ChatMessage, 0, count)
	for i := 0; i < count; i++ {
		messages = append(messages, twitch.ChatMessage{
			ID:          fmt.Sprintf("mock-%02d", i),
			Channel:     channel,
			Timestamp:   startedAt.Add(time.Duration(i) * time.Second),
			AuthorLogin: "viewer",
			DisplayName: "viewer",
			Text:        fmt.Sprintf("message-%02d", i),
			Type:        twitch.MessageTypeChat,
		})
	}
	return messages
}

func lipglossWidth(value string) int {
	return lipgloss.Width(value)
}
