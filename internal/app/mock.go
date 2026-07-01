package app

import (
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
	"github.com/w0rxbend/twi/internal/config"
	"github.com/w0rxbend/twi/internal/render"
	"github.com/w0rxbend/twi/internal/twitch"
	"golang.org/x/term"
)

const (
	defaultMockWidth  = 88
	defaultMockHeight = 22
)

type fdWriter interface {
	Fd() uintptr
}

type mockShellModel struct {
	channel       string
	animationMode string
	imageMode     string
	status        ConnectionState
	messages      []twitch.ChatMessage
	width         int
	height        int
	focus         mockFocus
	helpExpanded  bool
	composerText  string
	scrollOffset  int
}

var _ tea.Model = mockShellModel{}

type mockFocus int

const (
	mockFocusChat mockFocus = iota
	mockFocusComposer
)

type mockShellLayout struct {
	width                 int
	statusHeight          int
	chatHeight            int
	chatContentHeight     int
	chatFramed            bool
	composerHeight        int
	composerContentHeight int
	composerFramed        bool
	helpHeight            int
}

// RunMock starts the deterministic non-network mock chat shell. When stdout is
// not an interactive terminal, it writes the initial Bubble Tea view and exits
// so tests and redirected commands do not block waiting for keyboard input.
func RunMock(w io.Writer, cfg config.Config) error {
	channel := "mock"
	if len(cfg.DefaultChannels) > 0 {
		channel = cfg.DefaultChannels[0]
	}

	model := newMockShellModel(channel, cfg)
	if !isInteractiveTerminal(w) {
		_, err := fmt.Fprintln(w, model.View())
		return err
	}

	program := tea.NewProgram(model, tea.WithOutput(w), tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func newMockShellModel(channel string, cfg config.Config) mockShellModel {
	connectedAt := time.Date(2026, 7, 2, 20, 0, 0, 0, time.UTC)
	return mockShellModel{
		channel:       channel,
		animationMode: cfg.Features.AnimationMode,
		imageMode:     cfg.Features.ImageMode,
		status: ConnectionState{
			Status:  ConnectionConnected,
			Channel: channel,
			Detail:  "mock source ready",
			At:      connectedAt,
		},
		messages: seededMockMessages(channel, connectedAt),
		width:    defaultMockWidth,
		height:   defaultMockHeight,
		focus:    mockFocusChat,
	}
}

func seededMockMessages(channel string, startedAt time.Time) []twitch.ChatMessage {
	return []twitch.ChatMessage{
		{
			ID:          "mock-1",
			Channel:     channel,
			Timestamp:   startedAt.Add(time.Second),
			AuthorLogin: "twi_bot",
			DisplayName: "twi_bot",
			AuthorColor: "#9146ff",
			Text:        "Mock chat is ready in the Bubble Tea shell.",
			Type:        twitch.MessageTypeChat,
		},
		{
			ID:          "mock-2",
			Channel:     channel,
			Timestamp:   startedAt.Add(2 * time.Second),
			AuthorLogin: "viewer42",
			DisplayName: "viewer42",
			AuthorColor: "#00d1ff",
			Text:        "@twi_bot composer, help, and status regions are visible.",
			Type:        twitch.MessageTypeChat,
		},
		{
			ID:          "mock-3",
			Channel:     channel,
			Timestamp:   startedAt.Add(3 * time.Second),
			AuthorLogin: "moderator",
			DisplayName: "moderator",
			AuthorColor: "#00f593",
			Text:        "No Twitch credentials or network calls are used for --mock.",
			Type:        twitch.MessageTypeNotice,
		},
	}
}

func (m mockShellModel) Init() tea.Cmd {
	return nil
}

func (m mockShellModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyTab:
			m.cycleFocus()
		case tea.KeyPgUp:
			m.scrollBy(m.layout().chatContentHeight)
		case tea.KeyPgDown:
			m.scrollBy(-m.layout().chatContentHeight)
		case tea.KeyBackspace:
			if m.focus == mockFocusComposer {
				m.deleteComposerRune()
			}
		case tea.KeyCtrlU:
			if m.focus == mockFocusComposer {
				m.composerText = ""
			}
		case tea.KeySpace:
			if m.focus == mockFocusComposer {
				m.composerText += " "
			}
		case tea.KeyRunes:
			if len(msg.Runes) == 1 && msg.Runes[0] == '?' {
				m.helpExpanded = !m.helpExpanded
				m.clampScroll()
				return m, nil
			}
			if m.focus == mockFocusChat && len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
				return m, tea.Quit
			}
			if m.focus == mockFocusComposer {
				m.composerText += string(msg.Runes)
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampScroll()
	}
	return m, nil
}

func (m mockShellModel) View() string {
	layout := m.layout()

	regions := make([]string, 0, 4)
	if layout.statusHeight > 0 {
		regions = append(regions, m.statusLine(layout.width))
	}
	if layout.chatHeight > 0 {
		regions = append(regions, m.chatView(layout))
	}
	if layout.composerHeight > 0 {
		regions = append(regions, m.composerView(layout))
	}
	if layout.helpHeight > 0 {
		regions = append(regions, m.helpView(layout.width, layout.helpHeight))
	}

	return lipgloss.JoinVertical(lipgloss.Left, regions...)
}

func (m mockShellModel) statusLine(width int) string {
	left := fmt.Sprintf("#%s %s", m.channel, m.status.Status)
	if width >= 34 && m.status.Detail != "" {
		left += " - " + m.status.Detail
	}
	right := ""
	if width >= 64 {
		right = fmt.Sprintf(" focus=%s animation=%s images=%s", m.focusName(), m.animationMode, m.imageMode)
	} else if width >= 42 {
		right = fmt.Sprintf(" focus=%s", m.focusName())
	}
	line := fitLine(" "+left+right, width)

	return lipgloss.NewStyle().
		Width(width).
		Foreground(lipgloss.Color("#f8f8f2")).
		Background(lipgloss.Color("#4b367c")).
		Bold(true).
		Render(line)
}

func (m mockShellModel) chatView(layout mockShellLayout) string {
	rowWidth := layout.width
	if layout.chatFramed {
		rowWidth = layout.width - 4
	}
	rowWidth = clampMin(rowWidth, 1)

	rows := make([]string, 0, len(m.messages))
	for _, msg := range m.messages {
		rows = append(rows, fitLine(render.TextRow(msg, rowWidth), rowWidth))
	}
	rows = visibleRows(rows, layout.chatContentHeight, m.scrollOffset)

	if len(rows) < layout.chatContentHeight {
		for len(rows) < layout.chatContentHeight {
			rows = append(rows, "")
		}
	}

	content := strings.Join(rows, "\n")
	if !layout.chatFramed {
		return fitBlock(content, layout.width, layout.chatHeight)
	}

	borderColor := lipgloss.Color("#5f6c7b")
	if m.focus == mockFocusChat {
		borderColor = lipgloss.Color("#8bd5ff")
	}
	return lipgloss.NewStyle().
		Width(clampMin(layout.width-2, 0)).
		Height(layout.chatContentHeight).
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Render(content)
}

func (m mockShellModel) composerView(layout mockShellLayout) string {
	label := fmt.Sprintf(" Message #%s", m.channel)
	if m.focus == mockFocusComposer {
		label += " [focus]"
	}
	if layout.width < 28 {
		label = " >"
	}
	input := m.composerText
	if input == "" {
		input = "Type a message..."
	}
	input = " " + fitLine(input, clampMin(layout.width-4, 1))
	if !layout.composerFramed {
		return fitBlock(input, layout.width, layout.composerHeight)
	}

	box := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().Foreground(lipgloss.Color("#8bd5ff")).Render(label),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#a6adc8")).Render(input),
	)

	if layout.composerContentHeight == 1 {
		box = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6adc8")).Render(input)
	}

	borderColor := lipgloss.Color("#2a9d8f")
	if m.focus == mockFocusComposer {
		borderColor = lipgloss.Color("#f9e2af")
	}
	return lipgloss.NewStyle().
		Width(clampMin(layout.width-2, 0)).
		Height(layout.composerContentHeight).
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Render(box)
}

func (m mockShellModel) helpView(width, height int) string {
	lines := m.helpLines(width, height)
	for i := range lines {
		lines[i] = fitLine(lines[i], width)
	}
	return lipgloss.NewStyle().
		Width(width).
		Foreground(lipgloss.Color("#cdd6f4")).
		Background(lipgloss.Color("#1f2430")).
		Render(strings.Join(lines, "\n"))
}

func isInteractiveTerminal(w io.Writer) bool {
	file, ok := w.(fdWriter)
	return ok && term.IsTerminal(int(file.Fd()))
}

func clampMin(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}

func fitLine(value string, width int) string {
	if width <= 0 {
		return ""
	}

	var builder strings.Builder
	used := 0
	graphemes := uniseg.NewGraphemes(value)
	for graphemes.Next() {
		cluster := graphemes.Str()
		clusterWidth := uniseg.StringWidth(cluster)
		if used+clusterWidth > width {
			break
		}
		builder.WriteString(cluster)
		used += clusterWidth
	}
	if used < width {
		builder.WriteString(strings.Repeat(" ", width-used))
	}
	return builder.String()
}

func (m mockShellModel) layout() mockShellLayout {
	width := clampMin(m.width, 1)
	height := clampMin(m.height, 1)
	layout := mockShellLayout{
		width:        width,
		statusHeight: 1,
		helpHeight:   1,
	}
	if height == 1 {
		layout.helpHeight = 0
		return layout
	}

	if m.helpExpanded {
		switch {
		case height >= 14:
			layout.helpHeight = 3
		case height >= 10:
			layout.helpHeight = 2
		}
	}

	layout.composerHeight = 4
	layout.composerContentHeight = 2
	layout.composerFramed = width >= 5
	if height < 10 {
		layout.composerHeight = 3
		layout.composerContentHeight = 1
	}

	remaining := height - layout.statusHeight - layout.helpHeight - layout.composerHeight
	if remaining < 3 && layout.composerHeight > 3 {
		layout.composerHeight = 3
		layout.composerContentHeight = 1
		remaining = height - layout.statusHeight - layout.helpHeight - layout.composerHeight
	}
	if remaining < 1 && layout.helpHeight > 0 {
		layout.helpHeight = 0
		remaining = height - layout.statusHeight - layout.composerHeight
	}
	if remaining < 1 && layout.composerHeight > 0 {
		layout.composerHeight = clampMin(height-layout.statusHeight, 0)
		layout.composerContentHeight = clampMin(layout.composerHeight-2, 0)
		layout.composerFramed = layout.composerHeight >= 3 && width >= 5
		remaining = height - layout.statusHeight - layout.composerHeight
	}

	layout.chatHeight = clampMin(remaining, 0)
	layout.chatFramed = layout.chatHeight >= 3 && width >= 5
	layout.chatContentHeight = layout.chatHeight
	if layout.chatFramed {
		layout.chatContentHeight = layout.chatHeight - 2
	}
	if layout.chatContentHeight < 0 {
		layout.chatContentHeight = 0
	}

	used := layout.statusHeight + layout.chatHeight + layout.composerHeight + layout.helpHeight
	if used < height {
		layout.chatHeight += height - used
		if layout.chatFramed {
			layout.chatContentHeight = layout.chatHeight - 2
		} else {
			layout.chatContentHeight = layout.chatHeight
		}
	}

	return layout
}

func (m *mockShellModel) cycleFocus() {
	if m.focus == mockFocusChat {
		m.focus = mockFocusComposer
		return
	}
	m.focus = mockFocusChat
}

func (m *mockShellModel) scrollBy(delta int) {
	if delta == 0 {
		delta = 1
	}
	m.scrollOffset += delta
	m.clampScroll()
}

func (m *mockShellModel) clampScroll() {
	maxScroll := m.maxScrollOffset()
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m mockShellModel) maxScrollOffset() int {
	visible := m.layout().chatContentHeight
	if visible <= 0 || len(m.messages) <= visible {
		return 0
	}
	return len(m.messages) - visible
}

func (m *mockShellModel) deleteComposerRune() {
	if m.composerText == "" {
		return
	}
	runes := []rune(m.composerText)
	m.composerText = string(runes[:len(runes)-1])
}

func (m mockShellModel) focusName() string {
	if m.focus == mockFocusComposer {
		return "composer"
	}
	return "chat"
}

func (m mockShellModel) helpLines(width, height int) []string {
	if !m.helpExpanded {
		if width < 20 {
			return []string{" tab | ?"}
		}
		if width < 38 {
			return []string{" tab focus | ? help"}
		}
		return []string{" tab focus | ? help | pg scroll | q quit | ctrl+c quit | no network"}
	}

	lines := []string{
		" tab focus: chat/composer",
		" pgup/pgdn: scroll chat | ?: compact help",
		" q: quit from chat | ctrl+c: quit | mock source: no network",
	}
	if width < 38 {
		lines = []string{
			" tab: focus",
			" pgup/pgdn: scroll",
			" ?: help | ctrl+c: quit",
		}
	}
	if len(lines) > height {
		return lines[:height]
	}
	return lines
}

func visibleRows(rows []string, height, scrollOffset int) []string {
	if height <= 0 || len(rows) == 0 {
		return nil
	}
	if len(rows) <= height {
		return rows
	}

	maxScroll := len(rows) - height
	if scrollOffset > maxScroll {
		scrollOffset = maxScroll
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	end := len(rows) - scrollOffset
	start := end - height
	if start < 0 {
		start = 0
	}
	return rows[start:end]
}

func fitBlock(value string, width, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(value, "\n")
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, fitLine(line, width))
	}
	return strings.Join(out, "\n")
}
