# Library monitoring & auto-scrape — audit and plan

Status: **implemented in M3.x** (phases 1–4 delivered; automated watcher/pipeline tests pending)

Video Station is not just a scanner: it watches the library folders, indexes new
media, removes missing files, and tries to scrape metadata on first import. This
document records the pre-M3.x baseline and the monitoring pipeline that was
implemented (GAP-002, GAP-068, GAP-070, GAP-071).

## 1. Baseline (before M3.x, verified in code)

| behaviour | Video Station | MyVideoStation before M3.x | evidence |
|---|---|---|---|
| New library | Indexes the folder immediately and keeps watching it | `POST /libraries` only creates the row; the **web UI** calls `scanLibrary` once afterwards; API clients get no scan. New libraries default `auto_scan_minutes = 0` (off). | `internal/httpapi/libraries.go`, `web/src/routes/LibrariesPage.tsx`, migration 0004 |
| Folder changes | Real-time watching + scheduled indexing | **Scheduled scans only**, and only when the interval is set (default off). No `fsnotify` watcher. | `cmd/mvs/main.go` auto-scan ticker; no `fsnotify` in the tree |
| Add/remove files | Watcher or scan picks them up | A scan marks unseen files missing (`ResetMissing`) and deletes orphan items (`DeleteOrphanItems`). Works, but only when a scan runs. | `internal/scanner/scanner.go` |
| First-time scrape | Tries the provider's first/best result on import; keeps the item even if unmatched | The scan job **does not enqueue enrich**. "Metadata" is a manual button/API. Auto-enrich uses a **55% confidence threshold** and the best-scoring candidate; below it the item stays `unmatched`. | `cmd/mvs/main.go` scan handler, `internal/enrich/enrich.go` |
| Best-effort DB add | Adds files to the library even when it cannot identify them | Parseable movies/shows/episodes are added (unmatched if no metadata) and probe failures are still indexed. **Unparseable/empty-title files are skipped** (`KindUnknown`, `stats.Unidentified`). | `internal/scanner/scanner.go` classify |
| Duplicate jobs | n/a | `HasActiveJob("scan", library)` prevents duplicate scans; there is no enrich dedupe and no scan → enrich chaining. | `internal/store/collections.go` |

So: indexing itself is best-effort for parseable files, but **monitoring is not
automatic** (default off, no watcher) and **first-time scraping is not wired** to
the scan. Unidentified files never reach the DB.

## 2. Deployment assumption (watcher reliability)

The expected production deployment is **Docker on Synology with local shared folders
bind-mounted into the container** (`/volume1/video:/media/movies:ro`). That case is
fine for `fsnotify`: a bind mount shares the host kernel's inotify instance, so
events from DSM apps, downloaders and other host processes are delivered to the
container. The watcher should therefore be **on by default** there.

Not reliable:

- **CIFS/SMB/NFS mounts** (our dev setup, or a remote share mounted on the NAS):
  inotify only reports changes made on the same kernel, so writes from another
  host never generate events. Detect the filesystem type and skip the watcher;
  scheduled scans cover these libraries.
- **Docker Desktop file sharing** (gRPC-FUSE/VirtioFS on macOS/Windows): events are
  not guaranteed; treat it like a network mount.
- The **inotify watch limit** is host-wide
  (`fs.inotify.max_user_watches`), not namespaced per container. Synology admins
  can raise it on the host; the container inherits it. Check the limit and fall
  back to scheduled scans on `ENOSPC`/overflow.

So the watcher is not "optional" in the expected Synology deployment; it is
conditional only on the filesystem being local/bind-mounted.

## 3. Target pipeline (implemented)

```
add library
  └─ enqueue scan immediately
       └─ scan finishes
            ├─ if unmatched items > 0: enqueue enrich (auto-scrape)
            └─ schedule next scan / arm watcher
watcher (optional) or interval
  └─ debounced scan → enrich
```

- **Add**: `POST /libraries` enqueues a scan (not only the UI), and the Add form
  exposes the auto-scan interval (default **360 minutes**) and a watcher toggle.
- **Scan → enrich**: the scan job enqueues an `enrich` job at the end when the
  library has unmatched items. Deduplicate against an active enrich job; the
  enrich job processes only `unmatched` items.
- **Auto-scrape policy**: search with the parsed title/year; take the best
  candidate (or the only result); apply when the score passes a low, configurable
  threshold (default **0.35** instead of 0.55) and never overwrite `local`,
  `manual` or locked fields. Store the confidence; if no candidate, leave the item
  unmatched for the "Needs attention" queue. The user can always override via
  Edit video info / the provider search.
- **Best-effort indexing**: when parsing fails, add a placeholder item instead of
  dropping the file:
  - kind `home_video` for now (playable, editable), title from the filename,
    `metadata_status = unmatched`, no media moves;
  - show it in "Needs attention" and the library item count;
  - a future `unidentified` kind/flag can replace the placeholder without
    changing the DB contract.
- **Watcher** (**default on for local/bind mounts**, the expected Synology Docker
  deployment): `fsnotify` on each library path (watch new directories as they
  appear), debounce ~10 s per library, ignore temp/partial files. Detect the
  filesystem from `/proc/self/mountinfo`; skip CIFS/SMB/NFS and Docker Desktop
  shares (scheduled scans cover them). If the kernel watch limit is exhausted
  (`ENOSPC`) or events overflow, log and fall back to scheduled scans. A
  per-library toggle can still disable/enable it. Document
  `fs.inotify.max_user_watches` (host-wide, tune on Synology); keep the scheduled
  scan as the safety net.
- **Removal**: scans already mark missing files and delete orphan items; the
  watcher only triggers the same scan. Decide whether user-visible items should be
  soft-deleted/hidden first (the DB convention) instead of deleted immediately.

### What shipped

- `POST /libraries` enqueues the initial scan; auto-scan defaults to 360 minutes and
  the watcher to on; the Add/Edit forms expose both.
- A successful scan enqueues `enrich` for unmatched items (deduplicated); the
  auto-match threshold is 0.35 and low-confidence matches are flagged.
- Unparseable files are added as `home_video` placeholders (filename title,
  `unmatched`) instead of being skipped.
- `fsnotify` watches local/bind-mounted paths with a 10 s debounce, 20 s library
  resync and per-library toggle; CIFS/SMB/NFS and Docker Desktop shares are skipped;
  `ENOSPC`/overflow disables the watcher with a warning; scheduled scans remain the
  fallback.
- Verified manually: create→scan→enrich (jobs chained), watcher-triggered scan on a
  local temp library, and placeholder indexing of `random.mkv` in a TV library.

## 4. Implementation phases

| phase | work | closes |
|---|---|---|
| 1. Pipeline wiring | ✅ create → scan; scan → enrich; enrich dedupe; Add form interval; default 360 | GAP-068, GAP-070 |
| 2. Best-effort indexing | ✅ placeholder items for unparseable files; Needs attention UI | GAP-071 |
| 3. Watcher | ✅ fsnotify, recursive watches, debounce, local/bind-mount default, mountinfo detection, ENOSPC/overflow fallback, Synology inotify-limit docs | GAP-002 |
| 4. Auto-scrape policy | ✅ 0.35 best/first candidate, low-confidence flag on the detail page | GAP-070 |
| 5. Tests/CI | ⏳ create→scan→enrich integration test; watcher debounce; unknown files; no duplicate jobs (manual verification done) | — |

Phase 1 is small and gives most of the "monitor" behaviour. Phase 3 adds
near-real-time updates: on for local filesystems, automatically skipped for
network mounts and when the kernel watch limit is exhausted, with scheduled scans
as the correctness guarantee.

## 5. Resolved decisions

1. Default auto-scan interval: **360 minutes**.
2. Watcher: **on for local/bind mounts** (the expected Synology Docker deployment);
   CIFS/SMB/NFS and Docker Desktop shares fall back to scheduled scans; per-library
   toggle.
3. Placeholder kind: `home_video` with the filename title and `unmatched` status.
4. Auto-scrape: best/first candidate at a **0.35** threshold; low-confidence
   matches flagged on the detail page.
5. Removal semantics: orphan deletion stays immediate for now (open follow-up:
   soft-delete/missing flag for user history).
6. Network mounts: skipped by mountinfo detection; scheduled scans cover them.

## 6. Risks

- Wrong first-result matches pollute the library; mitigate with the confidence
  threshold, `unmatched` visibility and manual overrides.
- fsnotify storms on large libraries; debounce, per-library coalescing, and
  inotify-limit documentation.
- Network mounts do not emit reliable inotify events; detect CIFS/SMB/NFS and
  Docker Desktop shares and use scheduled scans there instead of pretending the
  watcher works.
- Synology's default inotify watch limit may be low for large libraries; document
  the host-side sysctl and fall back gracefully.
- Scan → enrich → scan loops; enrich never scans, and enrich is deduplicated.
- Placeholder items can clutter browse; keep them in "Needs attention" until
  identified.
