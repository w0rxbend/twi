package theme

import (
	"reflect"
	"testing"
)

func TestPresetNamesIncludesAllThirteen(t *testing.T) {
	want := []string{
		"btop", "catppuccin-mocha", "claude", "codex", "dracula", "gruvbox",
		"mono", "monokai", "nord", "one-dark", "rose-pine", "solarized-dark",
		"tokyo-night",
	}
	got := PresetNames()
	if len(got) != len(want) {
		t.Fatalf("PresetNames() = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("PresetNames()[%d] = %q, want %q", i, got[i], name)
		}
	}
}

func TestPresetNamesAreValidPalettes(t *testing.T) {
	for _, name := range PresetNames() {
		palette := Presets()[name]
		if palette == (Palette{}) {
			t.Fatalf("preset %q has zero-value palette", name)
		}
		if _, ok := parseHexColor(palette.Background); !ok {
			t.Fatalf("preset %q has invalid background %q", name, palette.Background)
		}
	}
}

func TestDefaultPaletteIsClaudePreset(t *testing.T) {
	if got, want := DefaultPalette(), Presets()["claude"]; got != want {
		t.Fatalf("DefaultPalette() = %+v, want %+v", got, want)
	}
}

func TestResolvePaletteKnownPreset(t *testing.T) {
	got, ok := ResolvePalette("Nord", Palette{})
	if !ok {
		t.Fatal("ResolvePalette(\"Nord\", ...) ok = false, want true")
	}
	if want := Presets()["nord"]; got != want {
		t.Fatalf("ResolvePalette(\"Nord\", ...) = %+v, want %+v", got, want)
	}
}

func TestResolvePaletteCustom(t *testing.T) {
	custom := Palette{Background: "#010101", Foreground: "#fefefe"}
	got, ok := ResolvePalette("custom", custom)
	if !ok || got != custom {
		t.Fatalf("ResolvePalette(\"custom\", %+v) = (%+v, %v), want (%+v, true)", custom, got, ok, custom)
	}
}

func TestResolvePaletteUnknownFallsBackToDefault(t *testing.T) {
	got, ok := ResolvePalette("not-a-theme", Palette{})
	if ok {
		t.Fatal("ResolvePalette(\"not-a-theme\", ...) ok = true, want false")
	}
	if want := DefaultPalette(); got != want {
		t.Fatalf("ResolvePalette(\"not-a-theme\", ...) = %+v, want %+v", got, want)
	}
}

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

func TestGradientInterpolatesEndpointsAndMidpoint(t *testing.T) {
	got := Gradient("#ff8000", "#00c0ff", 3)
	want := []string{"#ff8000", "#80a080", "#00c0ff"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Gradient() = %v, want %v", got, want)
	}
}

func TestGradientDegradesSafelyForInvalidCustomColors(t *testing.T) {
	got := Gradient("accent", "#ffffff", 2)
	want := []string{"accent", "accent"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Gradient() = %v, want %v", got, want)
	}
}

func TestDarkenAdjustsValidColorsAndPreservesInvalidValues(t *testing.T) {
	if got, want := Darken("#204060", 0.25), "#183048"; got != want {
		t.Fatalf("Darken() = %q, want %q", got, want)
	}
	if got, want := Darken("#204060", -1), "#204060"; got != want {
		t.Fatalf("Darken() with negative amount = %q, want %q", got, want)
	}
	if got, want := Darken("custom-background", 0.25), "custom-background"; got != want {
		t.Fatalf("Darken() invalid value = %q, want %q", got, want)
	}
}

func TestIdentityColorIsStableDistinctAndReadable(t *testing.T) {
	palette := DefaultPalette()
	backgrounds := []string{palette.Background, palette.Surface}
	alice := IdentityColor("Alice", backgrounds, palette.Foreground)
	if got := IdentityColor("alice", backgrounds, palette.Foreground); got != alice {
		t.Fatalf("case-normalized identity color = %q, want stable %q", got, alice)
	}
	bob := IdentityColor("bob", backgrounds, palette.Foreground)
	if bob == alice {
		t.Fatalf("different identities shared color %q", alice)
	}
	color, ok := parseHexColor(alice)
	if !ok {
		t.Fatalf("identity color %q is not valid hex", alice)
	}
	for _, value := range backgrounds {
		background, ok := parseHexColor(value)
		if !ok {
			t.Fatalf("test background %q is not valid hex", value)
		}
		if ratio := contrastRatio(color, background); ratio < minimumTextContrast {
			t.Fatalf("identity color contrast against %q = %.2f, want >= %.2f", value, ratio, minimumTextContrast)
		}
	}
	if got := IdentityColor("", backgrounds, palette.Foreground); got != palette.Foreground {
		t.Fatalf("empty identity color = %q, want fallback %q", got, palette.Foreground)
	}
}
