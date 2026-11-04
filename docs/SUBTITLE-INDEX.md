# Local subtitle search (design, not implemented)

> Re-scope of **GAP-012** (`docs/GAPS.md`). Video Station offered a *download*
> dialog backed by an online provider; we deliberately do **not** do that
> (accounts, rate limits, copyright, and it is the wrong problem). Instead we
> index the subtitles the user already has and make their text searchable.

Status: **design recorded 2026-11-04, no code yet.** Target M7, after the current
playback/transcoding work.

---

## 1. Goal

Answer "in which film did someone say *…*?" from the library itself:

- full-text search over subtitle **cue text** (not just file names),
- filter by library/language, ranked by relevance,
- every hit carries a **timestamp**, so it can open the player at that moment,
- never modifies or writes into media folders,
- cheap enough to rebuild from scratch after a library rescan or a lost file.

Non-goals:

- downloading subtitles from OpenSubtitles or any provider (permanently out of
  scope for v1; a user can drop sidecars next to their files),
- OCR of bitmap subtitles (PGS/VobSub) — they are not text,
- subtitle *editing*, sync correction across files, or translation,
- cross-library metadata/identity changes: the index is derived data only.

---

## 2. Sources we index

| Source | How | Notes |
|---|---|---|
| Sidecar files found by the scanner | read directly | SRT, ASS/SSA, WebVTT, MicroDVD (`.sub` with text), SMI later |
| Embedded **text** subtitle streams | one ffmpeg pass per file → WebVTT | reuse the existing `internal/subtitle` converter; cache the extracted `.vtt` under `<data>/cache/subs` |
| Embedded bitmap streams | skipped | PGS/VobSub/DVB cannot be converted without OCR |

The scanner already records `external_subtitles` rows and `subtitle_streams` JSON
per file (`media_files.subtitle_streams`), so the index job does not need its own
file discovery.

---

## 3. Why a separate SQLite database

`<data>/subtitles.db` (own `-wal`/`-shm`), **not** tables in `mvs.db`:

- it is a pure cache/index: if it is corrupt, locked or deleted the server keeps
  working and rebuilds it; nothing user-authored is lost,
- rebuilds do not bloat `mvs.db`, its migrations, or the backup/export bundle
  (the bundle stays small and restores fast),
- ingestion can run at its own pace without contending with the main write path
  for scans/enrichment,
- deleting it is the documented "clear index" action (no UI needed at first).

Consistency contract: rows are keyed by `(file_id, subtitle source, mtime, size)`.
Any mismatch with the main DB means "stale row → ignore and re-index". The main DB
is the source of truth; the index never holds authoritative state.

---

## 4. Schema sketch

```sql
CREATE TABLE subtitle_docs (
  id           INTEGER PRIMARY KEY,
  file_id      INTEGER NOT NULL,          -- media_files.id
  item_id      INTEGER NOT NULL,          -- media_items.id (for joins/filters)
  library_id   INTEGER NOT NULL,
  source       TEXT NOT NULL,             -- 'sidecar' | 'embedded'
  format       TEXT NOT NULL,             -- srt | ass | vtt | sub | smi
  language     TEXT,                      -- from the scanner/sidecar name
  stream_index INTEGER,                   -- embedded only
  label        TEXT,
  file_mtime   INTEGER NOT NULL,          -- cache key against the main DB
  file_size    INTEGER NOT NULL,
  cue_count    INTEGER NOT NULL,
  indexed_at   TEXT NOT NULL,
  UNIQUE(file_id, source, COALESCE(stream_index, -1), COALESCE(language, ''))
);

CREATE TABLE subtitle_cues (
  id       INTEGER PRIMARY KEY,
  doc_id   INTEGER NOT NULL REFERENCES subtitle_docs(id) ON DELETE CASCADE,
  start_ms INTEGER NOT NULL,
  end_ms   INTEGER NOT NULL,
  text     TEXT NOT NULL                 -- cue text, line breaks preserved
);

CREATE VIRTUAL TABLE subtitle_fts USING fts5(
  text,
  content='subtitle_cues',
  content_rowid='id',
  tokenize='unicode61 remove_diacritics 2'
);
```

Rules:

- Triggers or explicit statements maintain `subtitle_fts` in the same transaction
  as cue inserts (the main DB does the same for `media_fts`).
- Cue text is normalized for indexing (collapse cue line breaks to spaces, strip
  ASS override tags `{...}`, decode entities) but the **display text keeps line
  breaks** so a hit reads like a subtitle.
- Language falls back to `und`; never guess from provider data.

### CJK / tokenisation `[verify]`

`unicode61` does not segment Chinese/Japanese/Korean text, so a phrase search
would only match whole "words". Options, in order of preference:

1. **`tokenize='trigram'`** (SQLite ≥ 3.34) for a second FTS table
   (`subtitle_fts_cjk`) and route queries containing CJK code points there.
   `modernc.org/sqlite` bundles a recent SQLite, but this must be verified with a
   real build before committing to it.
2. Fallback: store a bigram-per-cue normalized column and use `LIKE`/prefix
   scans for CJK (slower, but correct, and CJK libraries are much smaller).

The search API hides the choice; results are merged and re-ranked (FTS5 `bm25`
for the latin table, `rank` by cue count/recency for CJK).

---

## 5. Ingestion pipeline

A new durable job type `subtitle_index`, driven by the existing `jobs` queue:

1. **Selection**: files in the requested libraries whose `(mtime, size)` differs
   from `subtitle_docs`, plus files with no docs yet. Re-scans only touch changed
   files. Full rebuild = "delete `subtitles.db`, enqueue for all libraries".
2. **Sidecars**: read + parse via `internal/subtitle` (already handles charset
   detection for UTF-8/GB18030/Big5 and SRT/ASS/SSA/VTT/SMI→WebVTT).
3. **Embedded**: one `ffmpeg -map 0:<idx> -f webvtt` per text stream, cached under
   `<data>/cache/subs/` (the existing on-demand extractor already writes there, so
   the job reuses it and warms the cache for playback).
4. **Write**: one transaction per document (delete old cues/rows, insert new,
   update FTS). Progress and per-file failures go through `jobs.Progress`/logs, so
   a broken subtitle never fails a scan.
5. **Parallelism**: 2–4 workers, CPU-bound only for the ffmpeg extraction; skip
   files with no subtitles entirely.

Triggers: after a successful scan (like auto-enrichment), on manual "reindex
subtitles" (admin), and lazily when a search finds no documents for a library that
has subtitles. Low priority — a scan must not wait for it.

---

## 6. Search API (sketch)

```
GET /api/v1/subtitles/search?q=&libraryId=&language=&limit=&offset=
```

```json
{
  "query": "make him an offer",
  "total": 3,
  "hits": [
    {
      "itemId": 42,
      "title": "The Godfather",
      "year": 1972,
      "libraryId": 1,
      "fileId": 77,
      "language": "en",
      "source": "sidecar",
      "startMs": 5123000,
      "endMs": 5129000,
      "text": "I'm gonna make him an offer he can't refuse."
    }
  ]
}
```

Implementation notes:

- FTS5 `MATCH` with `snippet()`/`highlight()` over the cue text; a phrase or
  prefix query is epigraph-escaped (no user-supplied FTS syntax passthrough).
- **Authorization**: the index is keyed by `item_id`/`library_id`; results are
  fetched from the index first, then intersected with the ids visible to the
  requesting user (library allow-list + rating ceiling) from the main DB.
  Unauthorized hits are dropped before counting — never leak titles.
- Dedupe: one hit per (item, cue) but cap to ~5 hits per item so a single film
  does not dominate; `total` counts distinct items as well as cues.
- Empty/very short queries (< 2 chars, or a single common word) are rejected.

Player integration: `?t=<ms>` on the existing player route. The player already
starts sessions at an offset (`POST /playback/{id}/transcode {startMs}`); for
direct play it seeks after `loadedmetadata`. Clicking a hit opens the item and
starts at the cue.

UI: a scope switch on the existing search page — `Titles | Subtitle lines`,
grouped by item, with language badges and timestamps. No new top-level route.

---

## 7. Storage & performance envelope

- Rough size: ~1,000 cues per film, ~50 bytes of text per cue → ~50–100 KB of DB
  per title; 5,000 titles ≈ 250–500 MB. Acceptable for a `/data` volume; the
  whole DB is cache and can be dropped.
- Ingestion cost: dominated by ffmpeg extraction for embedded streams (one pass,
  same order as probing). Sidecar-only libraries index in milliseconds per file.
- Search: FTS5 is sub-10 ms for hundreds of thousands of cues; use a `LIMIT` plus
  the per-item cap before hydration.

---

## 8. Failure & recovery

| Situation | Behaviour |
|---|---|
| `subtitles.db` missing | created empty; a rebuild is enqueued on the next scan/search miss |
| `subtitles.db` corrupt (`SQLITE_CORRUPT`) | log a warning, delete and recreate the file, re-enqueue; search returns "index unavailable" once |
| DB locked / disk full | job fails per-file, retried by the queue; playback and browsing unaffected |
| File renamed/moved | scanner changes `file_id` identity → stale docs are pruned on the next pass (compare live `file_id` set) |
| Media folder goes offline | docs are kept but hits are dropped when the item/file is no longer visible; no media access happens during search |
| Backup/export | `subtitles.db` is **not** included in bundles (derived data, rebuilt after restore) |

Never write extracted text into media folders: the cache lives under
`<data>/cache` and the DB under `<data>`.

---

## 9. Work breakdown (when scheduled)

1. `internal/subindex` package: open/migrate the side DB, schema, FTS maintenance,
   `Upsert(doc, cues)`, `Search(query, filters)`, `Prune(validFileIDs)`.
2. Ingest job `subtitle_index` + wiring after scans and an admin reindex action.
3. `/subtitles/search` handler with authorization intersection; tests for CJK,
   phrase search, stale rows and permission filtering.
4. Player deep link (`?t=`) and the `Titles | Subtitle lines` scope in the search UI.
5. Docs: API row, GAPS status → `CLOSED`, README feature row.

Estimate: ~3–5 focused days once the queue and subtitle converters are reused as
described; the risky part is FTS5 tokenisation for CJK `[verify]`, which is why it
is item 1 and testable in isolation.

---

## 10. Open questions

1. **CJK tokenizer**: trigram FTS table vs bigram fallback (verify with the pinned
   `modernc.org/sqlite`).
2. **Scope**: index all libraries by default, or make it opt-in per library
   (storage/CPU vs convenience)? Proposal: index by default, with a per-library
   toggle later alongside scan settings.
3. **Index while scanning or only on demand?** Proposal: enqueue after each scan
   (low priority, debounced), plus an explicit reindex button in Maintenance.
4. **Do we keep the extracted `.vtt` cache forever?** Proposal: it already backs
   playback subtitle extraction; keep it, cap the cache with the existing
   transcode/subs cache policy.
