// Package cover validates still-image uploads and stores them as WebP: a WebP that fits is
// kept as uploaded, anything else is encoded.
package cover

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/webp"
)

const MaxBytes = 8 * 1024 * 1024
const MaxPixels = 16_000_000
const MaxDimension = 8192

var ErrInvalid = errors.New("invalid cover image")

// codecs are the formats an upload may be in, by the name image.Decode gives them.
var codecs = map[string]struct {
	config func(io.Reader) (image.Config, error)
	decode func(io.Reader) (image.Image, error)
}{
	"jpeg": {jpeg.DecodeConfig, jpeg.Decode},
	"png":  {png.DecodeConfig, png.Decode},
	"webp": {webp.DecodeConfig, webp.Decode},
}

func ValidExtension(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp"
}

// CoverSize and BackgroundSize are the longest sides Encode and EncodeBackground keep.
const CoverSize, BackgroundSize = 1600, 1920

// Encode checks a cover upload and returns it as WebP, scaled down to CoverSize. A WebP no
// larger than that keeps its image data untouched.
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
	// The format the name gives is tried first. A file that is not that format (a WebP saved
	// as .png, say) is read as whichever of the three its bytes are.
	named := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if named == "jpg" {
		named = "jpeg"
	}
	var decoded image.Image
	var format string
	for _, format = range []string{named, "jpeg", "png", "webp"} {
		codec := codecs[format]
		cfg, err := codec.config(bytes.NewReader(data))
		if err != nil {
			continue
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxDimension || cfg.Height > MaxDimension || int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
			return nil, ErrInvalid
		}
		if decoded, err = codec.decode(bytes.NewReader(data)); err != nil {
			return nil, ErrInvalid
		}
		break
	}
	if decoded == nil {
		return nil, ErrInvalid
	}
	cfg := decoded.Bounds()
	if format == "webp" && cfg.Dx() <= longest && cfg.Dy() <= longest {
		return stripWebP(data), nil
	}
	// Encode the decoded pixels: no EXIF or arbitrary chunks reach the encoder.
	var normalized bytes.Buffer
	if err := png.Encode(&normalized, decoded); err != nil {
		return nil, fmt.Errorf("normalize cover: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	width, height := cfg.Dx(), cfg.Dy()
	if width > longest || height > longest {
		if width >= height {
			height = max(1, height*longest/width)
			width = longest
		} else {
			width = max(1, width*longest/height)
			height = longest
		}
	}
	cmd := exec.CommandContext(ctx, "cwebp", "-quiet", "-q", "100", "-metadata", "none", "-resize", strconv.Itoa(width), strconv.Itoa(height), "-o", "-", "--", "-")
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

// stripWebP returns a still WebP, which the caller has decoded, without anything but its image:
// the EXIF and XMP a camera or editor wrote, unknown chunks and bytes after the image are
// dropped. The colour profile stays, and the image data is copied as it is.
func stripWebP(data []byte) []byte {
	out := append(make([]byte, 0, len(data)), data[:12]...)
	for at := 12; at+8 <= len(data); {
		id := string(data[at : at+4])
		size := int(binary.LittleEndian.Uint32(data[at+4:]))
		end := at + 8 + size + size&1
		if size < 0 || end > len(data)+size&1 {
			break
		}
		chunk := data[at:min(end, len(data))]
		at = end
		switch id {
		case "VP8X":
			out = append(out, chunk...)
			out[len(out)-len(chunk)+8] &^= 0x08 | 0x04 // the EXIF and XMP flags
		case "ICCP", "ALPH":
			out = append(out, chunk...)
		case "VP8 ", "VP8L":
			out = append(out, chunk...)
			if len(chunk)&1 == 1 {
				out = append(out, 0)
			}
			binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
			return out
		}
	}
	return data
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxBytes {
		return 0, errors.New("encoded cover exceeds limit")
	}
	return b.Buffer.Write(p)
}
