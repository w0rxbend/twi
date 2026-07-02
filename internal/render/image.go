package render

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/w0rxbend/twi/internal/storage"
)

const kittyPNGFormat = 100

var (
	// ErrImageUnsupported reports that the terminal or config state should use
	// the already-reserved text fallback instead of inline image output.
	ErrImageUnsupported = errors.New("image renderer unsupported")
	// ErrImageRenderFailed reports that an otherwise supported renderer could
	// not produce terminal image output for a cached asset.
	ErrImageRenderFailed = errors.New("image render failed")
)

// KittyRenderer renders prepared local PNG assets with the Kitty graphics
// protocol. It is intended for asynchronous callers; View paths should render
// stable fallback fragments until a cell has been prepared.
type KittyRenderer struct {
	Decision ImageCapabilityDecision
}

var _ ImageRenderer = (*KittyRenderer)(nil)

// NewKittyRenderer creates a Kitty-compatible renderer from the resolved image
// capability state shared by app startup and diagnostics.
func NewKittyRenderer(decision ImageCapabilityDecision) *KittyRenderer {
	return &KittyRenderer{Decision: decision}
}

// RenderImage returns terminal output for one cached image while preserving the
// requested layout width on every error path.
func (r *KittyRenderer) RenderImage(ctx context.Context, asset storage.AssetRecord, spec ImageSpec) (ImageCell, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cell := fallbackImageCell(asset, spec)
	if err := ctx.Err(); err != nil {
		return cell, err
	}
	if r == nil || !r.supported() {
		return cell, ErrImageUnsupported
	}

	format, ok := kittyImageFormat(asset)
	if !ok {
		return cell, fmt.Errorf("%w: unsupported cached image media type", ErrImageRenderFailed)
	}
	path := strings.TrimSpace(asset.Path)
	if path == "" {
		return cell, fmt.Errorf("%w: missing cached image file", ErrImageRenderFailed)
	}
	info, err := os.Stat(path)
	if err != nil {
		return cell, fmt.Errorf("%w: cached image file is unavailable", ErrImageRenderFailed)
	}
	if info.IsDir() {
		return cell, fmt.Errorf("%w: cached image path is a directory", ErrImageRenderFailed)
	}
	file, err := os.Open(path)
	if err != nil {
		return cell, fmt.Errorf("%w: cached image file is unreadable", ErrImageRenderFailed)
	}
	if err := file.Close(); err != nil {
		return cell, fmt.Errorf("%w: cached image file close failed", ErrImageRenderFailed)
	}
	if err := ctx.Err(); err != nil {
		return cell, err
	}

	width := cell.WidthCells
	height := positiveFirst(spec.HeightCells, asset.HeightCells, 1)
	encodedPath := base64.StdEncoding.EncodeToString([]byte(path))
	escape := fmt.Sprintf(
		"\x1b_Ga=T,f=%d,t=f,q=2,C=1,i=%d,c=%d,r=%d;%s\x1b\\",
		format,
		kittyImageID(asset),
		width,
		height,
		encodedPath,
	)
	cell.Text = escape + strings.Repeat(" ", width)
	return cell, nil
}

func (r *KittyRenderer) supported() bool {
	decision := r.Decision
	if !decision.EnableKitty {
		return false
	}
	if !decision.Signals.KittyCompatible {
		return false
	}
	switch decision.Status {
	case ImageCapabilityEnabled, ImageCapabilityDegraded:
		return true
	default:
		return false
	}
}

func fallbackImageCell(asset storage.AssetRecord, spec ImageSpec) ImageCell {
	width := positiveFirst(spec.WidthCells, asset.WidthCells, textWidth(spec.Fallback), 1)
	return ImageCell{
		Text:       fitCells(spec.Fallback, width),
		WidthCells: width,
	}
}

func kittyImageFormat(asset storage.AssetRecord) (int, bool) {
	mediaType := strings.ToLower(strings.TrimSpace(asset.MediaType))
	if semicolon := strings.IndexByte(mediaType, ';'); semicolon >= 0 {
		mediaType = strings.TrimSpace(mediaType[:semicolon])
	}
	switch mediaType {
	case "", "image/png", "application/png":
		if mediaType == "" && strings.ToLower(filepath.Ext(asset.Path)) != ".png" {
			return 0, false
		}
		return kittyPNGFormat, true
	default:
		return 0, false
	}
}

func kittyImageID(asset storage.AssetRecord) uint32 {
	input := asset.Key.Kind + "\x00" + asset.Key.ID + "\x00" + asset.Path
	sum := sha256.Sum256([]byte(input))
	id := binary.BigEndian.Uint32(sum[:4])
	if id == 0 {
		return 1
	}
	return id
}

func positiveFirst(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
