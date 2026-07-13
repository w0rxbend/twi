package app

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const splashProgressWidth = 24

// splashActive reports whether the ~2s animated startup splash should still
// cover the normal dashboard. Any keypress sets splashSkipped so users are
// never trapped waiting it out.
func (m mockShellModel) splashActive() bool {
	if m.splashUntil.IsZero() || m.splashSkipped {
		return false
	}
	return time.Now().Before(m.splashUntil)
}

// splashView renders the startup splash: a centered wordmark, tagline, and a
// progress bar that fills as the splash's ~2s window elapses. Every visible
// cell uses the active theme, matching the rest of the dashboard.
func (m mockShellModel) splashView() string {
	width := clampMin(m.width, 1)
	height := clampMin(m.height, 1)

	elapsed := splashDuration - time.Until(m.splashUntil)
	fraction := float64(elapsed) / float64(splashDuration)
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	filled := int(fraction * splashProgressWidth)
	bar := strings.Repeat("#", filled) + strings.Repeat("-", splashProgressWidth-filled)

	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent)).Background(lipgloss.Color(m.theme.Background)).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Muted)).Background(lipgloss.Color(m.theme.Background))

	wordmark := "twi"
	tagline := "terminal Twitch chat"
	barLine := "[" + bar + "]"
	textWidth := lipgloss.Width(tagline)
	if w := lipgloss.Width(barLine); w > textWidth {
		textWidth = w
	}

	// Center each line as plain text *before* styling it, rather than
	// joining already-styled (already-reset-terminated) lines with
	// lipgloss.JoinVertical(lipgloss.Center, ...): that helper centers by
	// padding with plain, unstyled spaces, which carry no background since
	// they're added after each line's own ANSI reset. Styling the
	// already-centered plain text in one Render() call colors the padding
	// along with the text.
	content := strings.Join([]string{
		accent.Render(centeredPlainLine(wordmark, textWidth)),
		muted.Render(centeredPlainLine(tagline, textWidth)),
		muted.Render(centeredPlainLine("", textWidth)),
		muted.Render(centeredPlainLine(barLine, textWidth)),
	}, "\n")

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Background(lipgloss.Color(m.theme.Background)).
		Render(content)
}

// centeredPlainLine center-pads plain (non-ANSI) text to width with spaces.
// Callers must style the result afterward in one Render() call rather than
// centering already-styled text, so the padding this adds shares the same
// background as the text (see splashView's doc comment).
func centeredPlainLine(text string, width int) string {
	textWidth := lipgloss.Width(text)
	if textWidth >= width {
		return text
	}
	total := width - textWidth
	left := total / 2
	right := total - left
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
}
