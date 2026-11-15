package cover

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncode(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2000, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 2000; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 80, G: 160, B: 220, A: 180})
		}
	}
	for _, extension := range []string{"jpg", "jpeg", "JPEG", "PNG", "webp", "WEBP"} {
		t.Run(extension, func(t *testing.T) {
			isJPEG := extension == "jpg" || strings.EqualFold(extension, "jpeg")
			var source bytes.Buffer
			if isJPEG {
				if err := jpeg.Encode(&source, img, nil); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := png.Encode(&source, img); err != nil {
					t.Fatal(err)
				}
			}
			if extension == "webp" || extension == "WEBP" {
				args := []string{"-quiet", "-o", "-", "--", "-"}
				if extension == "WEBP" {
					args = append([]string{"-lossless"}, args...)
				}
				cmd := exec.Command("cwebp", args...)
				cmd.Stdin = bytes.NewReader(source.Bytes())
				encoded, err := cmd.Output()
				if err != nil {
					t.Fatal(err)
				}
				source.Reset()
				source.Write(encoded)
			}
			webp, err := Encode(context.Background(), "cover."+extension, source.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			// Decode using the independent libwebp decoder, checking resize and transparency.
			cmd := exec.Command("dwebp", "-quiet", "-o", "-", "--", "-")
			cmd.Stdin = bytes.NewReader(webp)
			out, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := png.Decode(bytes.NewReader(out))
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Bounds().Dx() != 1600 || decoded.Bounds().Dy() != 800 {
				t.Fatal(decoded.Bounds())
			}
			_, _, _, alpha := decoded.At(100, 100).RGBA()
			if !isJPEG && alpha >= 65535 {
				t.Fatal("transparency lost")
			}
		})
	}
}

func TestRejectInvalidImages(t *testing.T) {
	var pngData bytes.Buffer
	_ = png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 10, 10)))
	var tooWide bytes.Buffer
	_ = png.Encode(&tooWide, image.NewNRGBA(image.Rect(0, 0, 8193, 1)))
	var tooMany bytes.Buffer
	_ = png.Encode(&tooMany, image.NewNRGBA(image.Rect(0, 0, 4001, 4000)))
	webpData, err := Encode(context.Background(), "cover.png", pngData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"cover.svg", []byte("<svg/>")}, {"cover.svg", pngData.Bytes()},
		{"cover.png", []byte("invalid")}, {"cover.png", nil},
		{"cover.png", pngData.Bytes()[:30]}, {"cover.png", tooWide.Bytes()}, {"cover.png", tooMany.Bytes()},
		{"cover.png", make([]byte, MaxBytes+1)},
		{"cover.webp", []byte("invalid")}, {"cover.webp", nil},
		{"cover.webp", webpData[:len(webpData)-10]}, {"cover.png", webpData[:len(webpData)-10]},
		{"cover.webp", make([]byte, MaxBytes+1)},
	} {
		if _, err := Encode(context.Background(), tc.name, tc.data); !errors.Is(err, ErrInvalid) {
			t.Errorf("accepted %s (%d bytes): %v", tc.name, len(tc.data), err)
		}
	}
}

// A file whose name gives another format than its bytes are is read by its bytes.
func TestEncodeMisnamedImages(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 20, 10))
	var pngData, jpegData bytes.Buffer
	_ = png.Encode(&pngData, img)
	_ = jpeg.Encode(&jpegData, img, nil)
	webpData, err := Encode(context.Background(), "cover.png", pngData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"cover.webp", pngData.Bytes()}, {"cover.jpeg", pngData.Bytes()}, {"cover.jpg", pngData.Bytes()},
		{"cover.png", webpData}, {"cover.jpg", webpData}, {"cover.png", jpegData.Bytes()}, {"cover.webp", jpegData.Bytes()},
	} {
		out, err := Encode(context.Background(), tc.name, tc.data)
		if err != nil {
			t.Errorf("rejected %s (%d bytes): %v", tc.name, len(tc.data), err)
			continue
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(out))
		if err != nil || format != "webp" || cfg.Width != 20 || cfg.Height != 10 {
			t.Errorf("%s became %s %dx%d (%v)", tc.name, format, cfg.Width, cfg.Height, err)
		}
	}
}

func TestEncodeBackgroundKeepsFullHD(t *testing.T) {
	for _, size := range []struct{ w, h, wantW, wantH int }{{1920, 1080, 1920, 1080}, {3840, 2160, 1920, 1080}} {
		img := image.NewNRGBA(image.Rect(0, 0, size.w, size.h))
		var source bytes.Buffer
		if err := png.Encode(&source, img); err != nil {
			t.Fatal(err)
		}
		webp, err := EncodeBackground(context.Background(), "bg.png", source.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(webp))
		if err != nil || cfg.Width != size.wantW || cfg.Height != size.wantH {
			t.Fatalf("%dx%d became %dx%d (%v)", size.w, size.h, cfg.Width, cfg.Height, err)
		}
	}
}

// A WebP that fits is stored as uploaded, without its metadata; a larger one is scaled.
func TestEncodeKeepsWebP(t *testing.T) {
	webpOf := func(width, height int, args ...string) []byte {
		t.Helper()
		img := image.NewNRGBA(image.Rect(0, 0, width, height))
		for i := range img.Pix {
			img.Pix[i] = uint8(i * 7)
		}
		var source bytes.Buffer
		_ = png.Encode(&source, img)
		cmd := exec.Command("cwebp", append(args, "-quiet", "-o", "-", "--", "-")...)
		cmd.Stdin = &source
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	for name, source := range map[string][]byte{
		"lossy with alpha": webpOf(64, 32, "-q", "40"), "lossless": webpOf(64, 32, "-lossless"),
		"largest": webpOf(CoverSize, CoverSize, "-q", "1"),
	} {
		for _, file := range []string{"cover.webp", "cover.png"} {
			if out, err := Encode(context.Background(), file, source); err != nil || !bytes.Equal(out, source) {
				t.Errorf("%s as %s: changed (%d to %d bytes, %v)", name, file, len(source), len(out), err)
			}
		}
	}

	// EXIF, XMP and anything unknown are dropped, their flags cleared; the rest is byte for byte.
	clean := webpOf(64, 32, "-q", "40")
	if string(clean[12:16]) != "VP8X" {
		t.Fatal("expected an extended WebP")
	}
	tagged := append([]byte{}, clean...)
	tagged[20] |= 0x08 | 0x04
	for _, id := range []string{"EXIF", "XMP ", "JUNK"} {
		tagged = append(tagged, id+"\x05\x00\x00\x00where\x00"...)
	}
	binary.LittleEndian.PutUint32(tagged[4:], uint32(len(tagged)-8))
	if out, err := Encode(context.Background(), "cover.webp", tagged); err != nil || !bytes.Equal(out, clean) {
		t.Errorf("metadata kept: %d bytes became %d, want %d (%v)", len(tagged), len(out), len(clean), err)
	}

	big, err := Encode(context.Background(), "cover.webp", webpOf(CoverSize+400, 500, "-q", "40"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(big)); err != nil || cfg.Width != CoverSize || cfg.Height != 400 {
		t.Errorf("large WebP became %dx%d (%v)", cfg.Width, cfg.Height, err)
	}

	// An animation is not a cover.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.webp"), webpOf(16, 16, "-q", "10"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.webp"), webpOf(16, 16, "-q", "90"), 0o600); err != nil {
		t.Fatal(err)
	}
	animated := filepath.Join(dir, "animated.webp")
	if out, err := exec.Command("img2webp", "-o", animated, filepath.Join(dir, "a.webp"), filepath.Join(dir, "b.webp")).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	data, _ := os.ReadFile(animated)
	if !bytes.Contains(data, []byte("ANMF")) {
		t.Fatal("img2webp did not animate")
	}
	if _, err := Encode(context.Background(), "cover.webp", data); !errors.Is(err, ErrInvalid) {
		t.Errorf("accepted an animation: %v", err)
	}
}
