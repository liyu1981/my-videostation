# ADR 0004 — Thumbnails and the global cover setting (GAP-006)

Status: accepted · Date: 2026-11-04

## 1. Thumbnails are derived at serve time, never stored rewritten

The art handler (`GET /api/v1/items/{id}/poster|backdrop`) asks
`internal/artwork` for a resized variant and serves that instead of the
original. Variants are keyed by `(source path, source mtime+size, maxWidth,
quality)` and cached under `<data>/cache/thumbs/<sha>.jpg`.

Consequences:

- originals (download cache, sidecar files, media folders) are never modified,
- changing size/quality cannot corrupt anything — the cache is cleared and
  regenerated,
- no migration or background step is required for existing libraries: the first
  request generates the variant,
- a missing/corrupt thumbnail cache degrades to "serve the original", not an
  error.

Alternatives rejected: pre-generating on scan (wastes CPU for art nobody views,
and makes scans slower) and storing variants beside the source (violates the
read-only media rule).

## 2. JPEG output, never upscaled

Sources are decoded with the stdlib decoders plus `golang.org/x/image/webp`
(small, stdlib-adjacent dependency; `golang.org/x/image/draw` provides the
CatmullRom resampler). Output is always JPEG at the configured quality because
posters/backdrops are photographic; transparency is flattened onto the dark
theme background so there are no white edges. Sources narrower than the target
width are returned as-is (`no upscaling`), and `maxWidth = 0` (backdrop "original")
disables the step.

Limitation: poster art with alpha loses its transparency (rare for posters; logos
are not cached). If that ever matters, switch to PNG/WebP per source format.

## 3. Settings live in the global settings table

| Key | Default | Range |
|---|---|---|
| `artwork.thumbnails_enabled` | on | — |
| `artwork.poster_width` | 320 | 120–960 |
| `artwork.backdrop_width` | 1280 | 0 (original) or 640–3840 |
| `artwork.thumbnail_quality` | 82 | 40–95 |
| `artwork.cover_preference` | `provider` | `provider` / `local` |

Invalid stored values fall back to the defaults (a bad row must not break the
settings page or image serving). `PUT /settings` validates user input and clears
the thumbnail cache when width/quality/enabled changes, so orphaned variants do
not accumulate.

## 4. Cover setting semantics

`cover_preference` controls whether provider artwork replaces files found next
to the media:

- `provider` (default): scraped poster/backdrop wins, unless the field is locked
  or the provider has no art. This preserves the historical behaviour.
- `local`: a poster/backdrop that lives beside the media (not in
  `<data>/cache/images`) is kept; provider art fills only what is missing.

Local detection is "path is not under the download cache", which works for NFO
`<thumb>` references, folder art discovered on scan, and user uploads. Locked
fields always win regardless of the setting.

## 5. Rebuild is a job, clear is immediate

`POST /artwork/thumbnails/rebuild` clears the cache and enqueues a `thumbnails`
job (`internal/artwork.RebuildJob`) that walks items with art and pre-generates
both variants, reporting progress through the existing job queue.
`DELETE /artwork/thumbnails` clears synchronously and returns the removed
file count/bytes. The settings page shows the current cache size.

This deliberately stops short of GAP-055 (a single cache-management screen for
artwork downloads, transcodes and subtitle extracts); the thumbnail cache is the
first cache with full size/rebuild/clear controls.
