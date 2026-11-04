package artwork

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// thumbsSubdir keeps variants under <cache>/thumbs.
const thumbsSubdir = "thumbs"

// background fills transparent images so JPEG output has no white edges.
var background = color.RGBA{R: 0x0f, G: 0x11, B: 0x15, A: 0xff}

// ThumbPath returns the cache path of a variant (without checking it exists).
func ThumbPath(cacheDir, src string, maxWidth, quality int) (string, error) {
	st, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d|%d|%d", src, st.ModTime().UnixNano(), st.Size(), maxWidth, quality)))
	return filepath.Join(cacheDir, thumbsSubdir, hex.EncodeToString(sum[:])+".jpg"), nil
}

// Variant returns a resized JPEG for src, generating and caching it on demand.
// It returns src unchanged when no downscale is needed or maxWidth <= 0.
func Variant(cacheDir, src string, maxWidth, quality int) (string, error) {
	if maxWidth <= 0 || strings.TrimSpace(cacheDir) == "" {
		return src, nil
	}
	if quality <= 0 {
		quality = DefaultQuality
	}
	out, err := ThumbPath(cacheDir, src, maxWidth, quality)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	img, err := decode(src)
	if err != nil {
		return "", err
	}
	bounds := img.Bounds()
	if bounds.Dx() <= maxWidth {
		return src, nil // never upscale
	}
	height := bounds.Dy() * maxWidth / bounds.Dx()
	if height < 1 {
		height = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, maxWidth, height))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(out), ".thumb-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if err := jpeg.Encode(tmp, dst, &jpeg.Options{Quality: quality}); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, out); err != nil {
		return "", err
	}
	return out, nil
}

// decode reads jpeg/png/gif/webp from disk.
func decode(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(io.LimitReader(f, 128<<20))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return img, nil
}

// Stats reports the thumbnail cache size.
func Stats(cacheDir string) (files int, bytes int64) {
	if strings.TrimSpace(cacheDir) == "" {
		return 0, 0
	}
	dir := filepath.Join(cacheDir, thumbsSubdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil {
			files++
			bytes += info.Size()
		}
	}
	return files, bytes
}

var clearMu sync.Mutex

// Clear removes every cached variant and reports what was removed. Safe to call
// while requests are serving (they regenerate variants on demand).
func Clear(cacheDir string) (int, int64, error) {
	clearMu.Lock()
	defer clearMu.Unlock()
	if strings.TrimSpace(cacheDir) == "" {
		return 0, 0, nil
	}
	files, bytes := Stats(cacheDir)
	if err := os.RemoveAll(filepath.Join(cacheDir, thumbsSubdir)); err != nil {
		return files, bytes, err
	}
	return files, bytes, nil
}

// Supported reports whether src looks like an image we can decode.
func Supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	}
	return false
}
