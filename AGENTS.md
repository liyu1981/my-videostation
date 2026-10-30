# AGENTS.md — Working Rules for This Repository

Instructions for any coding agent (or human) working in **MyVideoStation**.

---

## 1. Commit policy (strict)

- **Never commit on your own initiative.** No `git commit`, `git add` + commit, amend,
  squash, rebase, tag, or push unless the user explicitly asks.
- When the user says "commit":
  1. Commit **once** — one commit containing the pending work, with a clear message.
  2. **Then stop and report**: show the commit hash, the changed files, and state that
     you are **waiting for the next commit instruction**.
  3. Do **not** continue making additional commits, and do not ask repeatedly.
- The user is expected to make the commit call each time. Leave work staged/unstaged is
  fine; never pre-commit "to be safe".
- Never rewrite history, never force-push, never commit secrets.

## 2. Data safety (non-negotiable)

- **Media directories are read-only.** Never rename, move, delete, chmod, or write
  `.nfo`/artwork into a user's media folders. All derived data goes to the configured
  data dir (`/data`), never beside the source files.
- Do not run destructive operations against a live library (mass delete, "fix" scripts,
  bulk metadata writes) without explicit approval.
- When testing against a real instance, **do read-only probing**. Any operation that
  writes (playback progress, share links, scans, metadata saves) requires explicit
  permission first.
- Never commit credentials, session IDs, API keys, tokens, or screenshots of a private
  library. Probe scripts must read secrets from env (`VS_USER`, `VS_PASS`).

## 3. Read the docs before changing code

| Doc | Use it for |
|---|---|
| [`docs/PLAN.md`](./docs/PLAN.md) | Architecture, data model, API sketch, phase plan (M0–M7), open decisions |
| [`docs/GAPS.md`](./docs/GAPS.md) | Known feature gaps vs. Synology Video Station (IDs `GAP-nnn`, `ADV-nn`) |
| [`docs/UI-PROBE.md`](./docs/UI-PROBE.md) | Verified design tokens, layout metrics, component specs, parity checklist |

Rules of upkeep:

- Closing a gap → update its row in `GAPS.md` (status/phase), don't delete it.
- New scope or phase change → update `PLAN.md`.
- Design-token or component changes → update `UI-PROBE.md` §9 (adopt/modernize) or add an ADR.
- Adding a deliberate non-goal → record it in `GAPS.md` with a workaround note.

## 4. Stack (locked unless an ADR says otherwise)

Backend: **Go 1.27**, `net/http` ServeMux, `modernc.org/sqlite` (CGO-free), `sqlc`
queries, `goose` migrations, `log/slog`, `ffprobe`/`ffmpeg` as the only external runtime
dependency. Frontend: **React 19**, **TypeScript 7** (`tsgo` typecheck only), **Vite 8**,
**TanStack** (Router/Query/Table), **Tailwind CSS 4**, **Jotai**, **pnpm**. Delivery:
single static binary with embedded SPA; Alpine Docker image, non-root, amd64+arm64.

- No CGO. No framework churn. No new heavyweight dependency without a short justification
  in the PR/commit message.
- Front is embedded via `go:embed`; the binary must build and run without Node.
- Migrations are forward-only in production, with `down` scripts for development.

## 5. Repository conventions

```
cmd/mvs/            main
internal/           config, db, store (sqlc), media, naming, scanner,
                    metadata, library, playback, auth, jobs, httpapi, web (embed)
migrations/         goose SQL (embedded)
sql/                sqlc queries
web/                pnpm workspace (single app)
deployments/        Dockerfile, compose
docs/               PLAN, GAPS, UI-PROBE, ADRs
testdata/           parser corpus, generated sample media
```

- Go: `internal/…` is private; no `pkg/` dumping ground. Errors wrapped with `%w`.
  Contexts passed through; no global state outside the composition root.
- API: `/api/v1`, JSON camelCase, cursor pagination, problem-style errors, stable IDs.
- DB: every table has `created_at`/`updated_at` where meaningful; soft-delete/broken-file
  flags instead of row deletion for user-visible history.
- Naming parser lives in `internal/naming` and must have table tests for every supported
  pattern; add a test case for every real-world filename bug fixed.
- Frontend: route components thin, feature logic in `features/`; server state in TanStack
  Query, ephemeral UI state in Jotai; no prop-drilling of query data.
- Never invent API shapes in the frontend — keep one typed client as the single source.

## 6. Verification before declaring work done

```bash
pnpm lint       # gofmt check + go vet + tsc --noEmit
pnpm test       # go test ./...
pnpm build      # pnpm build:web → go:embed → bin/mvs
pnpm bootstrap  # one-time dev tools (air, static ffprobe)
```

If a command does not exist yet (pre-M0), say so instead of pretending it passed.
Report actual command output for anything claimed as verified.

## 7. Communication

- Be explicit about uncertainty: mark unverified claims `[verify]` in docs, as `GAPS.md` does.
- State what was and wasn't done (especially for probes/tests), and which operations could
  have side effects.
- Prefer small, reviewable diffs; do not mix refactors with behaviour changes.
