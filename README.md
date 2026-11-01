# MyVideoStation

**A modern, open-source re-creation of Synology Video Station.** One Go binary, SQLite,
and your existing folders — point it at a library and it identifies, organises, and plays
your movies and shows.

> **Status: M0 + M1 implemented.** A single static binary with an embedded React UI can
> index movie / TV / home-video libraries, probe files with ffprobe, group shows into
> seasons and episodes, browse (poster/tile/list), search, edit metadata with per-field
> locks, and direct-play. Metadata providers, users and transcoding are the next milestones.

---

## Why

Synology discontinued Video Station, and newer DSM builds gate HEVC/AAC behind a paid
codec extension tied to a Synology account. Meanwhile the self-hosted alternatives are
either heavy (a plugin platform plus a separate server stack) or stuck in a decade-old UI.

MyVideoStation aims for the opposite:

- **Media management first.** A correct scanner and filename parser, fast search, reliable
  metadata matching with manual override — before any streaming features.
- **Boring to operate.** One static binary, one SQLite file, one data volume, no Redis,
  no Node runtime, no external services except `ffmpeg`.
- **Non-destructive.** Never renames, moves, or deletes media. Sidecar data lives in `/data`.
- **Modern UI.** Dark-first, keyboard-friendly, responsive — inspired by Video Station's
  information architecture, not its 2013 ExtJS chrome.

## Planned features

| Area | Capabilities |
|---|---|
| Libraries | Movies / TV Shows / Home Videos / TV Recordings, multiple paths per library, scheduled + optional live scanning |
| Metadata | TMDB provider (pluggable), poster/backdrop management, manual edit with per-field locks, "needs attention" queue, NFO sidecar support |
| Organisation | Poster grid, tile and list views, categories (recently added / watched / released, by folder, timeline), facets, global search, collections, favorites, watchlist |
| Playback | Direct play with range requests, subtitles (SRT/ASS/VTT), resume + watched state, then HLS remux/transcode with quality selection and optional hardware acceleration |
| People | Multi-user accounts, per-library permissions, parental controls, public share links |
| Ops | Durable job queue with progress, incremental scans, metrics/logs, backup/restore |

Known gaps versus the original (and where we deliberately diverge) are tracked
individually in [`docs/GAPS.md`](./docs/GAPS.md).

## Stack

| Layer | Choice |
|---|---|
| Backend | Go 1.27 · `net/http` · `modernc.org/sqlite` (CGO-free) · `sqlc` · `goose` · FTS5 |
| Media | `ffprobe` for metadata · `ffmpeg` for remux/transcode (HLS fMP4) |
| Frontend | React 19 · TypeScript 7 (`tsgo`) · Vite 8 · TanStack Router/Query/Table · Jotai · Tailwind CSS 4 |
| Packaging | Single static binary with embedded SPA · Alpine image (`amd64` + `arm64`) · Docker Compose |

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  single static binary (CGO off)                              │
│  /api/v1/*   JSON REST        /            embedded SPA      │
│  /media/*    byte-range       /hls/*       transcoded output │
│                                                              │
│  services: library · scanner · metadata · playback · auth     │
│  jobs:     in-process workers + durable `jobs` table (SSE)    │
│  storage:  SQLite (WAL) + /data/cache for art and transcodes  │
└──────────────────────────────────────────────────────────────┘
      media dirs (read-only)               /data (read/write)
```

Planned layout:

```
cmd/mvs/            app entry point
internal/           config · db · store · media · naming · scanner · metadata
                    library · playback · auth · jobs · httpapi · web(embed)
migrations/         embedded goose SQL
sql/                sqlc query files
web/                React app (pnpm workspace)
deployments/        Dockerfile, docker-compose.yml
docs/               PLAN · GAPS · UI-PROBE · ADRs
testdata/           parser corpus and generated sample media
```

## What works today (M1)

- **Libraries**: movie / TV / home-video libraries with multiple read-only folders.
- **Scanning**: incremental (size+mtime), durable jobs with live progress, results streamed
  into the library while scanning, per-file failure isolation, `@eaDir`/`#recycle` skipping,
  orphan cleanup, parallel probing (4 workers), extras indexed hidden and attached to their
  movie/show, samples skipped.
- **Parsing**: scene names, `SxxExx`, `1x02`, multi-episode ranges (`S01E01-E02`), CJK
  titles, editions, folders as fallback — 34-case test corpus.
- **Probing**: ffprobe (container, duration, resolution, codecs, HDR, audio/subtitle tracks);
  graceful degradation when ffprobe is absent.
- **Library UI**: Video-Station-style header (logo block as wide as the rail, home, search,
  user, settings), an inset **"bookshelf" hero** (`<poster> | <bg>`: a full-height poster
  next to a fixed 16:9 backdrop with a left-dark mask, bottom-aligned title/stars/runtime/
  age chip, and a ring-shaped shelf of full-height poster spines; each slide is an
  already-expanded poster + backdrop block, and clicking a spine simply slides that block
  into the poster slot), continue-watching, **fixed-size 120 px poster grid** (posters never stretch to fill the page), thumbnail/tile/list views, category + sort
  dropdowns, pagination, global FTS5 search, show → season → episode navigation.
- **Edit video info (VS parity)**: one overflow-menu entry opens a tabbed dialog
  (Video Info / Poster / Backdrop) with chip editors, per-field locks, artwork
  Original / No images / URL, and a nested TMDb search with a remembered language
  picker.
- **Item UI (VS parity)**: 270×388 poster with 76 px play overlay, unwatched chip + action
  icons, 36 px title, tagline, iconised info line (release date · stars · clock duration ·
  video codec · audio codec) with an age-classification chip, definition rows (genre/cast/
  writer/director/reference/subtitles/audio), summary with "View all", season bar + landscape
  episode cards, prev/next item arrows, **media info dialog** from the top-right info button
  and overflow menu, extras.
- **Player (VS parity)**: custom auto-hiding control bar (prev/next, ±10 s, volume, time,
  subtitle menu, audio-track list, speed, fullscreen), buffered progress bar, big pause
  overlay, next-up prompt, keyboard shortcuts, browser Back closing the player, and a clear
  message when the browser cannot decode the file (transcoding lands in M5).
- **Metadata (M2)**: TMDB matching with confidence scoring, cast/crew/genres/certification,
  artwork caching under `<data>/cache`, a Video-Station-style **Edit video info** dialog
  (tabs, chip editors, nested TMDb search with a remembered language picker; episodes
  search the show and match by season/episode, including stills), a
  "Needs attention" queue, **Select from existing videos** (copy metadata from another
  item in the library when TMDb has no result), refresh that never overwrites locked fields,
  and a merge/keep
  prompt when an edit or match collides with an existing same-title item (movies merge
  files, shows merge seasons/episodes).
- **Local sidecars**: reads Jellyfin/Kodi `.nfo` (movie/tvshow/episode, local art, cast,
  unique ids) and Synology `.vsmeta` (protobuf; embedded poster/backdrop) — **vsmeta wins
  over NFO field-by-field** (Video Station's priority). Imported items are marked `local`;
  TMDb matches and user edits are written to the DB and are never reverted by a rescan.
  Format notes: [`docs/VSMETA-FORMAT.md`](docs/VSMETA-FORMAT.md).
- **Organisation (M3)**: Favorites / Watchlist / custom collections, selection mode with
  bulk watched + add-to-collection, category presets (added / watched / released / A–Z),
  genre/year/watched/folder filters from live facets, per-library auto-scan intervals.
- **Operations**: per-library **Scan** (incremental) and **Rescan** (force re-probe), with
  **outstanding tasks shown inside each library card** (queued/running/failed), plus a full
  task monitor with **persisted logs** — per-file probe/index lines including full paths,
  warn/error entries, copy-all, per-task delete and "clear finished".
- **Playback (M4)**: direct play scaled to fit the window (`object-contain`, any aspect
  ratio), resume, sidecar + embedded subtitle tracks served as WebVTT (UTF-8/GB18030/Big5),
  subtitle menu, keyboard shortcuts, and next-episode prompt.

## Getting started

```bash
pnpm install

# one-time (idempotent): installs air (Go live reload) + static ffprobe into .tools/
pnpm bootstrap

# dev: one server on :6500 — air rebuilds the SPA, embeds it and restarts the binary
pnpm dev

# production: web bundle -> embedded dist -> single static binary
pnpm build && ./bin/mvs --data ./data
```

Editing anything under `cmd/`, `internal/` or `web/src/` triggers a frontend build, a Go build
with the SPA embedded, and a restart — the same single server as production, on `:6500`
(reload the browser tab after a change; there is no separate Vite dev server or HMR).
Migrations are embedded too, so changing SQL also triggers a rebuild.

The dev server binds `0.0.0.0`, so you can open the UI from another device on the same LAN:
`http://<your-lan-ip>:6500`.

| Script | Purpose |
|---|---|
| `pnpm dev` | air watches Go + frontend, rebuilds the embedded single binary |
| `pnpm build` | embed the SPA and produce `bin/mvs` |
| `pnpm test` | Go test suite |
| `pnpm lint` | gofmt check, go vet, `tsc --noEmit` |
| `pnpm typecheck` | TypeScript only |
| `pnpm docker` | build the Alpine image with version metadata |
| `pnpm clean` | remove build artifacts (`pnpm clean -- --data` also drops the DB) |

Open <http://127.0.0.1:6500> (or :5173 in dev), go to **Libraries → Add library**, point it at
a folder, and it creates + starts a scan.

Docker (media mounts are read-only by design):

```bash
MEDIA_MOVIES=/mnt/media/movies MEDIA_TV=/mnt/media/tv \
  docker compose -f deployments/docker-compose.yml up -d --build
```

> **Security:** M1 has no authentication. The `mvs` binary defaults to `127.0.0.1`, but the
> **dev loop binds `0.0.0.0`** so you can test from a TV/phone on your LAN. Run dev only on a
> trusted network, and do not expose either port to the internet until M6 (users/sessions).

Configuration precedence: CLI flags → `MVS_*` env → defaults (a config file lands later).

| Flag | Env | Default |
|---|---|---|
| `--listen` | `MVS_LISTEN` | `127.0.0.1:6500` |
| `--data` | `MVS_DATA_DIR` | `./data` |
| `--db` | `MVS_DB` | `<data>/mvs.db` |
| `--ffprobe` | `MVS_FFPROBE` | auto-detected |
| `--log-level` | `MVS_LOG_LEVEL` | `info` |
| `--log-format` | `MVS_LOG_FORMAT` | `text` |

## Roadmap

| Milestone | Scope |
|---|---|
| **M0** ✅ | Repo scaffold, tokens/theme, single-binary build, Docker, CI |
| **M1** ✅ | Libraries, incremental scanner, filename parser, browse UI, search, media info |
| **M2** ✅ | TMDB matching, art management, identify/correct flows |
| **M3** ✅ | Collections, selection/bulk actions, categories/facets, scheduler, task monitor |
| **M4** ✅ | Player: subtitle tracks, resume, next episode |
| **M5** | HLS remux/transcode, audio/subtitle switching, optional HW acceleration |
| **M6** | Users, permissions, parental controls, share links, i18n |
| **M7** | PWA, casting, trailers, offline conversion, metrics |

Some M4 groundwork already ships: byte-range streaming, progress reporting and a minimal
direct-play modal (see [ADR 0001](docs/adr/0001-m0-m1-decisions.md)).

Details and acceptance criteria: [`docs/PLAN.md`](./docs/PLAN.md) §8.

## Docs

- [`docs/PLAN.md`](./docs/PLAN.md) — vision, architecture, data model, API, phases, decisions
- [`docs/GAPS.md`](./docs/GAPS.md) — feature-parity register vs. Synology Video Station
- [`docs/UI-PROBE.md`](./docs/UI-PROBE.md) — verified UI measurements from a live 3.0.7 instance
- [`docs/VSMETA-FORMAT.md`](./docs/VSMETA-FORMAT.md) — reverse-engineered `.vsmeta` protobuf layout
- [`docs/adr/0001-m0-m1-decisions.md`](./docs/adr/0001-m0-m1-decisions.md) — M0/M1 decisions and deviations
- [`AGENTS.md`](./AGENTS.md) — working rules for contributors and coding agents

## Non-goals (v1)

Live TV/DVR, DLNA/UPnP serving, native mobile/TV apps, a plugin marketplace, cloud sync,
and zero-config remote access (use Tailscale/Cloudflare Tunnel). Workarounds for each are
documented alongside the gap entries.

## License

Not chosen yet — tracked as an open decision in [`docs/PLAN.md`](./docs/PLAN.md) §12.
Until decided, treat the repository as all-rights-reserved.

## Acknowledgements

Design vocabulary is inspired by Synology Video Station; no Synology code, assets, fonts,
or icons are used. Icons come from [Lucide](https://lucide.dev) (ISC); fonts will be
Apache/OFL-licensed (Inter/Roboto).
