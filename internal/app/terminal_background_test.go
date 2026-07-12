package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/w0rxbend/twi/internal/config"
)

func TestThemeBackgroundSequenceOnlyWhenInteractive(t *testing.T) {
	model := newMockShellModel("alpha", config.Default())
	if got := model.themeBackgroundSequence(); got != "" {
		t.Fatalf("themeBackgroundSequence() without terminalOutput = %q, want empty", got)
	}

	var buf bytes.Buffer
	model.terminalOutput = &buf
	want := "\x1b]11;" + model.theme.Background + "\x07"
	if got := model.themeBackgroundSequence(); got != want {
		t.Fatalf("themeBackgroundSequence() = %q, want %q", got, want)
	}
}

func TestViewEmbedsThemeBackgroundSequenceWhenInteractive(t *testing.T) {
	var buf bytes.Buffer
	model := newMockShellModel("alpha", config.Default())
	model.width, model.height = 88, 22
	model.terminalOutput = &buf

	view := model.View()
	want := "\x1b]11;" + model.theme.Background + "\x07"
	if !strings.HasPrefix(view, want) {
		t.Fatalf("View() does not start with theme background sequence %q:\n%s", want, view)
	}
}

func TestViewOmitsThemeBackgroundSequenceWhenNotInteractive(t *testing.T) {
	model := newMockShellModel("alpha", config.Default())
	model.width, model.height = 88, 22

	view := model.View()
	if strings.Contains(view, "\x1b]11;") {
		t.Fatalf("non-interactive View() unexpectedly includes OSC 11 sequence:\n%s", view)
	}
}

func TestViewEmbedsThemeBackgroundSequenceDuringSplash(t *testing.T) {
	var buf bytes.Buffer
	model := newMockShellModel("alpha", config.Default())
	model.width, model.height = 88, 22
	model.terminalOutput = &buf
	model.splashUntil = time.Now().Add(splashDuration)

	view := model.View()
	want := "\x1b]11;" + model.theme.Background + "\x07"
	if !strings.HasPrefix(view, want) {
		t.Fatalf("splash View() does not start with theme background sequence %q:\n%s", want, view)
	}
}

func TestPrimeTerminalBackgroundEmitsOSC11(t *testing.T) {
	var buf bytes.Buffer
	primeTerminalBackground(&buf, "#111018")
	if got, want := buf.String(), "\x1b]11;#111018\x07"; got != want {
		t.Fatalf("primeTerminalBackground output = %q, want %q", got, want)
	}
}

func TestPrimeTerminalBackgroundNoopWithEmptyColor(t *testing.T) {
	var buf bytes.Buffer
	primeTerminalBackground(&buf, "")
	if buf.Len() != 0 {
		t.Fatalf("primeTerminalBackground with empty color wrote %q, want nothing", buf.String())
	}
}

func TestResetTerminalBackgroundEmitsOSC111(t *testing.T) {
	var buf bytes.Buffer
	resetTerminalBackground(&buf)
	if got, want := buf.String(), "\x1b]111\x07"; got != want {
		t.Fatalf("resetTerminalBackground output = %q, want %q", got, want)
	}
}
