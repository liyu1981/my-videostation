# ADR 0001 — M0/M1 implementation decisions

Status: accepted · Date: 2026-10-30

Records where the code delivered for M0/M1 differs from (or narrows) `docs/PLAN.md`,
so the reasoning is not lost.

## 1. Module path

`github.com/liyu1981/myvideostation` (provisional). Changing it later is a single
`go.mod` line plus an import rewrite; it is not load-bearing.

## 2. Migrations: embedded SQL runner instead of goose

`internal/db` applies `internal/db/migrations/*.sql` in filename order inside a
transaction and records versions in `schema_migrations`.

Why: goose's dependency surface is not justified for forward-only embedded migrations,
and the runner is ~80 lines with no magic. Revisit if we need `down`, squashing, or
out-of-order migrations.

## 3. Queries: hand-written typed store instead of sqlc

`internal/store` exposes typed methods; SQL lives next to the code that uses it.

Why: the schema is still moving weekly (M1 added three tables and changed two indexes).
Introducing sqlc now would mean constant regeneration churn. The store API is already the
seam sqlc would generate behind, so the switch is mechanical once the model is stable
(target: after M2, when metadata/provider columns settle).

## 4. Router: code-based TanStack Router instead of file-based

Routes are declared in `web/src/router.tsx` with `createRoute` and a typed search schema.

Why: file-based routing needs the router plugin/codegen step, which complicates the
single-binary build and CI. The route tree is small; code-based routing keeps `pnpm build`
a plain Vite build. Revisit if route count grows past ~25.

## 5. Direct play shipped early (a slice of M4)

`GET /api/v1/media/{fileId}` serves byte ranges and the UI has a minimal `<video>` modal
that reports progress. It is not the M4 player (no subtitle/audio selection, no resume UI,
no codec fallback), but it makes M1 verifiable end-to-end and exercises the progress table.

## 6. Local artwork only in M1

Scanner looks for `poster/folder/cover/fanart/backdrop` images next to the file or one
directory up, and stores absolute paths. No downloads (TMDB is M2). Serving is guarded so
only files under a configured library root can be read.

## 7. No authentication in M1

The API is single-user (`users` row id=1), and the server **binds to 127.0.0.1 by default**.
M6 adds accounts/sessions and changes the default bind only for Docker (`0.0.0.0`, which is
opt-in via env). Until then: do not expose the port.

## 8. Search index is maintained by the store

`media_fts` is a plain FTS5 table (not external-content) whose rowid mirrors
`media_items.id`; `refreshFTS` runs on item insert/update and on scanner batches.
This keeps the index correct without triggers that would need to join people tables.

## 9. Testing strategy for M1

- `internal/naming`: table-driven corpus (34 cases), including the user's real-world
  CJK/scene filenames.
- `internal/httpapi`: full-stack test with a temp DB + temp media dir — library creation
  through the API, then scan → browse → search → patch → progress → incremental re-scan.
