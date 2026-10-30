# Feature Gap Register — MyVideoStation vs. Synology Video Station

Companion to [`PLAN.md`](./PLAN.md). This file tracks **what Video Station can do that
MyVideoStation currently does not**, so gaps are deliberate rather than accidental.

- Scope: functional parity with Synology Video Station (DSM package, discontinued /
  no longer developed by Synology).
- "VS" = Synology Video Station. Behavior claims marked **[verify]** are from memory
  and should be confirmed against a real DSM before we invest in parity work.
- Keep this file updated whenever a gap is closed, accepted, or re-scoped.

---

## Legend

| Status | Meaning |
|---|---|
| `GAP-LATER` | Agreed to build, but scheduled after the milestone listed |
| `GAP-NEVER` | Deliberately out of scope (rationale required) |
| `GAP-UNDECIDED` | Needs a product decision; see §12 of `PLAN.md` |
| `PLANNED` | Already covered by a milestone in `PLAN.md` — not a gap, listed for parity confirmation |
| `CLOSED` | Implemented — keep the row as history, strike through the description |
| `ADV` | We already exceed VS (tracked here only for balance) |

**Severity** = how much a typical Video Station user would miss it on day one.
`S1` = would block adoption · `S2` = frequent complaint · `S3` = nice to have.

---

## 1. Summary

| Area | Open gaps | S1 | S2 | S3 | Notable |
|---|---|---|---|---|---|
| Library & indexing | 8 | 1 | 3 | 4 | 1 planned (multi-path) |
| Metadata | 3 | 0 | 2 | 1 | NFO + field locks (ADV) |
| Playback & transcoding | 11 | 2 | 5 | 4 | — |
| Clients & remote access | 3 | 3 | 0 | 0 | all permanent-for-v1 |
| Users, security, permissions | 6 | 0 | 5 | 1 | — |
| Admin, ops, i18n | 6 | 0 | 4 | 2 | single binary, job visibility |
| Integrations | 3 | 0 | 0 | 3 | all permanent |
| **Total** | **40** | **6** | **19** | **15** | 10 advantages, 1 planned |

Distribution of the six S1 gaps matters more than the total: **five are client/playback
side** (codec breadth, casting, mobile apps, TV apps, remote access) and only one is
library-side (DVR/TV recording). That matches the agreed priority: management
(M1–M3) first, with playback/client parity debt paid in M4–M7. Nothing S1 in M1–M3
remains unaddressed by the plan.

---

## 2. Library & indexing

| ID | Gap | VS behavior | Status | Sev | Target | Notes |
|---|---|---|---|---|---|---|
| GAP-001 | Network/cloud sources configured in-app | VS indexes SMB/NFS shares and cloud folders configured inside DSM | `GAP-LATER` | S2 | post-M7 | v1 requires host-mounted paths (compose `volumes:`). SMB/NFS clients and S3/WebDAV/SFTP backends are a separate storage-abstraction project. |
| GAP-002 | Real-time index watching on by default | VS has real-time indexing plus scheduled indexing | `GAP-LATER` | S3 | M3 | We ship scheduled scans by default and `fsnotify` as opt-in, because inotify limits inside containers caused most VS-vs-Docker support noise. |
| GAP-003 | Optical/ISO image playback (`ISO`, `BDMV/`, `VIDEO_TS/`) | VS can play DVD/BD folder structures and ISOs | `GAP-UNDECIDED` | S3 | — | ffmpeg can read these, but stream/seek/subtitle semantics and chapter handling are nontrivial. Decide after M5. |
| GAP-004 | TV recording library + EPG + scheduled recording | VS records from USB DTV dongles / HDHomeRun, with EPG and series recording | `GAP-NEVER` (v1) | S1 | — | Requires tuner abstraction, EPG ingestion, conflict resolution, post-processing. Live TV is a separate product (see also GAP-045). Revisit only with a dedicated design. |
| GAP-005 | Indexing/exclusion rules per folder | VS excludes folders, skips `@eaDir`, allows include filters | `GAP-LATER` | S3 | M3 | We ship sane defaults; user-configurable globs, min-size, and "skip extras" toggles come with bulk edit. |
| GAP-006 | Thumbnail generation controls | VS lets you toggle thumbnail generation, set quality, rebuild thumbnails | `GAP-LATER` | S2 | M3 | We need thumbnails (poster grid + trickplay), so controls (on/off, size, regenerate, cache budget) land with the first thumbnail worker. |
| GAP-007 | Scrub-preview trickplay sprites | VS shows a preview thumbnail while seeking | `GAP-LATER` | S3 | M5 | Requires a sprite/BIF pipeline + player overlay. Cheap once the transcode worker exists. |
| GAP-008 | Import/migration from an existing Video Station library | N/A (VS is the source) | `GAP-UNDECIDED` | S2 | post-M3 | VS stores metadata in PostgreSQL; a one-shot importer (paths + watch state + manual matches) would remove the biggest switching cost. High leverage, low glamour. |

## 3. Metadata

| ID | Gap | VS behavior | Status | Sev | Target | Notes |
|---|---|---|---|---|---|---|
| GAP-010 | Multiple scrapers + per-library provider choice | VS offers TheMovieDB and TheTVDB, plus region-specific sources (Douban) **[verify]** | `GAP-LATER` | S2 | M2 / M6 | v1 ships TMDB only behind a provider interface. TVDB matters for anime/absolute ordering; Douban matters for zh-CN libraries. |
| GAP-011 | Trailer playback (YouTube) | VS fetches and plays trailers | `GAP-LATER` | S3 | M7 | Requires provider trailer keys + an embed/privacy decision (no third-party iframe by default). |
| GAP-012 | Online subtitle search & download | VS/DS video can search and download subtitles **[verify]** | `GAP-UNDECIDED` | S2 | M6 | OpenSubtitles requires an account/API key and has rate limits. Also a copyright-sensitive feature; decide policy explicitly. |
| GAP-013 | Metadata language/region per library | VS selects scraper language per library | `PLANNED` | S3 | M1 | Already in the plan (`libraries.language` + provider language param); listed to confirm parity intent rather than claim a gap. |
| GAP-014 | Rich person pages / filmography | VS shows cast lists only | `ADV` | — | M2 | We store `people` as first-class rows, so filmography pages are cheap and exceed VS. |

## 4. Playback & transcoding

| ID | Gap | VS behavior | Status | Sev | Target | Notes |
|---|---|---|---|---|---|---|
| GAP-016 | Broad codec/container coverage in browser | VS transcodes anything it can demux, incl. HEVC, AC3/DTS/TrueHD, VC-1, MPEG-2, Xvid, RMVB | `GAP-LATER` | S1 | M5 | Direct-play (M4) covers H.264/AAC/WebM only. Until M5, users with HEVC/DTS libraries will see "cannot play" — this is the single largest parity debt. |
| GAP-017 | Subtitle charset detection, `SMI`, sync offset, styling | VS guesses Big5/GBK/Shift-JIS, reads SMI, offers per-playback delay and font/size/color | `GAP-LATER` | S2 | M4 (sync/styling) · M6 (charset/SMI) | Charset guessing is a real-world requirement for CJK libraries. `golang.org/x/text/encoding` + heuristics. |
| GAP-018 | Image-based subtitles (PGS/VobSub) | VS renders or burns them in | `GAP-LATER` | S2 | M5 | Cannot be sent as WebVTT. Options: burn-in (re-encode) or client-side canvas rendering with extracted bitmaps. |
| GAP-019 | Continuous play, "play all", shuffle | VS plays a season/collection continuously and supports play-all | `GAP-LATER` | S2 | M4 | Next-episode autoplay is planned; shuffle/queue semantics need a small player state machine. |
| GAP-020 | Batch/offline conversion ("convert video") | VS converts files to MP4 with target device profiles, as a background job | `GAP-LATER` | S3 | M7 | Our job queue makes this straightforward, but it writes new files — needs an explicit output policy that respects the read-only rule. |
| GAP-021 | ISO/3D/MVC playback | VS supported 3D MVC and disc images | `GAP-NEVER` (v1) | S3 | — | 3D is effectively dead; disc images fold into GAP-003. |
| GAP-022 | Casting: Chromecast, AirPlay, DLNA renderers | VS casts from web and mobile to TVs/speakers | `GAP-LATER` | S1 | M7 | Cast SDK needs HTTPS + a public-ish origin; AirPlay needs an audio/video sender or HLS. Browser-only "cast" is increasingly limited. |
| GAP-023 | DLNA/UPnP media server | VS exposes libraries to TVs/consoles over DLNA | `GAP-NEVER` (v1) | S2 | — | Large, cursed spec; Jellyfin/minidlna can front our files if needed. Revisit only on demand. |
| GAP-024 | Multi-audio-track switching during transcode | VS switches tracks and keeps position | `GAP-LATER` | S2 | M5 | Requires restarting the HLS session at the current offset; design it in from the start of the transcode session model. |
| GAP-025 | Per-user bandwidth / concurrent stream limits | VS has bandwidth control and device/stream limits | `GAP-LATER` | S3 | M6 | Needed before exposing to a family/Internet; cheap token-bucket + per-user session cap. |
| GAP-026 | Resume playback across devices | VS syncs position per DSM user | `GAP-LATER` | S3 | M4 | `playback_progress` is already in the schema; listed to confirm the cross-device expectation. |

## 5. Clients & remote access

| ID | Gap | VS behavior | Status | Sev | Target | Notes |
|---|---|---|---|---|---|---|
| GAP-030 | Native mobile apps + offline downloads | DS video for iOS/Android, with download-for-offline | `GAP-NEVER` (v1) | S1 | — | We ship a responsive PWA. True offline download of DRM-free media is feasible later via service worker + OPFS/IndexedDB but is a project in itself. |
| GAP-031 | Native TV apps | DS video on Apple TV / Android TV / Fire TV / Roku **[verify]** | `GAP-NEVER` (v1) | S1 | — | Mitigation: browser on TV, or expose a Jellyfin-compatible API later so existing TV clients work. The latter is the pragmatic path — worth an ADR. |
| GAP-032 | Zero-config remote access (QuickConnect / DDNS + relay) | VS works outside the LAN with no setup via QuickConnect relay | `GAP-NEVER` | S1 | — | We require the user to provide connectivity: Tailscale/WireGuard, Cloudflare Tunnel, or a reverse proxy. Running a relay is a hosted-service commitment. Document recipes instead. |

## 6. Users, security, permissions

| ID | Gap | VS behavior | Status | Sev | Target | Notes |
|---|---|---|---|---|---|---|
| GAP-040 | DSM/LDAP/AD identity integration | VS uses DSM accounts, so NAS permissions and VS permissions are one system | `GAP-LATER` | S2 | M6 | We own accounts. OIDC (and optionally LDAP) is the modern equivalent; must not become a blocker for family installs. |
| GAP-041 | 2FA, password policy, lockout, session management | Handled by DSM for VS | `GAP-LATER` | S2 | M6 | TOTP + lockout + active-session revocation. Also mandatory before any Internet exposure. |
| GAP-042 | Parental controls (content-rating ceiling, PIN) | VS restricts libraries/items by age rating per user | `GAP-LATER` | S2 | M6 | `users.rating_ceiling` is reserved in the schema; needs per-library override + PIN for profile switching. |
| GAP-043 | Per-library view permissions | VS supports allow/deny per library per user | `GAP-LATER` | S2 | M6 | `library_perms` table reserved; must be enforced in queries, not just in the UI. |
| GAP-044 | Audit/activity log (logins, playback, admin actions) | VS surfaces DSM logs and per-user history | `GAP-LATER` | S3 | M6 | We have `slog`; a queryable `events` table is the parity item. |
| GAP-045 | Public share links with expiry | VS shares videos/collections via link | `GAP-LATER` | S2 | M6 | Already planned (signed, expiring tokens). Must be rate-limited and revocable. |

## 7. Admin, ops, i18n

| ID | Gap | VS behavior | Status | Sev | Target | Notes |
|---|---|---|---|---|---|---|
| GAP-050 | Backup/restore of library DB, art cache, watch state | VS relies on DSM Hyper Backup; metadata lives in PostgreSQL | `GAP-LATER` | S2 | M3 | SQLite makes this easy: `VACUUM INTO` snapshot + a documented restore, plus an export endpoint (json). Must exist before users invest in manual metadata edits. |
| GAP-051 | Notifications (email, push, webhook) | VS sends DSM notifications for scan/transcode events | `GAP-LATER` | S3 | M7 | Webhook first (cheap), SMTP later. |
| GAP-052 | DSM Package Center install/upgrade and DSM UI integration | VS is a DSM package | `GAP-NEVER` | S2 | — | We are intentionally DSM-independent (Docker/binary). Cost of a `.spk` is high and ties us to Synology. |
| GAP-053 | UI localization breadth (~20 locales) | VS ships Synology-wide translations | `GAP-LATER` | S2 | M6 | i18n scaffolding in M6, English-first. CJK + de/fr/es are the likely first community contributions. |
| GAP-054 | Hardware transcoding breadth + autodetect | VS supports Intel QSV on many models; newer DSM gates HEVC/AAC behind Advanced Media Extensions | `GAP-LATER` | S2 | M5/M7 | We detect VA-API/QSV/NVENC at startup and fall back to CPU. **Advantage**: no Synology account or AME license needed. |
| GAP-055 | Cache/thumbnail store management UI | VS shows cache size and offers rebuild/clear | `GAP-LATER` | S3 | M3 | Needed because art + transcodes can grow to many GB; include a size cap early. |

## 8. Integrations

| ID | Gap | VS behavior | Status | Sev | Target | Notes |
|---|---|---|---|---|---|---|
| GAP-060 | Download Station / File Station / Photos integration | VS links into DSM apps (open folder, download management) | `GAP-NEVER` | S3 | — | No DSM. Equivalent: documented "completed downloads" watch folder + optional file browser later. |
| GAP-061 | Cloud sync (Synology C2, Google Drive, OneDrive) | VS indexes cloud-mounted folders | `GAP-NEVER` (v1) | S3 | — | Folded into GAP-001's storage abstraction if ever needed. |
| GAP-062 | DSM single sign-on / QuickConnect identity | One login for DSM + VS | `GAP-NEVER` | S3 | — | Covered by GAP-032/GAP-040 alternatives. |

---

## 9. Where MyVideoStation is already ahead (`ADV`)

| ID | Advantage | Why it matters |
|---|---|---|
| ADV-01 | No codec licensing gate | Video Station's HEVC/AAC support was tied to Advanced Media Extensions and a Synology account; we use distro `ffmpeg` (LGPL build) with no account. |
| ADV-02 | NFO read/write + local art precedence | Kodi/Jellyfin-compatible sidecars; users keep their metadata if they leave. VS has no `.nfo` support **[verify]**. |
| ADV-03 | Single static binary, no DSM | Runs on any Linux, NAS, Pi, or laptop; no DSM version coupling. |
| ADV-04 | FTS5 search over title, overview, cast | Instant search, no filesystem walk. |
| ADV-05 | Per-item "needs attention" queue + field-level locks | Manual matches and edits are never silently overwritten. |
| ADV-06 | Durable job queue with SSE progress + scan stats | Visibility into indexing/transcoding instead of a spinner. |
| ADV-07 | API-first with documented REST contract | Scriptable, testable, integrable (and a path to client compatibility). |
| ADV-08 | Docker + amd64/arm64, no vendor account | Deploy anywhere; reproducible upgrades. |
| ADV-09 | Modern responsive UI (keyboard, mobile, dark-first) | VS's UI is a decade old and DSM-bound. |
| ADV-10 | Non-destructive by default | Never renames, moves, or deletes user media. |

---

## 10. Verification backlog (things to confirm before committing work)

1. **[verify]** Does VS support NFO sidecars? (If yes, ADV-02 weakens; if no, strengthen it.)
2. **[verify]** Online subtitle search/download — which provider, still functional post-EOL?
3. **[verify]** Exact scraper set by region (TMDB/TVDB/Douban/others) and whether VS supports per-library selection.
4. **[verify]** Which client apps actually shipped (Apple TV/Android TV/Fire TV/Roku) and what they support.
5. **[verify]** Parental control semantics: rating source, per-library override, PIN behavior.
6. **[verify]** Whether VS keeps watch state across re-indexing/file moves (informs our re-link UX).
7. Confirm DS video/VS offline-download semantics (container limits, DRM).

## 11. Gap policy (proposed)

- **Management parity is non-negotiable**: no S1/S2 gap may remain open in library,
  metadata, or organization once M3 is declared done.
- **Playback parity is a ladder**: direct play (M4) → remux (M5) → transcode (M5) →
  casting (M7). Each rung must be explicitly released, not implied.
- **`GAP-NEVER` requires a workaround documented in `docs/`** (e.g. Tailscale recipe
  for GAP-032, minidlna recipe for GAP-023, Jellyfin-API bridge for GAP-031).
- Anything with an unresolved product question lives in §12 (Decisions needed) of
  `PLAN.md`, not silently in code.

## 12. Open decisions raised by this register

| # | Question | Default proposal |
|---|---|---|
| D-1 | Do we want a **Jellyfin-compatible API** layer so existing TV/mobile clients work (closes GAP-030/031 cheaply)? | Yes, but only after M5 — as an adapter over our own API. |
| D-2 | Import tool from Video Station's PostgreSQL DB (GAP-008)? | Yes, M4-ish; big switching-cost reducer. |
| D-3 | Subtitle download integration (GAP-012): include or leave to the user? | Leave out of v1; revisit on request. |
| D-4 | Allow opt-in writes to media folders (`.nfo`, `poster.jpg`) for Kodi interop? | Yes, opt-in per library, default off. |
| D-5 | DLNA (GAP-023): never, or "minidlna recipe + docs"? | Docs recipe only. |
| D-6 | Anything USB DTV/DVR (GAP-004): permanently out? | Out of scope; document a Tvheadend + folder-ingest recipe. |
