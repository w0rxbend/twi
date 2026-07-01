package theme

import "testing"

func TestContrastCorrectedForegroundKeepsReadableColor(t *testing.T) {
	got := ContrastCorrectedForeground("#00d1ff", "#111018", "#f6f2ff")
	if want := "#00d1ff"; got != want {
		t.Fatalf("color = %q, want %q", got, want)
	}
}

func TestContrastCorrectedForegroundUsesFallbackForLowContrastColor(t *testing.T) {
	got := ContrastCorrectedForeground("#111111", "#111018", "#f6f2ff")
	if want := "#f6f2ff"; got != want {
		t.Fatalf("color = %q, want %q", got, want)
	}
}

func TestContrastCorrectedForegroundHandlesInvalidInput(t *testing.T) {
	got := ContrastCorrectedForeground("not-a-color", "#111018", "#f6f2ff")
	if want := "#f6f2ff"; got != want {
		t.Fatalf("color = %q, want %q", got, want)
	}
}
