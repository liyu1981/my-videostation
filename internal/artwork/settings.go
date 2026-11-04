// Package artwork owns the artwork/thumbnail settings and the thumbnail
// generation pipeline (GAP-006).
//
// Thumbnails are derived, never authoritative: originals stay where they are
// (download cache, sidecar file or media folder) and resized JPEG variants live
// under <data>/cache/thumbs. The directory is disposable and can be cleared or
// rebuilt at any time.
package artwork

import (
	"fmt"
	"strconv"
	"strings"
)

// Setting keys (stored in the `settings` table).
const (
	SettingThumbnails      = "artwork.thumbnails_enabled"
	SettingPosterWidth     = "artwork.poster_width"
	SettingBackdropWidth   = "artwork.backdrop_width"
	SettingQuality         = "artwork.thumbnail_quality"
	SettingCoverPreference = "artwork.cover_preference"
)

// Cover preference values.
const (
	// CoverProvider lets scraped provider artwork replace local sidecar art
	// (unless the field is locked). This is the historical behaviour.
	CoverProvider = "provider"
	// CoverLocal keeps poster/backdrop files found next to the media instead of
	// replacing them with provider artwork.
	CoverLocal = "local"
)

// Defaults for the thumbnail pipeline.
const (
	DefaultPosterWidth   = 320
	DefaultBackdropWidth = 1280
	DefaultQuality       = 82
)

// Options is the effective artwork configuration.
type Options struct {
	Thumbnails      bool   `json:"thumbnails"`
	PosterWidth     int    `json:"posterWidth"`
	BackdropWidth   int    `json:"backdropWidth"`
	Quality         int    `json:"quality"`
	CoverPreference string `json:"coverPreference"`
}

// DefaultOptions returns the shipped defaults.
func DefaultOptions() Options {
	return Options{
		Thumbnails:      true,
		PosterWidth:     DefaultPosterWidth,
		BackdropWidth:   DefaultBackdropWidth,
		Quality:         DefaultQuality,
		CoverPreference: CoverProvider,
	}
}

// FromSettings overlays stored settings on the defaults. Invalid stored values
// fall back to the default instead of failing a page load.
func FromSettings(values map[string]string) Options {
	o := DefaultOptions()
	if v, ok := values[SettingThumbnails]; ok {
		o.Thumbnails = v == "1"
	}
	if n, ok := intValue(values[SettingPosterWidth]); ok && validPosterWidth(n) {
		o.PosterWidth = n
	}
	if n, ok := intValue(values[SettingBackdropWidth]); ok && validBackdropWidth(n) {
		o.BackdropWidth = n
	}
	if n, ok := intValue(values[SettingQuality]); ok && validQuality(n) {
		o.Quality = n
	}
	if v, ok := values[SettingCoverPreference]; ok && (v == CoverLocal || v == CoverProvider) {
		o.CoverPreference = v
	}
	return o
}

// Validate checks a user-supplied option set and normalises empty values.
func (o *Options) Validate() error {
	if !validPosterWidth(o.PosterWidth) {
		return fmt.Errorf("poster width must be between 120 and 960")
	}
	if !validBackdropWidth(o.BackdropWidth) {
		return fmt.Errorf("backdrop width must be 0 (original) or between 640 and 3840")
	}
	if !validQuality(o.Quality) {
		return fmt.Errorf("thumbnail quality must be between 40 and 95")
	}
	switch o.CoverPreference {
	case "", CoverProvider:
		o.CoverPreference = CoverProvider
	case CoverLocal:
		o.CoverPreference = CoverLocal
	default:
		return fmt.Errorf("cover preference must be %q or %q", CoverProvider, CoverLocal)
	}
	return nil
}

// SettingsMap renders the options as key/value pairs for the settings store.
func (o Options) SettingsMap() map[string]string {
	enabled := "0"
	if o.Thumbnails {
		enabled = "1"
	}
	return map[string]string{
		SettingThumbnails:      enabled,
		SettingPosterWidth:     strconv.Itoa(o.PosterWidth),
		SettingBackdropWidth:   strconv.Itoa(o.BackdropWidth),
		SettingQuality:         strconv.Itoa(o.Quality),
		SettingCoverPreference: o.CoverPreference,
	}
}

func validPosterWidth(n int) bool   { return n >= 120 && n <= 960 }
func validBackdropWidth(n int) bool { return n == 0 || (n >= 640 && n <= 3840) }
func validQuality(n int) bool       { return n >= 40 && n <= 95 }

func intValue(v string) (int, bool) {
	if strings.TrimSpace(v) == "" {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, false
	}
	return n, true
}

// WidthFor returns the thumbnail max width for an artwork kind.
func WidthFor(o Options, kind string) int {
	if kind == "backdrop" {
		return o.BackdropWidth
	}
	return o.PosterWidth
}
