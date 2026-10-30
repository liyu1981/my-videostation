# MyVideoStation

**A modern, open-source re-creation of Synology Video Station.** One Go binary, SQLite,
and your existing folders — point it at a library and it identifies, organises, and plays
your movies and shows.

> **Status: planning / pre-M0.** No application code yet. The architecture, data model,
> feature-gap register, and a verified UI study of the real Video Station are complete —
> see [Docs](#docs). Build commands below are the intended target, not yet available.

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

## Getting started

Not runnable yet — the scaffold lands with milestone **M0**. The intended workflow:

```bash
make dev      # Go API on :8080 + Vite on :5173 (proxied)
make build    # pnpm build → go:embed → ./bin/mvs
make test     # go test ./... + vitest
make docker   # multi-stage Alpine image
```

Runtime shape:

```bash
docker run -d --name myvideostation \
  -p 8080:8080 \
  -v /path/to/data:/data \
  -v /mnt/media/movies:/media/movies:ro \
  -v /mnt/media/tv:/media/tv:ro \
  ghcr.io/<org>/myvideostation:latest
```

Configuration precedence: CLI flags → `MVS_*` env → `/data/config.yaml` → defaults.

## Roadmap

| Milestone | Scope |
|---|---|
| **M0** | Repo scaffold, tokens/theme, single-binary build, Docker, CI |
| **M1** | Libraries, incremental scanner, filename parser, browse UI, search, media info |
| **M2** | TMDB matching, art management, identify/correct flows, collections |
| **M3** | Show/season/episode UX, categories/facets, selection mode, scheduler, job monitor |
| **M4** | Direct playback, subtitles, resume, watched state |
| **M5** | HLS remux/transcode, audio/subtitle switching, optional HW acceleration |
| **M6** | Users, permissions, parental controls, share links, i18n |
| **M7** | PWA, casting, trailers, offline conversion, metrics |

Details and acceptance criteria: [`docs/PLAN.md`](./docs/PLAN.md) §8.

## Docs

- [`docs/PLAN.md`](./docs/PLAN.md) — vision, architecture, data model, API, phases, decisions
- [`docs/GAPS.md`](./docs/GAPS.md) — feature-parity register vs. Synology Video Station
- [`docs/UI-PROBE.md`](./docs/UI-PROBE.md) — verified UI measurements from a live 3.0.7 instance
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
