package render

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/w0rxbend/twi/internal/storage"
	"github.com/w0rxbend/twi/internal/twitch"
)

func TestKittyRendererProducesFixedCellOutput(t *testing.T) {
	path := writeTinyPNG(t)
	asset := storage.AssetRecord{
		Key:         storage.AssetKey{Kind: "emoji", ID: "1f600"},
		Path:        path,
		MediaType:   "image/png",
		WidthCells:  2,
		HeightCells: 1,
	}
	spec := ImageSpec{
		Ref:         twitch.AssetRef{Kind: "emoji", ID: "1f600"},
		WidthCells:  4,
		HeightCells: 1,
		Fallback:    "😀",
	}
	renderer := NewKittyRenderer(supportedKittyDecision())

	cell, err := renderer.RenderImage(context.Background(), asset, spec)
	if err != nil {
		t.Fatalf("RenderImage returned error: %v", err)
	}
	if cell.WidthCells != 4 {
		t.Fatalf("cell.WidthCells = %d, want 4", cell.WidthCells)
	}
	if !strings.HasPrefix(cell.Text, "\x1b_G") || !strings.Contains(cell.Text, "a=T") {
		t.Fatalf("cell.Text missing Kitty graphics command: %q", cell.Text)
	}
	for _, want := range []string{"f=100", "t=f", "q=2", "C=1", "c=4", "r=1"} {
		if !strings.Contains(cell.Text, want) {
			t.Fatalf("cell.Text = %q, want it to contain %q", cell.Text, want)
		}
	}
	if !strings.Contains(cell.Text, base64.StdEncoding.EncodeToString([]byte(path))) {
		t.Fatalf("cell.Text does not include encoded cached path: %q", cell.Text)
	}
	if !strings.HasSuffix(cell.Text, strings.Repeat(" ", 4)) {
		t.Fatalf("cell.Text should end with four width-reserving spaces: %q", cell.Text)
	}
}

func TestKittyRendererUnsupportedTerminalReturnsFallbackCell(t *testing.T) {
	asset := storage.AssetRecord{
		Key:       storage.AssetKey{Kind: "twitch_emote", ID: "25"},
		Path:      "does-not-need-to-exist.png",
		MediaType: "image/png",
	}
	spec := ImageSpec{WidthCells: 6, HeightCells: 1, Fallback: "Kappa"}
	renderer := NewKittyRenderer(ImageCapabilityDecision{
		Status:      ImageCapabilityUnsupported,
		EnableKitty: true,
		Signals:     TerminalImageSignals{KittyCompatible: false},
	})

	cell, err := renderer.RenderImage(context.Background(), asset, spec)
	if !errors.Is(err, ErrImageUnsupported) {
		t.Fatalf("RenderImage error = %v, want ErrImageUnsupported", err)
	}
	if cell.WidthCells != 6 {
		t.Fatalf("cell.WidthCells = %d, want 6", cell.WidthCells)
	}
	if got, want := cell.Text, "Kappa "; got != want {
		t.Fatalf("cell.Text = %q, want fallback %q", got, want)
	}
}

func TestKittyRendererFailurePreservesReservedWidth(t *testing.T) {
	secretLookingPath := filepath.Join(t.TempDir(), "oauth:fixture-token.png")
	spec := ImageSpec{WidthCells: 5, HeightCells: 1, Fallback: "[AL]"}
	renderer := NewKittyRenderer(supportedKittyDecision())

	cell, err := renderer.RenderImage(context.Background(), storage.AssetRecord{
		Key:       storage.AssetKey{Kind: "avatar", ID: "user-1"},
		Path:      secretLookingPath,
		MediaType: "image/png",
	}, spec)
	if !errors.Is(err, ErrImageRenderFailed) {
		t.Fatalf("RenderImage error = %v, want ErrImageRenderFailed", err)
	}
	if strings.Contains(err.Error(), "oauth:fixture-token") || strings.Contains(err.Error(), secretLookingPath) {
		t.Fatalf("RenderImage error leaked cached path detail: %v", err)
	}
	if cell.WidthCells != 5 {
		t.Fatalf("cell.WidthCells = %d, want 5", cell.WidthCells)
	}
	if got, want := cell.Text, "[AL] "; got != want {
		t.Fatalf("cell.Text = %q, want fallback %q", got, want)
	}
}

func TestKittyRendererCancellationReturnsFallbackCell(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	spec := ImageSpec{WidthCells: 2, HeightCells: 1, Fallback: "😀"}
	renderer := NewKittyRenderer(supportedKittyDecision())

	cell, err := renderer.RenderImage(ctx, storage.AssetRecord{
		Key:       storage.AssetKey{Kind: "emoji", ID: "1f600"},
		Path:      writeTinyPNG(t),
		MediaType: "image/png",
	}, spec)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RenderImage error = %v, want context.Canceled", err)
	}
	if cell.WidthCells != 2 {
		t.Fatalf("cell.WidthCells = %d, want 2", cell.WidthCells)
	}
	if got, want := cell.Text, "😀"; got != want {
		t.Fatalf("cell.Text = %q, want fallback %q", got, want)
	}
}

func TestKittyRendererRejectsUnsupportedMediaTypeWithFallback(t *testing.T) {
	spec := ImageSpec{WidthCells: 6, HeightCells: 1, Fallback: "Kappa"}
	renderer := NewKittyRenderer(supportedKittyDecision())

	cell, err := renderer.RenderImage(context.Background(), storage.AssetRecord{
		Key:       storage.AssetKey{Kind: "twitch_emote", ID: "25"},
		Path:      writeTinyPNG(t),
		MediaType: "image/webp",
	}, spec)
	if !errors.Is(err, ErrImageRenderFailed) {
		t.Fatalf("RenderImage error = %v, want ErrImageRenderFailed", err)
	}
	if got, want := cell.Text, "Kappa "; got != want {
		t.Fatalf("cell.Text = %q, want fallback %q", got, want)
	}
	if cell.WidthCells != 6 {
		t.Fatalf("cell.WidthCells = %d, want 6", cell.WidthCells)
	}
}

func supportedKittyDecision() ImageCapabilityDecision {
	return ImageCapabilityDecision{
		Status:      ImageCapabilityEnabled,
		EnableKitty: true,
		Signals: TerminalImageSignals{
			KittyCompatible: true,
			KittyWindowID:   "42",
			TrueColor:       true,
		},
	}
}

func writeTinyPNG(t *testing.T) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode fixture PNG: %v", err)
	}
	path := filepath.Join(t.TempDir(), "asset.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture PNG: %v", err)
	}
	return path
}
