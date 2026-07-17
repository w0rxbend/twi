package app

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

const splashProgressWidth = 28

var splashLogo = []string{
	"████████╗██╗    ██╗██╗",
	"╚══██╔══╝██║    ██║██║",
	"   ██║   ██║ █╗ ██║██║",
	"   ██║   ██║███╗██║██║",
	"   ╚═╝   ╚══╝╚══╝╚═╝╚═╝",
}

// splashActive reports whether the animated startup splash should still cover
// the normal dashboard. Any keypress sets splashSkipped so users are never
// trapped waiting it out.
func (m mockShellModel) splashActive() bool {
	if m.splashUntil.IsZero() || m.splashSkipped {
		return false
	}
	return time.Now().Before(m.splashUntil)
}

func (m mockShellModel) splashView() string {
	return m.splashViewAt(time.Now())
}

// splashViewAt renders a staged, terminal-native boot sequence. The logo's
// accent gradient shimmers with the shared frame clock while the tagline types
// in and the progress head moves through named initialization phases. Keeping
// now explicit makes every animation state deterministic in tests while
// splashView remains the production wall-clock adapter.
func (m mockShellModel) splashViewAt(now time.Time) string {
	width := clampMin(m.width, 1)
	height := clampMin(m.height, 1)
	fraction := m.splashFraction(now)
	canvas := m.canvasBackground()

	contentWidth := splashContentWidth(width)
	logo := splashLogo
	if width < 38 || height < 13 {
		logo = []string{"╭── ✦ twi ✦ ──╮"}
	}
	logoWidth := widestLine(logo)
	phase := m.gradientPhase(clampMin(logoWidth, 1))

	logoLines := make([]string, 0, len(logo))
	for row, line := range logo {
		centered := centeredFittedLine(line, contentWidth)
		logoLines = append(logoLines, gradientForegroundText(
			centered,
			m.theme.Accent,
			m.gradientEndColor(),
			canvas,
			phase+row*2,
			true,
		))
	}

	tagline := revealDisplayCells("twi // terminal Twitch chat", int(float64(27)*clampFraction((fraction-0.12)/0.38)))
	taglineLine := splashStyledLine(centeredFittedLine(tagline, contentWidth), m.theme.Foreground, canvas, true)
	decorativeLine := splashStyledLine(centeredFittedLine("✦  expressive chat, zero browser chrome  ✦", contentWidth), m.theme.Muted, canvas, false)
	blankLine := splashStyledLine(centeredFittedLine("", contentWidth), m.theme.Muted, canvas, false)
	progressWidth := minInt(splashProgressWidth, clampMin(contentWidth-2, 0))
	progressLine := gradientForegroundText(
		centeredFittedLine(splashProgressBar(fraction, progressWidth), contentWidth),
		m.theme.Accent,
		m.gradientEndColor(),
		canvas,
		phase,
		true,
	)
	phaseLine := splashStyledLine(centeredFittedLine(splashPhaseLabel(fraction, m.activeChannelName()), contentWidth), m.theme.Muted, canvas, false)
	lines := splashLinesForHeight(height, logoLines, taglineLine, decorativeLine, blankLine, progressLine, phaseLine)

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Background(lipgloss.Color(canvas)).
		Render(content)
}

func splashLinesForHeight(height int, logo []string, tagline, decorative, blank, progress, phase string) []string {
	if height <= 1 {
		return logo[:1]
	}
	if height == 2 {
		return []string{logo[0], phase}
	}
	if height == 3 {
		return []string{logo[0], progress, phase}
	}
	if height == 4 {
		return []string{logo[0], tagline, progress, phase}
	}
	if height == 5 {
		return []string{logo[0], tagline, decorative, progress, phase}
	}
	lines := make([]string, 0, len(logo)+5)
	lines = append(lines, logo...)
	lines = append(lines, tagline, decorative, blank, progress, phase)
	return lines
}

func (m mockShellModel) splashFraction(now time.Time) float64 {
	elapsed := splashDuration - m.splashUntil.Sub(now)
	return clampFraction(float64(elapsed) / float64(splashDuration))
}

func splashContentWidth(width int) int {
	if width <= 2 {
		return width
	}
	return minInt(width-2, 54)
}

func splashProgressBar(fraction float64, width int) string {
	if width <= 0 {
		return "◆"
	}
	filled := int(clampFraction(fraction) * float64(width))
	if filled >= width {
		return "[" + strings.Repeat("━", width) + "]"
	}
	return "[" + strings.Repeat("━", filled) + "◆" + strings.Repeat("·", width-filled-1) + "]"
}

func splashPhaseLabel(fraction float64, channel string) string {
	switch {
	case fraction < 0.22:
		return "◌ twi · loading palette"
	case fraction < 0.48:
		return "◐ warming animation clock"
	case fraction < 0.76:
		return "◓ composing chat surfaces"
	default:
		return "● ready for #" + normalizeChannelName(channel)
	}
}

func revealDisplayCells(value string, cells int) string {
	if cells <= 0 {
		return ""
	}
	var builder strings.Builder
	used := 0
	graphemes := uniseg.NewGraphemes(value)
	for graphemes.Next() {
		cluster := graphemes.Str()
		width := uniseg.StringWidth(cluster)
		if used+width > cells {
			break
		}
		builder.WriteString(cluster)
		used += width
	}
	return builder.String()
}

func widestLine(lines []string) int {
	widest := 0
	for _, line := range lines {
		widest = maxInt(widest, uniseg.StringWidth(line))
	}
	return widest
}

func splashStyledLine(text, foreground, background string, bold bool) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(foreground)).
		Background(lipgloss.Color(background)).
		Bold(bold).
		Render(text)
}

func clampFraction(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// centeredPlainLine center-pads plain (non-ANSI) text to width with spaces.
// Callers style the complete padded result so every cell carries the active
// terminal background instead of exposing transparent gaps after ANSI resets.
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

func centeredFittedLine(text string, width int) string {
	return centeredPlainLine(strings.TrimRight(fitLine(text, width), " "), width)
}
