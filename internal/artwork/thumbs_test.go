package artwork

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writeImage(t *testing.T, path string, w, h int, encode func(*os.File, image.Image) error) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 120, A: 255})
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func jpegEncode(w *os.File, img image.Image) error { return jpeg.Encode(w, img, nil) }
func pngEncode(w *os.File, img image.Image) error  { return png.Encode(w, img) }

func decodeSize(t *testing.T, path string) (int, int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img.Bounds().Dx(), img.Bounds().Dy()
}

func TestVariantDownscalesAndCaches(t *testing.T) {
	dir := t.TempDir()
	src := writeImage(t, filepath.Join(dir, "images", "poster.jpg"), 800, 1200, jpegEncode)
	cache := filepath.Join(dir, "cache")

	out, err := Variant(cache, src, 320, 82)
	if err != nil {
		t.Fatal(err)
	}
	if out == src {
		t.Fatal("expected a generated variant")
	}
	if w, h := decodeSize(t, out); w != 320 || h != 480 {
		t.Fatalf("variant size = %dx%d, want 320x480", w, h)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	// Second call reuses the cached file.
	again, err := Variant(cache, src, 320, 82)
	if err != nil {
		t.Fatal(err)
	}
	st2, _ := os.Stat(again)
	if again != out || !st2.ModTime().Equal(st.ModTime()) {
		t.Fatal("variant should be reused from cache")
	}
}

func TestVariantSkipsSmallAndZeroWidth(t *testing.T) {
	dir := t.TempDir()
	src := writeImage(t, filepath.Join(dir, "small.png"), 200, 300, pngEncode)
	cache := filepath.Join(dir, "cache")

	out, err := Variant(cache, src, 320, 82)
	if err != nil {
		t.Fatal(err)
	}
	if out != src {
		t.Fatalf("small image should not be upscaled, got %s", out)
	}
	out, err = Variant(cache, src, 0, 82)
	if err != nil || out != src {
		t.Fatalf("maxWidth=0 should return the source, got %s err=%v", out, err)
	}
}

func TestVariantHandlesTransparency(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 400, 600))
	img.Set(0, 0, color.RGBA{A: 0})
	path := filepath.Join(dir, "alpha.png")
	f, _ := os.Create(path)
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	out, err := Variant(filepath.Join(dir, "cache"), path, 200, 82)
	if err != nil {
		t.Fatal(err)
	}
	// JPEG output must decode (no alpha crash).
	if w, _ := decodeSize(t, out); w != 200 {
		t.Fatalf("width = %d", w)
	}
}

func TestClearAndStats(t *testing.T) {
	dir := t.TempDir()
	src := writeImage(t, filepath.Join(dir, "poster.jpg"), 800, 1200, jpegEncode)
	cache := filepath.Join(dir, "cache")
	if _, err := Variant(cache, src, 320, 82); err != nil {
		t.Fatal(err)
	}
	files, bytes := Stats(cache)
	if files != 1 || bytes == 0 {
		t.Fatalf("stats = %d files, %d bytes", files, bytes)
	}
	removedFiles, removedBytes, err := Clear(cache)
	if err != nil {
		t.Fatal(err)
	}
	if removedFiles != 1 || removedBytes != bytes {
		t.Fatalf("clear = %d/%d, want 1/%d", removedFiles, removedBytes, bytes)
	}
	if files, _ := Stats(cache); files != 0 {
		t.Fatalf("cache not empty: %d files", files)
	}
}

func TestOptionsFromSettingsAndValidate(t *testing.T) {
	o := FromSettings(map[string]string{
		SettingThumbnails:      "0",
		SettingPosterWidth:     "480",
		SettingBackdropWidth:   "1920",
		SettingQuality:         "88",
		SettingCoverPreference: CoverLocal,
	})
	if o.Thumbnails || o.PosterWidth != 480 || o.BackdropWidth != 1920 || o.Quality != 88 || o.CoverPreference != CoverLocal {
		t.Fatalf("options = %+v", o)
	}
	// Bad stored values fall back to defaults.
	o = FromSettings(map[string]string{
		SettingPosterWidth: "12", SettingBackdropWidth: "abc", SettingQuality: "100",
	})
	if o.PosterWidth != DefaultPosterWidth || o.BackdropWidth != DefaultBackdropWidth || o.Quality != DefaultQuality {
		t.Fatalf("fallback options = %+v", o)
	}
	bad := Options{PosterWidth: 5, BackdropWidth: 1280, Quality: 82}
	if err := bad.Validate(); err == nil {
		t.Fatal("bad poster width should fail validation")
	}
	good := Options{Thumbnails: true, PosterWidth: 320, BackdropWidth: 0, Quality: 82}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid options rejected: %v", err)
	}
	if good.CoverPreference != CoverProvider {
		t.Fatalf("empty cover preference should default to provider, got %q", good.CoverPreference)
	}
}

func TestSupported(t *testing.T) {
	for _, p := range []string{"a.jpg", "a.JPEG", "a.png", "a.webp", "a.gif"} {
		if !Supported(p) {
			t.Errorf("Supported(%q) = false", p)
		}
	}
	for _, p := range []string{"a.mkv", "a.txt", ""} {
		if Supported(p) {
			t.Errorf("Supported(%q) = true", p)
		}
	}
}
