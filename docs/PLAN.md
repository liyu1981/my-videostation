# MyVideoStation — Project Plan

> An open-source, modern re-creation of Synology Video Station.
> Single Go binary (embedded React SPA) + SQLite. Media-management first.

Status: **draft / awaiting decisions** (see §12)

Companion docs:
[`GAPS.md`](./GAPS.md) — feature-parity register vs Synology Video Station (44 known gaps,
6 of them S1), including deliberate non-goals and the advantages we hold.
[`UI-PROBE.md`](./UI-PROBE.md) — verified probe of a live Video Station 3.0.7 instance
(42 screenshots): shell geometry, colour/type/layout tokens, component specs, verbatim
labels, field lists, and a parity checklist for M1–M3.

---

## 1. Vision & positioning

Synology's Video Station was a folder-based media library with metadata scraping,
a web player, transcoding, cast support and multi-user watch history. It was
discontinued (codec licensing), leaving users with no like-for-like replacement
that is self-hosted, simple to deploy, and pleasant to use.

MyVideoStation aims at:

- **Drop-in mental model**: point it at folders → it identifies movies/shows →
  you browse posters and play. No plugin ecosystem required to get started.
- **Correct media management** (the hard part): robust scanner, filename parsing,
  provider matching with manual override, non-destructive storage, fast search.
- **Modern UX**: dark-first cinematic UI, instant filtering, keyboard-driven,
  mobile-friendly PWA. Not a 2013 DSM window.
- **Operational simplicity**: one static binary, one SQLite file, one volume.
  Runs on Alpine, a NAS, a Raspberry Pi, or a laptop.

Non-goals for v1: live TV/DVR, DLNA/UPnP server, native mobile/TV apps,
plugin marketplace, cloud sync, distributed multi-node.

---

## 2. Feature map (Video Station core → MyVideoStation)

| Video Station capability | Our equivalent | Phase |
|---|---|---|
| Multiple libraries (Movies / TV / Home video / TV recording) | `libraries` with type + multiple paths | M1 |
| Scheduled folder indexing | Incremental scanner + persisted job queue + scheduler | M1 |
| Online metadata scrape (posters, fanart, cast, plot) | Provider layer (TMDB primary, TVDB optional) | M2 |
| TV hierarchy Season/Episode, next-up | `shows → seasons → episodes`, auto-advance | M2/M3 |
| Home videos / personal videos | `kind=home_video`, minimal metadata, date-sorted | M3 |
| Web playback (HTML5 + transcode for unsupported codecs) | Direct play + HLS transcode on demand | M4/M5 |
| Subtitles (external + embedded), online subtitle search | Sidecar/embedded tracks, OpenSubtitles later | M4/M6 |
| Resume playback / watched state | Central `playback_progress` per user | M4 |
| Collections, genres, favorites, recently added | Collections + genre/favorite/recent browse | M2/M3 |
| Search | FTS5 index over titles/overview/cast | M1/M2 |
| Filters & sorting | Server-side filter/sort + URL state | M3 |
| Manual "Identify" / edit metadata | Match override + field-level locks | M2 |
| User accounts, per-library permissions, parental control | Users/roles/permissions, rating ceilings | M6 |
| Public share links | Signed expiring share links | M6 |
| Chromecast / AirPlay / DLNA | Web cast (Chrome), AirPlay later; DLNA non-goal | M7 |
| Hardware transcoding | VA-API / QSV / NVENC opt-in | M5/M7 |
| Mobile apps | Responsive PWA | M3/M7 |

---

## 3. Architecture

```
┌───────────────────────────────────────────────────────────────┐
│  single static binary (CGO_ENABLED=0, linux/amd64+arm64)      │
│                                                               │
│  net/http  ── /api/v1/*   JSON REST                           │
│            ── /          embedded SPA (go:embed web/dist)     │
│            ── /media/*   byte-range streaming                 │
│            ── /hls/*     transcoded segments (later)          │
│                                                               │
│  services: library · scanner · metadata · playback · auth     │
│  jobs:     in-process workers + durable `jobs` table          │
│  data:     SQLite (WAL) + /data/cache for images & transcodes │
└───────────────────────────────────────────────────────────────┘
```

Principles:

1. **Media directories are read-only by default.** We never rename/move/delete
   user files unless explicitly enabled. All derived data lives in `/data`.
2. **All long work is a job.** Scan, identify, art download, transcode → rows in
   `jobs`, progress over SSE, resumable after restart.
3. **Files are dumb, identity is ours.** A file row is identified by path;
   `media_items` are logical entities that survive file moves (re-link UI later).
4. **Everything is incremental.** Stat-based change detection (size+mtime+inode),
   no full re-probe unless requested.
5. **Deterministic core, pluggable edges.** Parser and matcher are pure Go and
   unit-tested with a filename corpus; providers/searchers behind interfaces.

### 3.1 Repo layout

```
my-video-station/
├── cmd/mvs/                 # main, flag/config wiring
├── internal/
│   ├── config/              # env > file > defaults
│   ├── db/                  # open, pragmas, embedded migrations
│   ├── store/               # sqlc-generated queries + repositories
│   ├── media/               # ffprobe wrapper, stream model
│   ├── naming/              # filename/folder parser (pure, heavily tested)
│   ├── scanner/             # walker, change detection, job orchestration
│   ├── metadata/            # tmdb, tvdb, nfo providers + matcher
│   ├── library/             # domain services (browse, edit, collections)
│   ├── playback/            # direct play; transcode/HLS later
│   ├── auth/                # users, sessions, permissions
│   ├── jobs/                # queue, workers, SSE progress bus
│   ├── httpapi/             # routes, middleware, DTOs, error mapping
│   └── web/                 # go:embed dist
├── web/                     # pnpm workspace (single app)
│   └── src/{routes,features,components,lib,atoms}
├── migrations/              # SQL up/down (goose, embedded)
├── sql/                     # sqlc query files
├── deployments/             # Dockerfile, docker-compose.yml
├── docs/                    # ADRs, filename conventions, API notes
├── testdata/                # media corpus for parser/probe tests
├── package.json         # pnpm scripts: dev/build/test/lint/docker
└── PLAN.md
```

### 3.2 Backend stack decisions (proposed)

| Concern | Choice | Why |
|---|---|---|
| HTTP | Go 1.27 `net/http` ServeMux + thin middleware | no framework needed; easy SSE/range |
| SQLite driver | `modernc.org/sqlite` | CGO-free → true static binary on Alpine |
| Queries | `sqlc` (typed, compile-checked) | avoids ORM surprises; fast |
| Migrations | `goose` embedded via `embed.FS` | simple up/down, single binary |
| Search | SQLite **FTS5** (external-content tables) | zero extra infra |
| Probing | `ffprobe -print_format json` | only reliable codec/stream source |
| Transcoding | `ffmpeg` HLS fMP4, on demand | M5 |
| Jobs | DB-backed queue + worker pool + SSE | durable, no redis |
| Logging | `log/slog` JSON | stdlib |
| Tests | stdlib + `httptest`, golden files for parser | |

### 3.3 Frontend stack

- **React 19** + **TypeScript 7** (`tsgo` for typecheck; Vite transpiles only)
- **Vite 8** (Rolldown), `@tailwindcss/vite` v4 (CSS-first `@theme`)
- **TanStack**: Router (file-based routes) · Query (server state) · Table (admin/scan lists)
- **Jotai** for ephemeral UI state (player, filters draft, dialogs)
- shadcn-style components on **Radix primitives** + `cva` + Tailwind tokens
- `ky` or `fetch` wrapper + generated types from the Go API (single source: Go structs
  or a hand-maintained `types.ts` kept honest by contract tests)
- PWA manifest + service worker (M7)

**Dev loop**: `pnpm dev` → air watches Go/SQL and `web/src`, runs `scripts/dev-build.mjs`
(Vite bundle → `internal/web/dist` → `go build -tags embedui`) and restarts the single binary
on :6500. No separate frontend server or proxy; dev and production serve the app the same way.

---

## 4. Data model (first draft)

Logical items use **one table with self-referencing parents** so search,
collections and watch state work uniformly.

```
users            (id, username, password_hash, role, rating_ceiling, created_at)
sessions         (id, user_id, expires_at, user_agent, ip)
libraries        (id, name, type[movie|tv|home_video], language, scanner_opts_json,
                  metadata_provider, created_at)
library_paths    (id, library_id, path, recursive, last_scan_at)
scan_runs        (id, library_id, started_at, finished_at, status, stats_json)

media_items      (id, library_id, parent_id NULL, kind[movie|show|season|episode|
                  home_video|extra], title, sort_title, original_title, year,
                  overview, tagline, runtime_ms, rating, content_rating,
                  poster_path, backdrop_path, provider, provider_id,
                  season_number, episode_number, air_date, absolute_number,
                  added_at, updated_at, metadata_locked, hidden)
media_files      (id, item_id, path UNIQUE, size, mtime, inode, container,
                  duration_ms, bitrate, width, height, video_codec, hdr,
                  audio_streams_json, subtitle_streams_json, probed_at, missing)
item_people      (item_id, person_id, role[cast|director|writer|producer],
                  character, ordering)
people           (id, name, tmdb_id, profile_path)
genres           (id, name) · item_genres(item_id, genre_id)
collections      (id, name, sort_name, overview, poster_path)
item_collections (collection_id, item_id)
tags             (id, name) · item_tags(item_id, tag_id)
playback_progress(user_id, item_id, position_ms, duration_ms, played, updated_at)
favorites        (user_id, item_id)
library_perms    (user_id, library_id, can_view)
jobs             (id, type, payload_json, status, progress, message, created_at,
                  started_at, finished_at, error)
settings         (key, value_json)      -- app + per-library options
media_search     (FTS5: title, original_title, overview, people_names)
```

Notes:
- `media_items.parent_id` + `kind` gives show/season/episode without joins across
  three tables; `episodes` are searchable directly.
- `metadata_locked` (plus optional per-field locks later) means manual edits and
  manual matches are never overwritten by a re-scan.
- `media_files.missing` flags orphaned files instead of deleting history/progress.
- Multi-episode files: one `media_files` row attached to the first episode, with
  `episode_end_number` on the item (decide in M3).
- Art is stored content-addressed in `/data/cache/images/<sha>` and referenced by
  path; originals in media folders are never touched.

---

## 5. Scan & identification pipeline

```
walk (skip @eaDir, #recycle, .hidden, sample/ extras rules)
  → classify file (video ext, sample/trailer/extra heuristics)
  → change detect (size+mtime+inode)
  → probe (ffprobe) → technical metadata
  → parse path → candidate {kind, title, year, season, episode, edition}
  → group into items (show/season/episode or movie)
  → match provider (only for new/unmatched/locked=false items)
  → fetch art + details → cache
  → update FTS index
```

**Parser** (pure package, golden-file tested):
- Movie: `Title (Year)/Title (Year).mkv`, `Title.2023.1080p.WEB-DL...`
- TV: `Show/Season 01/Show - S01E02 - Title.mkv`, `Show (2019)/S01E02`, `1x02`,
  `Show - 01x02`, anime absolute numbering (later)
- Extras: `Extras/`, `Featurettes/`, `-trailer`, `-sample`, `behindthescenes`
- Cleanup of release tags (`1080p, 2160p, x265, HEVC, WEB-DL, DDP5.1, REPACK, PROPER`)
- Multi-part/multi-episode ranges (`S01E01-E02`)

**Matcher**: normalize + year proximity + popularity tiebreak; confidence score.
Below threshold → item stays `unmatched` and appears in a "Needs attention" queue
with a manual Identify dialog (search provider by title/year and pick).

**Providers**: TMDB primary (movies+TV), optional TVDB for anime/absolute ordering,
local `.nfo` reader (Kodi/Jellyfin compatible). Art from provider, or local
`poster.jpg`/`fanart.jpg`/`season01-poster.jpg` if present.

---

## 6. API sketch (`/api/v1`)

```
POST   /auth/login | /auth/logout | GET /auth/me
GET    /libraries                         POST /libraries
PATCH  /libraries/{id}                    DELETE /libraries/{id}
POST   /libraries/{id}/scan               (job)
GET    /libraries/{id}/items?sort=&filter=&cursor=
GET    /items/{id}                        PATCH /items/{id}   (edit/lock)
GET    /items/{id}/children               GET /items/{id}/related
POST   /items/{id}/identify               POST /items/{id}/refresh
POST   /items/{id}/art                     (choose art)
GET    /search?q=                          (FTS)
GET    /home                               (rows: continue watching, recent, ...)
GET    /media/{fileId}/stream              (Range, direct play)
GET    /media/{fileId}/subtitles/{index}
PUT    /progress/{itemId}                  POST /items/{id}/played
GET    /jobs  /jobs/{id}  /events (SSE)    GET /settings  PATCH /settings
```

Conventions: JSON, camelCase, cursor pagination, RFC7807-ish problem objects,
`ETag`/`If-None-Match` on list endpoints later.

---

## 7. Frontend structure

```
routes/
  __root.tsx                 # shell: sidebar/topbar, session guard
  index.tsx                  # Home: continue watching, recently added, collections
  movies.tsx  tv.tsx  home-videos.tsx
  search.tsx
  item/$id.tsx               # movie/episode detail + play + edit
  show/$id.tsx               # seasons/episodes
  collection/$id.tsx
  settings/{libraries,metadata,transcoding,users}.tsx
  admin/scan.tsx             # job monitor
features/  browse/ detail/ player/ identify/ libraries/ scan/
components/ poster-card, poster-grid, media-row, filters, player, …
lib/       api client, query keys, formatters, i18n
atoms/     player, ui preferences (grid density, theme)
```

UX direction: dark cinematic default, large poster grids, sticky filter bar,
keyboard shortcuts (`/` search, `f` fullscreen, `space` play), skeleton states,
optimistic watch-state updates, URL-as-state for filters.

**Design parity rules** (from `UI-PROBE.md`):
- Keep Video Station's *information architecture and vocabulary* (Home → category →
  library → item; Movies / TV Shows / Home Videos / Recordings; Playlist → Favorites /
  Watchlist / Shared; Watchlist, Collection, "Change video info"), so migrating users
  recognise it immediately.
- Keep the watch-state language: 3 px progress line, amber "new", teal "in progress",
  red "active/selected/recording" — but red never means hover.
- Keep **three view modes** (thumbnail/tile/list), the category menu (Recently Added /
  Watched / Released / All / By Folder / Timeline), and selection mode with a swapped
  toolbar (bulk edit is a real VS workflow).
- Keep the detail anatomy: poster + play overlay + unwatched chip + four actions
  (favorite/watchlist/playlist/share) + media badges + certification + cast/writer/
  director + reference links + subtitle/audio dropdowns + summary with "View all".
- Media info is split Overview / Video / Audio (per-track audio); metadata editing has
  Info / Poster / Backdrop tabs. VS locks at item level — we lock per field, which is a
  strict superset, so no regression.
- Replace ExtJS chrome and PNG sprites with Tailwind tokens + Lucide SVG; add focus rings,
  ≥13 px secondary text, and a comfortable/compact density toggle.
- Icons: 24 px grid, 1 px strokes, `currentColor`; media-type glyphs may be two-tone.
- Never copy Synology CSS, sprites, fonts or the logo.

---

## 8. Delivery phases

Each phase closes a defined slice of the gap register in [`GAPS.md`](./GAPS.md);
`GAP-*` / `ADV-*` IDs are the shared vocabulary between the two documents.

### M0 — Foundation (skeleton that builds & ships)
- Repo scaffolding, pnpm scripts, `.gitignore`, gofmt/vet/tsc checks, CI
- `theme.css` Tailwind tokens + Lucide icon map derived from `docs/UI-PROBE.md` §7
- Single binary serving embedded "hello" SPA; config loader; slog; `/healthz`
- SQLite open with pragmas (WAL, busy_timeout, foreign_keys); migration runner
- Dockerfile (multi-stage → alpine + ffmpeg + tini, non-root), compose, volume layout
- **Done when**: `pnpm build` yields one binary; `docker compose up` serves UI+API.

### M1 — Media management core (the milestone that matters)
- Libraries CRUD with multiple paths; library types
- Scanner: walk, ignore rules, change detection, probe, grouping, jobs + progress
- Filename parser with corpus tests (≥40 real-world patterns)
- DB schema + migrations + queries + FTS5
- Browse API + UI: grid (thumbnail/tile/list), sort/filter, category menu, pagination,
  item counts, global search, empty states, item detail with technical info
- Media info panel (Overview / Video / Audio, per-track audio) fed by ffprobe output
- Manual metadata edit (title/year/overview/genres/poster) with per-field locks
- **Done when**: 100-file library indexes correctly; re-scan is incremental
  (<1s for unchanged files); titles/seasons/episodes are right; search is instant.

### M2 — Metadata enrichment
- TMDB provider (movies/TV), art caching, cast/crew, collections, genres
- Matcher + confidence + "Needs attention" queue + Identify dialog (VS parity:
  "Search from Video Info Plugin" / "Select from existing", plus a separate
  "Correct video info" entry point)
- Poster/backdrop picker (provider art, URL, from media folder) + NFO reader + local art
- System collections first (Favorites, Watchlist, Shared), then user collections
- Home page rows (recently added, collections, genres)
- **Done when**: typical library auto-matches ≥90%; unmatched are obvious and fixable.

### M3 — Organization & library UX
- Show → season → episode navigation, next-up, specials
- Collections, favorites, watched/unwatched, tags, home videos by date
- Bulk edit, identify-many, hidden items, library permissions groundwork
- Scheduler (interval/daily) + optional fsnotify live watch + job monitor UI
- **Done when**: library of 5k files is navigable and filterable without lag.

### M4 — Playback (management loop closes)
- Direct-play streaming with Range; subtitle conversion (srt/ass → vtt)
- Player: audio/subtitle track pick, resume, mark watched, next episode
- Per-user progress; "Continue watching" row
- **Done when**: H.264/AAC/WebM direct-play flawlessly with resume + subtitles.

### M5 — Transcoding
- ffmpeg HLS fMP4 sessions on demand with quality ladder; remux-first
- Segment cleanup, session limits, in-process transcode progress
- Optional HW accel (VA-API/QSV/NVENC) autodetect; subtitle burn-in
- **Done when**: HEVC/AC3/DTS titles play in browser with seek.

### M6 — Users, sharing, i18n
- Accounts, roles, per-library permissions, parental rating ceilings
- Share links with expiry, optional passcode; watch history per user
- i18n (en + de/zh scaffold), OG metadata for shared links

### M7 — Polish / optional
- PWA/offline shell, Chromecast, online subtitle search, trailers,
  duplicate detection, file-move re-link, Prometheus metrics, ARM builds

---

## 9. Deployment

**Image**: `node:24-alpine` (web build) → `golang:1.27-alpine` (CGO off) →
`alpine:3.23` runtime with `ffmpeg`, `tzdata`, `tini`, `ca-certificates`.
Non-root `mvs` user, `VOLUME /data`, media mounted read-only, `HEALTHCHECK`.

**Volumes / dirs**
```
/config  or /data
  mvs.db  (+ -wal, -shm)
  config.yaml (generated; env overrides)
  cache/images/…   cache/transcodes/…   logs/
/movies  /tv  /home-videos   (read-only mounts)
```

**Config precedence**: CLI flags > `MVS_*` env > `/data/config.yaml` > defaults.

**Extras**: docker-compose example, PUID/PGID support, healthcheck, goreleaser for
linux/amd64+arm64 binaries, GH Actions (lint, test, build, docker multi-arch).

---

## 10. Testing & quality

- Go: table-driven tests; golden files for the naming parser; `httptest` API tests
  against a temp SQLite; fuzz test for the parser (fuzz target with `go test -fuzz`).
- A tiny synthetic media corpus generated by ffmpeg in CI (not committed binaries).
- Frontend: Vitest for logic/hooks; Playwright smoke (browse → detail → play) in M4.
- Contract: OpenAPI (or zod schemas from a single source) checked in CI to prevent
  frontend/backend drift.

## 11. Risks & mitigations

| Risk | Mitigation |
|---|---|
| Filename parsing is the #1 quality driver | large corpus, confidence scoring, always-available manual Identify |
| Provider rate limits / API key missing | cache aggressively, backoff, degrade to folder names, NFO path |
| ffprobe absence in dev/host | preflight check with clear error; docker image ships ffmpeg; dev script fetches static build |
| SQLite write contention with scans | WAL, single writer goroutine/batch transactions, short txns |
| Inotify limits in Docker | default to scheduled scan; watcher opt-in |
| Art cache unbounded growth | size cap + LRU eviction (M2+) |
| Codec licensing (the reason Video Station died) | we ship ffmpeg (LGPL build) as a separate binary; no proprietary codecs bundled |
| CGO/driver surprises on Alpine | modernc (pure Go) + CGO_ENABLED=0 verified in CI |
| Scope creep into streaming platform | phases: management is M1–M3 and must be excellent before M4 |

---

## 12. Decisions needed (please answer by number; "defaults" is a valid reply)

**A. Scope**
1. Phase priority: I assume **M1–M3 (management) before playback**. Agree? Or must
   direct-play exist in the first usable release?
2. Users: schema supports multi-user from day 1, but v1 UI is single admin? (default: yes)
3. Do we ever **write** into media folders (rename, `.nfo`, local art)? Default:
   **never**; read sidecars, write only to `/data`.
4. Extras/trailers/samples: index them as `extra`, hide by default? (default: yes)
5. Anime/absolute-numbering support in v1? (default: M7)

**B. Metadata**
6. TMDB: do you have an API key? Is TMDB the only provider for v1 (default: yes,
   TVDB pluggable later)? Key via env var or settings UI (default: both, settings wins)?
7. Metadata language default? (default: `en-US`, per-library override)
8. Do you need IMDb/TVDB ratings and Rotten Tomatoes-style scores in v1? (default: no)
9. Parental/content-rating filtering in v1? (default: M6)

**C. Media & runtime**
10. ffmpeg/ffprobe is **not installed** on this host. Preferred dev setup:
    (a) `pnpm bootstrap` downloads a static ffmpeg into `.tools/`,
    (b) you install it via apt/brew, (c) develop only through Docker. Default: **(a) + docker**.
11. SQLite driver: **modernc.org/sqlite** (pure Go, static, slightly slower) vs
    `mattn/go-sqlite3` (CGO). Default: **modernc** (matches single static binary + alpine).
12. Search: SQLite **FTS5** enough, or do you want trigram/fuzzy matching too?
    (default: FTS5 now, fuzzy in M3)
13. Transcoding target: **HLS fMP4** with remux-first, HW accel optional. Any
    specific client (Safari/iOS) constraint we must design for now? (default: HLS)
14. Hardware transcoding: needed in v1? Which GPU/NAS (Intel QSV, AMD VA-API, Nvidia)?
15. Media locations: local paths only for v1; SMB/NFS assumed pre-mounted by host.
    Remote backends (S3/WebDAV) later? (default: yes)
16. Live watcher (fsnotify) default **off**, scheduled scans default every 6h? (default: yes)

**D. Stack & repo**
17. Go module path + license: `github.com/<you>/myvideostation`, **AGPL-3.0**
    (like Jellyfin) or **MIT**? Please give me the GitHub org/user name.
18. Repo layout: monorepo with `web/` (recommended) vs separate frontend repo?
19. Query layer: **sqlc** (recommended) vs sqlx/gorm? Migrations: **goose** ok?
20. UI kit: shadcn-style on Radix (recommended) vs Base UI vs hand-rolled?
21. Router: TanStack Router file-based routing (recommended) vs react-router?
22. Auth: cookie sessions (HttpOnly, server-side) vs JWT? OIDC/SSO later? (default: cookie sessions)
23. API docs: OpenAPI generated from Go (e.g. huma) or hand-written spec?
    (default: hand-written OpenAPI + CI drift check)
24. TypeScript 7 `tsgo` is very new — okay to pin exactly and use it for typecheck
    only (Vite handles transpile)? Any fallback preference if it misbehaves?
25. CI: GitHub Actions + goreleaser? Multi-arch images (amd64/arm64) required?

**E. Product UX**
26. Primary target screen: desktop browser on NAS/LAN (default) or TV/large screen?
27. Design direction: dark cinematic (default) with optional light theme; any brand
    color/name preference besides "MyVideoStation"?
28. Languages for UI: English only for now, i18n skeleton later? (default: yes)
29. Any must-have Video Station behavior you personally rely on that I should not
    drop (e.g. "TV recording", "personal video collections", "share to DS video")?

---

## 13. Proposed immediate next steps (after answers)

1. Record decisions as `docs/adr/0001-stack.md`.
2. Execute **M0** end-to-end so `pnpm dev`, `pnpm build`, `docker compose up` all work.
3. Build the **naming parser + corpus tests** first — it de-risks the entire product.
4. Then M1 scanner → DB → browse UI with one real library mounted read-only.

---

## 14. Implementation log

### M0 — delivered

- Go module `github.com/liyu1981/myvideostation`; `cmd/mvs` with flag/env config,
  `log/slog`, graceful shutdown.
- SQLite via `modernc.org/sqlite` (CGO off) with WAL/busy_timeout/foreign keys, plus an
  embedded forward-only migration runner (`internal/db`).
- Embedded SPA with two build modes: `embedui` (real UI) and a stub for `go test`/`go build`.
- pnpm scripts (`dev`, `build`, `test`, `lint`, `docker`, `bootstrap`), air config,
  multi-stage Alpine Dockerfile (ffmpeg, non-root, healthcheck), compose file, CI workflow.

### M1 — delivered

| Plan item | Status |
|---|---|
| Libraries CRUD, multiple paths | ✅ (API + UI, path validation) |
| Incremental scanner, ignore rules, jobs | ✅ durable `jobs`, progress polling, missing-file + orphan cleanup |
| Filename parser + corpus | ✅ 34-case table test (scene, SxxExx, 1x02, multi-ep, CJK, editions) |
| Schema + FTS5 | ✅ single self-referencing `media_items`, `media_files`, people/genres/locks, `media_fts` |
| Browse UI (grid/list/tile, sort, filter) | ✅ plus pagination, per-library filter, empty/error states |
| Item detail + technical info | ✅ poster/backdrop, badges, cast/genres, media-info panel (audio/subtitle tracks) |
| Manual metadata edit + locks | ✅ Video-Station-style **Edit video info** dialog opened from the detail overflow menu: Video Info / Poster / Backdrop tabs; title/tagline/release date/classification/rating/genre chips/cast chips/summary; Poster & Backdrop tabs with Original / No images / Enter URL; per-field lock chips and item-wide lock under "Keep locked after refresh" |
| Extras indexing | ✅ trailers/featurettes hidden from browse, attached to the folder's item |
| Multi-episode files | ✅ `episode_end` recorded and shown as `S01E01-E02` |
| Multi-file movies | ✅ `CD1`/`CD2`/`Part 1`/`Disc 2` markers are stripped so the files share one identity; the DB stores them as one item with multiple `media_files`. A scan also merges pre-existing part duplicates (same title/year + part-marked files), and `keep_separate` opts a pair out (verified: 非诚勿扰 CD1+CD2 collapsed to one item with both files) |
| Local metadata search | ✅ **Select from existing videos** in Edit video info: FTS search of the same library/kind, select an item and Apply copies its title, overview, tagline, release date, rating, artwork, genres and cast/crew onto the current item (works for movies, shows and episodes when TMDb has no result) |
| Duplicate prompt | ✅ saving metadata or applying a TMDb match checks for another same-title item and offers **Merge** or **Keep separate**. Works for movies/home videos (files are moved) and **shows** (seasons/episodes are merged by number); migration 0011 drops the old unique show-title index so the match can apply before the prompt |
| Dialog chrome | ✅ every dialog uses the shared Video-Station-style shell (red top rule, centred red title, tab bar, square corners, blue primary action) |
| Episode editing | ✅ each episode card in the show/season view has a pencil action that opens Edit video info for that episode (Episode title / Air date labels), in addition to the episode detail overflow menu. Episodes also support **Search from Video Info Plugin**: the dialog searches TV shows (default query = show title) with **Season / Episode number inputs** (prefilled from the item) and matches the selected episode, storing title, overview, air date, rating and the TMDb still; Refresh metadata works the same way. Episode lists are sorted by episode number, the episode page uses a **16:9 still** in the artwork slot (not a 2:3 poster) and the **show's backdrop** behind the page, and an episode match invalidates the season/show queries so the card thumbnail refreshes immediately |
| Probe performance | ✅ 4-way parallel ffprobe, sequential batched writes |
| Home | ✅ continue watching, recently added, per-library shelves |
| Search | ✅ FTS5 across title/original/overview/people |

Extras shipped beyond M1 scope: direct-play streaming with range requests, progress
reporting, watched toggle, local sidecar artwork, end-to-end API test.

### M2 — delivered

| Plan item | Status |
|---|---|
| Metadata provider | ✅ `internal/metadata` interface + TMDB client (v3 key or v4 token), configurable language/region |
| Matching | ✅ token-overlap + year + popularity score, 55% threshold, manual override wins |
| Enrichment | ✅ background `enrich` job per library, per-item refresh, `metadata_status`/`match_confidence` |
| Artwork | ✅ poster/backdrop downloaded to `<data>/cache/images` (content-addressed), URL picker in the edit dialog |
| Cast/crew/genres/certification | ✅ applied transactionally; crew roles mapped to director/writer |
| Locks | ✅ automatic enrichment skips locked fields; manual match overrides (explicit user choice) |
| DB authority | ✅ local `.vsmeta` > `.nfo` applied at import; user edits and TMDb matches set `metadata_status = manual` and rescans never revert them |
| Identify UI | ✅ nested **Search from Video Info Plugin** dialog inside Edit video info: title search, **language picker** (remembered in `localStorage`, default `en-US`), results table (Poster / Title / Release date / Source / Summary), row select + OK. Manual searches are deliberately broad (the item's stored year is **not** applied implicitly), so re-searching an already-matched item still lists all provider results for a manual change; "Needs attention" row on Home and per-library counts. The default search language setting was removed from Settings. |
| Settings UI | ✅ TMDB key (masked) and certification region; the default search language was removed (per-search language picker, remembered) |

### M2 — known limitations

- Only TMDB is implemented; TVDB/Douban remain pluggable via the provider interface.
- Episode-level metadata (titles, stills) is not fetched — shows are enriched at show level.
- No NFO import yet; manual edits are the fallback.
- Provider calls are synchronous per item inside the job (no rate-limit pacing yet); TMDB's
  40 req/10 s limit is respected only by the sequential job loop.

### M3 — delivered

| Plan item | Status |
|---|---|
| Collections | ✅ Favorites + Watchlist system collections, user collections (create/rename/delete, add/remove items), collection pages |
| Selection mode | ✅ toggle in browse, per-card check state, bulk watched/unwatched and add-to-collection |
| Categories | ✅ recent added / recently watched / recently released / A–Z, plus the "needs attention" preset |
| Facets | ✅ genre + year counts per library, watched-state filter, folder filter (rel-path prefix) |
| Scheduler | ✅ per-library auto-scan interval (hourly…weekly), dedupe against active scans, `last_auto_scan_at` |
| Task monitor | ✅ `/tasks` page with live progress for scans and enrichment |

### M3 — known limitations

- Selection mode is poster-grid only (view toggles are disabled while selecting).
- Bulk actions cover watched state and collections; hide/delete-library-item bulk actions are
  deliberately absent until M6 permissions.
- Folder filter is a flat prefix list (no tree UI); "Timeline" grouping is not implemented yet.
- Auto-scan intervals are minute-based and evaluated once per minute; no cron expressions.

### M4 — delivered

| Plan item | Status |
|---|---|
| Direct play | ✅ byte-range streaming + the M4 player UI |
| Subtitle discovery | ✅ sidecar `<stem>.<lang>.srt/ass/ssa/vtt` found during scans, stored per file |
| Subtitle conversion | ✅ SRT/ASS/SSA/VTT → WebVTT, charset detection (UTF-8 / GB18030 / Big5), embedded tracks extracted with ffmpeg and cached |
| Track selection | ✅ subtitle menu in the player; default prefers English |
| Resume | ✅ playback endpoint returns `resumeMs`; player seeks on load; progress saved on pause/interval/close |
| Next episode | ✅ next-up card on end (same season, next number) + watch state |
| Keyboard | ✅ space, ←/→ (10 s), f (fullscreen), esc |

### M4 — known limitations

- `streamUrl` is the original file: browsers that cannot decode HEVC/AC3/DTS will not play.
  That is what M5 (remux/transcode) fixes.
- Audio-track switching is not possible with plain `<video>`; it arrives with HLS renditions in M5.
- Image-based subtitles (PGS/VobSub) are skipped in the player (burn-in needs M5).
- No subtitle sync offset, styling controls, or SMI support yet.
- Next-up covers the same season only (no cross-season rollover), and does not auto-play.

### Local sidecar metadata (post-M4) — delivered

| Item | Status |
|---|---|
| `.nfo` import | ✅ Jellyfin/Kodi `movie`, `tvshow`, `episodedetails`; title/originaltitle/year/plot/tagline/mpaa/ratings/runtime/genres/actor/director/credits/uniqueid/thumbs |
| `.vsmeta` import | ✅ schema-less protobuf parser with recovered field map; embedded poster/backdrop cached by MD5; provider JSON (IMDB/TMDB ids, rating) |
| Precedence | ✅ NFO overrides `.vsmeta` field-by-field; `.vsmeta` fills gaps and provides artwork |
| Scanner integration | ✅ `provider = nfo\|vsmeta\|nfo+vsmeta`, `metadata_status = local`, locks respected, manual Identify overrides |
| Identity safety | ✅ item identity follows the file path, so sidecar titles and user renames survive re-scans (no duplicates) |
| Show-level NFO | ✅ `tvshow.nfo` in show/season ancestors applied to the show; episode NFO applied to the episode |

Limitations: NFO `<role>`/character names are parsed as cast names only (characters are not
stored yet); remote NFO `<thumb>` URLs are ignored; `.vsmeta` TV-specific fields are unknown
(no sample); sidecars are re-parsed on every scan (correct, but `vsmeta` files with embedded
art cost a read per scan).

### Scan robustness (post-M4 fixes)

| Item | Status |
|---|---|
| Incremental persistence | ✅ items are committed in small, time-bounded batches (8 files / 400 ms), so the library populates while the scan runs |
| Failure isolation | ✅ if a batch fails, files are retried one-by-one; the bad file is skipped and counted (`skipped`) instead of aborting the scan |
| Duplicate titles | ✅ migration 0006 drops the unique `(library_id, kind, title, year)` index; the same movie in two folders/qualities is allowed (identity is the file path; version merging is future work) |
| Job diagnostics | ✅ task rows expand to the full message/error and have a copy button (Tasks page and Libraries page) |

### Home layout (post-M4) — delivered

| Item | Status |
|---|---|
| Hero carousel | ✅ **VS "bookshelf" ring**: inset like the rows below (`mx-6 lg:mx-8`, rounded). Layout is `<poster> | <bg>`: a full-height poster (`0.8 × height` wide) with a **fixed 16:9 backdrop block immediately to its right**, so the poster + bg block keeps a constant aspect ratio instead of stretching with the page width; a **left-dark gradient mask** on the backdrop makes the poster pop; collapsed shelf posters (and the left sliver) carry the same dark gradient mask, which is removed on hover. The title/stars/runtime/age chip sit over the backdrop (bottom-left). A **48 px sliver of the previous poster** peeks at the far left (its left/poster-art edge, lightly blended), narrower than a 90 px spine. The right shelf **starts immediately after the poster + 16:9 block** (no pinned-right gap: extra page width simply reveals more following cards) and shows tightly packed full-height poster spines with a **dark gradient mask that lifts on hover**; it is a **ring** (three copies in the DOM, offset by the current index, so the last title is followed by the first). Each slide is an **already-expanded block (poster + 16:9 bg)**: clicking a spine translates that block from its shelf position to the poster slot while the shelf glides left by that many spines and the ring rotates and the old block slides out left. The moving block's bg travels under the shelf so the strips stay visible and move; the real poster/backdrop are revealed in the same frame the clones are removed (no blink). No prev/next arrows — the shelf and progress bars are the navigation. Animation (ReactBits-inspired patterns, implemented originally): block translation, shelf glide with an invisible snap on wrap-around, backdrop crossfade, ken-burns, staggered title rise, autoplay bars, drag/swipe (14 px threshold), hover pause. Reduced-motion aware (direct switch). |
| Rows | ✅ rows grouped by media type (Movies, TV Shows, Home Videos) plus Continue watching and Needs attention; fixed 120 px posters, row icons, counts, ‹ › paging, View all |
| Browse grid | ✅ fixed card size (120×180, 104 in compact density) with `flex-wrap`; column count follows the viewport, posters never scale up to fill the page (matches Video Station's 120 px grid) |
| Playback entry | ✅ `PlayerModal` is now global, so hero/row Play works from any page |

### Operations & player fit (post-M4)

| Item | Status |
|---|---|
| Force rescan | ✅ `POST /libraries/{id}/scan {force:true}` re-probes every file and re-applies sidecars; UI **Scan** (incremental) vs **Rescan** (confirmed, tooltipped) |
| Job logs | ✅ migration 0007 `job_logs`; manager logs start/finish/failure; scanner logs walk roots, classification summary, **per-file probe progress with full paths** (`probing 83/734: /mnt/…/Movie.mkv`), index progress with the current file, and warnings (probe failures, skipped files, unidentified files, walk errors); enrich jobs log progress too |
| Log viewing | ✅ Tasks page: "Logs" toggle (live refresh while running), per-line level colours, copy-all, per-task delete, "Clear finished" |
| Player fit | ✅ `<video class="h-full w-full object-contain">` in a full-bleed black stage: always scaled to the viewport, aspect ratio preserved, no 1152 px cap |

Retention: logs are kept until the task is deleted (or "Clear finished"). To bound growth,
each job is pruned to its newest 50,000 lines when it finishes; the API/UI fetch the newest
5,000 by default.

### Detail page & player parity (post-M4)

| Item | Status |
|---|---|
| Header | ✅ Video Station layout: logo block exactly as wide as the rail (240 px), home button immediately after it (x=248), then the search field; settings + current user menu on the right; the header sits above page content (z-index) so its dropdowns are never covered by the hero; navigation rail below the header |
| Detail layout | ✅ Video-Station structure: **backdrop covers the whole page** (not a top band), 270×388 poster with 76 px play button and 3 px progress line, yellow Unwatched chip, icon actions, 36 px title, tagline, info bar (year/stars/clock duration/certification + resolution/audio badges), bold-label definition rows, summary with View all, bottom-corner prev/next arrows, overflow menu (edit/identify/refresh/watched) |
| Shows & episodes | ✅ season bar with red active tab, 180×120 landscape episode cards; episode pages reuse the layout with `SxxEyy`; episode titles that look like filenames/release tags are repaired to "Episode N" (including when a Jellyfin `.nfo` contains a filename as its title) |
| Player | ✅ browser Back closes the player (opening pushes one history entry; X/Esc use the same entry); custom control bar: prev/next item, ±10 s, play/pause, volume slider, clock, subtitle menu, audio-track list (informational until M5), speed menu, fullscreen, buffered bar, auto-hide (cursor hidden), big pause overlay, next-up prompt, keyboard shortcuts |
| Unsupported codecs | ✅ explicit error panel + download link instead of a silent black screen (Chromium cannot demux MKV/XviD; H.264 needs proprietary codecs) |
| Loading polish | ✅ blurred poster placeholder behind hero/detail backdrops so images never "pop in" onto an empty page; backdrop fades in on load |
| API | ✅ `/items/{id}/neighbors`, audio tracks in `/items/{id}/playback` |
| Detail info line | ✅ 15 px iconised row: release date (calendar), star rating, clock duration, video codec (film), audio codec (volume), age-classification chip on the right |
| Release date | ✅ migration 0008 `media_items.release_date`, imported from `.vsmeta` field 6 and NFO `<premiered>`/`<aired>`, surfaced by the API and UI |
| Media info | ✅ removed from the page body; opened as a dialog from the top-right info button or the overflow menu (VS behaviour) |
| Per-library tasks | ✅ migration 0009 `jobs.library_id`; the Libraries page shows an **Outstanding tasks** section inside each library card (queued / running / failed) with progress and expandable/copyable messages; the global list was removed in favour of an "All tasks" link |
| Browse toolbar | ✅ reduced to VS parity: category dropdown (Recently Added/Watched/Released/All), sort dropdown, thumbnail/tile/list switch, selection mode; genre/year/watched/folder filters and the local filter box removed |

### Deviations (see `docs/adr/0001-m0-m1-decisions.md`)

1. **sqlc deferred** — hand-written typed store until the schema settles (post-M2).
2. **goose replaced** by an embedded ~80-line migration runner.
3. **TanStack Router is code-based** (no plugin/codegen) to keep the build simple.
4. **No auth**, and the default bind is `127.0.0.1` (M6 adds sessions; Docker opts into
   `0.0.0.0` via env).

### Known M1 limitations

- No metadata provider yet (titles/years come from filenames; posters from local files).
- Posters/backdrops are not cached or downloaded; the UI renders a deterministic gradient
  placeholder when no artwork exists.
- TV show year/art is taken from the folder or the first episode; no show-level scraping.
- Extras are attached by folder heuristic (the nearest directory containing media); extras
  in an unrelated folder stay unattached (hidden, not shown).
- Multi-episode files create one episode item with an `episodeEnd` range (no per-episode
  watch state yet).
- Duplicate movies (same title/year, e.g. two qualities in separate folders) become separate
  items; merging them as versions with multiple files is not implemented yet.
- Probing is 4-way parallel; DB writes remain sequential per batch of 100.
- Thumbnails/trickplay, collections, categories/timeline, scheduler and bulk selection are
  M3+.
