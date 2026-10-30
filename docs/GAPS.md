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
| Library & indexing | 9 | 1 | 3 | 5 | 1 planned (multi-path) |
| Metadata | 4 | 0 | 2 | 2 | NFO + field locks (ADV) |
| Playback & transcoding | 12 | 2 | 5 | 5 | — |
| Clients & remote access | 3 | 3 | 0 | 0 | all permanent-for-v1 |
| Users, security, permissions | 6 | 0 | 5 | 1 | — |
| Admin, ops, i18n | 6 | 0 | 4 | 2 | single binary, job visibility |
| Integrations | 4 | 0 | 0 | 4 | all permanent |
| **Total** | **44** | **6** | **19** | **19** | 10 advantages, 1 planned |

Four gaps (GAP-063…066) were discovered during the live UI/API probe documented in
[`UI-PROBE.md`](./UI-PROBE.md); several previously `[verify]` claims are now `[confirmed]`.
The rendered probe also **corrected three assumptions** (Video Station does have an
item-level metadata lock, per-item poster/backdrop management, and a "Correct video
info" re-match flow), so parity expectations in M1–M3 were raised accordingly.

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
| GAP-004 | TV recording library + EPG + scheduled recording | VS records from USB DTV dongles / HDHomeRun, with EPG and series recording | `GAP-NEVER` (v1) | S1 | — | **[confirmed]** `SYNO.VideoStation2.DTV.{Tuner,Channel,ChannelScan,Program,Schedule,Statistic,Streaming}`. Requires tuner abstraction, EPG ingestion, conflict resolution, post-processing. Live TV is a separate product. Revisit only with a dedicated design. |
| GAP-005 | Indexing/exclusion rules per folder | VS excludes folders, skips `@eaDir`, allows include filters | `GAP-LATER` | S3 | M3 | **[confirmed]** `SYNO.Core.MediaIndexing.{Scheduler,IndexFolder}`. We ship sane defaults; user-configurable globs, min-size, and "skip extras" toggles come with bulk edit. |
| GAP-006 | Thumbnail generation controls | VS lets you toggle thumbnail generation, set quality, rebuild thumbnails, and choose per-item poster/backdrop (Original / URL / from folders), with a global cover setting under Settings › Advanced | `GAP-LATER` | S2 | M3 | **[confirmed]** `SYNO.Core.MediaIndexing.ThumbnailQuality`; per-item art picker verified in the Edit dialog (see `UI-PROBE.md` §6). Controls (on/off, size, regenerate, cache budget) land with the first thumbnail worker. |
| GAP-007 | Scrub-preview trickplay sprites | VS shows a preview thumbnail while seeking | `GAP-LATER` | S3 | M5 | **[confirmed]** `SYNO.VideoStation2.Screenshot`. Requires a sprite/BIF pipeline + player overlay. Cheap once the transcode worker exists. |
| GAP-008 | Import/migration from an existing Video Station library | N/A (VS is the source) | `GAP-UNDECIDED` | S2 | post-M3 | **[verify]** storage is believed to be PostgreSQL. A one-shot importer (paths + watch state + manual matches) would remove the biggest switching cost. High leverage, low glamour. |
| GAP-063 | Indexing of cloud/mounted sources tied to DSM services | VS can index videos surfaced by Synology Drive and DSM cloud mounts | `GAP-NEVER` | S3 | — | **[confirmed]** `SYNO.SynologyDrive.Services.VideoStation`. We only index real filesystem paths; cloud sources need host mounts (GAP-001). |

## 3. Metadata

| ID | Gap | VS behavior | Status | Sev | Target | Notes |
|---|---|---|---|---|---|---|
| GAP-010 | Multiple scrapers + per-library provider choice | Per-type plugin list under Settings › Video Info Plugin, with drag-to-reorder priority and `Test Connection`; only **The Movie Database (System default)** was installed here for both Movie and TV Show | `GAP-LATER` | S2 | M2 / M6 | **[confirmed]** `SYNO.VideoStation2.{Plugin,PluginSearch,Metadata}` + live UI. No TVDB/Douban present — they would be installed plugins. **M2 ships TMDB behind a provider interface** (plugin UX and extra providers still missing). |
| GAP-011 | Trailer playback (YouTube) | VS fetches and plays trailers | `GAP-LATER` | S3 | M7 | Requires provider trailer keys + an embed/privacy decision (no third-party iframe by default). |
| GAP-012 | Online subtitle search & download | VS has a "Search for subtitles" flow with Language / File name / Source columns | `GAP-UNDECIDED` | S2 | M6 | **[confirmed]** dialog captured; no provider configured on this instance ("No match found"). OpenSubtitles needs an account/API key + rate limits, and this is copyright-sensitive — decide policy explicitly. |
| GAP-013 | Metadata language/region per library | VS selects scraper language per library | `PLANNED` | S3 | M1 | Already in the plan (`libraries.language` + provider language param); listed to confirm parity intent rather than claim a gap. |
| GAP-014 | Rich person pages / filmography | VS shows cast lists only | `ADV` | — | M2 | We store `people` as first-class rows, so filmography pages are cheap and exceed VS. |
| GAP-064 | Third-party scraper/plugin marketplace | VS exposes a metadata plugin system (install/search plugins, per-type ordering) | `GAP-NEVER` | S3 | — | **[confirmed]** `SYNO.VideoStation2.{Plugin,PluginSearch}` + Settings › Video Info Plugin. We expose a Go provider interface + config instead of a sandboxed plugin runtime. Revisit only if third-party providers become a real demand. |
| GAP-067 | Per-item poster/backdrop from URL or media folder | Edit dialog offers Original / Enter URL / Select from folders / Select | `CLOSED` | S3 | M2 | Implemented: URL download + cache (`POST /items/{id}/art`), and sidecar art discovery on scan. Folder picker UI is still a dropdown-less URL field. |

## 4. Playback & transcoding

| ID | Gap | VS behavior | Status | Sev | Target | Notes |
|---|---|---|---|---|---|---|
| GAP-016 | Broad codec/container coverage in browser | VS transcodes anything it can demux, incl. HEVC, AC3/DTS/TrueHD, VC-1, MPEG-2, Xvid, RMVB | `GAP-LATER` | S1 | M5 | Direct-play (M4) covers H.264/AAC/WebM only. Until M5, users with HEVC/DTS libraries will see "cannot play" — this is the single largest parity debt. |
| GAP-017 | Subtitle charset detection, `SMI`, sync offset, styling | VS guesses Big5/GBK/Shift-JIS, reads SMI, offers per-playback delay and font/size/color | `GAP-LATER` | S2 | M4 (sync/styling) · M6 (charset/SMI) | Charset guessing is a real-world requirement for CJK libraries. `golang.org/x/text/encoding` + heuristics. |
| GAP-018 | Image-based subtitles (PGS/VobSub) | VS renders or burns them in | `GAP-LATER` | S2 | M5 | Cannot be sent as WebVTT. Options: burn-in (re-encode) or client-side canvas rendering with extracted bitmaps. |
| GAP-019 | Continuous play, "play all", shuffle | VS plays a season/collection continuously and supports play-all | `GAP-LATER` | S2 | M4 | Next-episode autoplay is planned; shuffle/queue semantics need a small player state machine. |
| GAP-020 | Batch/offline conversion ("convert video") | VS converts files to MP4 with target device profiles, as a background job | `GAP-LATER` | S3 | M7 | **[confirmed]** `SYNO.VideoStation2.{OfflineConversion,Setting.PreAnalysis}`. Our job queue makes this straightforward, but it writes new files — needs an explicit output policy that respects the read-only rule. |
| GAP-021 | ISO/3D/MVC playback | VS supported 3D MVC and disc images | `GAP-NEVER` (v1) | S3 | — | 3D is effectively dead; disc images fold into GAP-003. |
| GAP-022 | Casting: Chromecast, AirPlay, DLNA renderers | VS casts from web and mobile to TVs/speakers | `GAP-LATER` | S1 | M7 | **[confirmed]** `SYNO.VideoStation2.Controller.{Device,Password,Playback,Volume}` — cast targets are paired devices with their own volume/transport control. Cast SDK needs HTTPS + a public-ish origin; AirPlay needs a sender or HLS. |
| GAP-023 | DLNA/UPnP media server | VS exposes libraries to TVs/consoles over DLNA | `GAP-NEVER` (v1) | S2 | — | Large, cursed spec; Jellyfin/minidlna can front our files if needed. Revisit only on demand. |
| GAP-024 | Multi-audio-track switching during transcode | VS switches tracks and keeps position | `GAP-LATER` | S2 | M5 | Requires restarting the HLS session at the current offset; design it in from the start of the transcode session model. |
| GAP-025 | Per-user bandwidth / concurrent stream limits | VS has bandwidth control and device/stream limits | `GAP-LATER` | S3 | M6 | Needed before exposing to a family/Internet; cheap token-bucket + per-user session cap. |
| GAP-026 | Resume playback across devices | VS syncs position per DSM user | `GAP-LATER` | S3 | M4 | `playback_progress` is already in the schema; listed to confirm the cross-device expectation. |
| GAP-065 | Capture a frame/screenshot during playback | VS can save a still from the player | `GAP-LATER` | S3 | M7 | **[confirmed]** `SYNO.VideoStation2.Screenshot`. Trivial once we have an ffmpeg worker; needs a storage policy (write to `/data`, never the media folder). |

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
| GAP-042 | Parental controls (content-rating ceiling, PIN) | VS restricts libraries/items by age rating per user | `GAP-LATER` | S2 | M6 | **[confirmed]** `SYNO.VideoStation2.ParentalControl`. `users.rating_ceiling` is reserved in the schema; needs per-library override + PIN for profile switching. |
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
| GAP-066 | Voice assistant integration (Alexa/Google) | VS can be controlled by voice assistants | `GAP-NEVER` | S3 | — | **[confirmed]** `SYNO.VideoStation2.{VoiceAssistant,Setting.VoiceAssistant}`. Requires cloud account linking + an intent model; out of scope permanently for a self-hosted, account-free product. |

---

## 9. Where MyVideoStation is already ahead (`ADV`)

| ID | Advantage | Why it matters |
|---|---|---|
| ADV-01 | No codec licensing gate | Video Station's HEVC/AAC support was tied to Advanced Media Extensions and a Synology account; we use distro `ffmpeg` (LGPL build) with no account. |
| ADV-02 | NFO read/write + local art precedence | Kodi/Jellyfin-compatible sidecars; users keep their metadata if they leave. VS has no `.nfo` support **[verify]**. |
| ADV-03 | Single static binary, no DSM | Runs on any Linux, NAS, Pi, or laptop; no DSM version coupling. |
| ADV-04 | FTS5 search over title, overview, cast | Instant search, no filesystem walk. |
| ADV-05 | Per-field metadata locks + unmatched queue | VS has only a single item-level `Lock/Unlock` toggle and no "needs attention" queue; we lock per field and surface unmatched items explicitly. |
| ADV-06 | Durable job queue with SSE progress + scan stats | Visibility into indexing/transcoding instead of a spinner. |
| ADV-07 | API-first with documented REST contract | Scriptable, testable, integrable (and a path to client compatibility). |
| ADV-08 | Docker + amd64/arm64, no vendor account | Deploy anywhere; reproducible upgrades. |
| ADV-09 | Modern responsive UI (keyboard, mobile, dark-first) | VS's UI is a decade old and DSM-bound. |
| ADV-10 | Non-destructive by default | Never renames, moves, or deletes user media. |

---

## 10. Verification backlog (things to confirm before committing work)

Status after the live probe (`UI-PROBE.md`) — the probe confirmed 9 behaviours via the
public stylesheet and the unauth API inventory; deeper items still need a DSM session.

1. **[open]** Does VS support NFO sidecars? No NFO strings/APIs appeared in the probe, so
   ADV-02 likely holds. Needs one logged-in check.
2. **[confirmed capability / provider open]** Online subtitle search exists
   (`search-subtitle-dialog`); provider and post-EOL functionality unverified.
3. **[confirmed] plugin architecture** for scrapers; **[open]** exact provider list per region.
4. **[open]** Which client apps actually shipped (Apple TV/Android TV/Fire TV/Roku).
5. **[confirmed API / open semantics]** `ParentalControl` exists; rating source, per-library
   override and PIN behaviour still need UI capture.
6. **[open]** Whether VS keeps watch state across re-indexing/file moves (informs re-link UX).
7. **[open]** DS video/VS offline-download semantics (container limits, DRM).

## 10b. Probe evidence pointers

| Claim | Evidence |
|---|---|
| Version 3.0.7-2512 | `@import "controller/ui/style.css?v=3.0.7-2512"` |
| Casting with paired devices | `SYNO.VideoStation2.Controller.{Device,Password,Playback,Volume}` |
| Trickplay / screenshots | `SYNO.VideoStation2.Screenshot` |
| DVR/live TV | `SYNO.VideoStation2.DTV.*` (10 APIs) |
| Offline conversion + pre-analysis | `SYNO.VideoStation2.{OfflineConversion,Setting.PreAnalysis}` |
| Parental control | `SYNO.VideoStation2.ParentalControl` |
| Scraper plugins | `SYNO.VideoStation2.{Plugin,PluginSearch}` |
| Watch state | `SYNO.VideoStation.WatchStatus` (v1) + `VideoStation2.Info` |
| Sharing links | `SYNO.VideoStation2.Sharing` + `vc-sharing-*` CSS |
| Subtitle search UI | `.search-subtitle-dialog` (56 rule groups) |
| Thumbnail quality settings | `SYNO.Core.MediaIndexing.ThumbnailQuality` |
| Voice assistants | `SYNO.VideoStation2.VoiceAssistant`, `Setting.VoiceAssistant` |

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
