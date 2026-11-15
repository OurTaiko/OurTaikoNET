// Package cover validates still-image uploads and stores only normalized WebP bytes.
package cover

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // uploads may be JPEG
	"image/png"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
)

const MaxBytes = 8 * 1024 * 1024
const MaxPixels = 16_000_000
const MaxDimension = 8192

var ErrInvalid = errors.New("invalid cover image")

func ValidExtension(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp"
}

// CoverSize and BackgroundSize are the longest sides Encode and EncodeBackground keep.
const CoverSize, BackgroundSize = 1600, 1920

// Encode checks a cover upload and returns it as WebP, scaled down to CoverSize.
func Encode(ctx context.Context, name string, data []byte) ([]byte, error) {
	return encode(ctx, name, data, CoverSize)
}

// EncodeBackground does the same for a chart's background image (BGIMAGE), which a game shows
// full screen, scaled down to BackgroundSize.
func EncodeBackground(ctx context.Context, name string, data []byte) ([]byte, error) {
	return encode(ctx, name, data, BackgroundSize)
}

func encode(ctx context.Context, name string, data []byte, longest int) ([]byte, error) {
	if !ValidExtension(name) || len(data) == 0 || len(data) > MaxBytes {
		return nil, ErrInvalid
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	expected := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if strings.EqualFold(filepath.Ext(name), ".jpg") {
		expected = "jpeg"
	}
	if err != nil || format != expected || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxDimension || cfg.Height > MaxDimension || int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, ErrInvalid
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalid
	}
	// Re-encode decoded pixels: no EXIF, arbitrary chunks, or animation reach the encoder.
	var normalized bytes.Buffer
	if err := png.Encode(&normalized, decoded); err != nil {
		return nil, fmt.Errorf("normalize cover: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	width, height := cfg.Width, cfg.Height
	if width > longest || height > longest {
		if width >= height {
			height = max(1, height*longest/width)
			width = longest
		} else {
			width = max(1, width*longest/height)
			height = longest
		}
	}
	cmd := exec.CommandContext(ctx, "cwebp", "-quiet", "-q", "85", "-metadata", "none", "-resize", strconv.Itoa(width), strconv.Itoa(height), "-o", "-", "--", "-")
	cmd.Stdin = &normalized
	var out limitedBuffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("encode cover: %w", err)
	}
	data = out.Bytes()
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, errors.New("encoder did not produce WebP")
	}
	return data, nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxBytes {
		return 0, errors.New("encoded cover exceeds limit")
	}
	return b.Buffer.Write(p)
}
