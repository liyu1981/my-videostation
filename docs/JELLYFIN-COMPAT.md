# Jellyfin Player Compatibility Plan

> Goal: let existing Jellyfin clients (mobile apps, Android TV / Google TV, web) play
> content served by MyVideoStation, without shipping native TV apps. This closes
> GAP-030/031 cheaply (see `GAPS.md`).

Status: **research + plan, not yet implemented**. Prerequisites are now met:
M5 (HLS transcode sessions) and M6 (accounts/sessions) shipped, so phase J1 can start
whenever the decision below is confirmed.

---

## 1. Why this path

Native TV apps are a `GAP-NEVER` (v1) non-goal. Reimplementing ExoPlayer-based
Android TV clients is out of scope. Jellyfin already ships mature mobile + TV
clients. If we expose a *subset* of the Jellyfin server API that those clients
understand, their players work against our library with no client changes.

This is an **adapter over our own API**, not a replacement of it (ADR D-1).
Our canonical API stays `/api/v1`. The Jellyfin-shaped routes live under a
separate prefix (e.g. `/jellyfin/...` or the root paths Jellyfin clients expect)
and translate to internal services.

---

## 2. Protocols involved

### 2.1 Jellyfin REST API (primary)

Clients discover and browse via JSON REST. Minimum surface needed for playback:

- **Server info / discovery**
  - `GET /System/Info/Public` → server name, version, id, startup wizard flag.
  - `GET /System/Info` (auth) → full info.
  - `GET /Health` or root → 200.
- **Auth**
  - `POST /Users/AuthenticateByName` `{ Username, Pw }` → `{ User, AccessToken, ServerId }`.
  - Header `X-Emby-Authorization: MediaBrowser Client=..., Device=..., DeviceId=..., Version=..., Token=...`.
  - `GET /Users/Me` or `/Users/{id}`.
- **Library browsing**
  - `GET /Items` with query params: `ParentId`, `IncludeItemTypes`, `Recursive`,
    `SortBy`, `SortOrder`, `StartIndex`, `Limit`, `SearchTerm`, `Genres`, `Years`,
    `Filters`, `Fields`, `ImageTypeLimit`, `EnableImageTypes`.
  - `GET /Items/{id}` → item detail with provider ids, media sources, chapters.
  - `GET /Users/{userId}/Items/{id}` variants (some clients use these).
  - `GET /Items/{id}/Images/{type}` (Primary, Backdrop, Logo) with `maxWidth`/`quality`.
  - `GET /Items/{id}/Images` list.
- **Playback info**
  - `GET /Items/{id}/PlaybackInfo` → `MediaSources[]` with `Id`, `Container`,
    `MediaStreams[]` (video/audio/subtitle codec, language, index), `SupportsDirectPlay`,
    `SupportsDirectStream`, `SupportsTranscoding`, `TranscodingUrl`, `DirectStreamUrl`.
- **Streaming**
  - Direct play: `GET /Videos/{id}/stream?static=true&mediaSourceId=...&api_key=...`
    (byte-range supported).
  - HLS transcode: `GET /Videos/{id}/master.m3u8?...` and segment endpoints
    `/Videos/{id}/hls1/...` (Jellyfin's path shape). Our M5 HLS pipeline must be
    able to serve these URLs.
  - Subtitles: `GET /Videos/{id}/{mediaSourceId}/Subtitles/{index}/...`.
- **Playback reporting** (so resume/watched state syncs)
  - `POST /Sessions/Playing` (start), `/Sessions/Playing/Progress` (position),
    `/Sessions/Playing/Stopped`.
  - Maps to our `playback_progress` table.

### 2.2 Discovery

- **SSDP / UPnP** multicast (`239.255.255.250:1900`) advertising a device
  description XML. Jellyfin clients on the same LAN auto-discover. Optional but
  high value for TV. Can be added later; manual server URL entry always works.
- **Jellyfin's `/System/Info/Public`** is what clients hit to validate a URL.

### 2.3 Chromecast (separate)

Jellyfin has a custom Cast receiver (`receiver.jellyfin.org`). To support "cast to
Chromecast" from a Jellyfin client, we'd either need their receiver (which calls
Jellyfin APIs → could work if our adapter is complete) or our own receiver. Out of
scope for v1; our own web Chromecast (GAP-022) covers browser casting.

### 2.4 Google TV / Android TV clients

Jellyfin's Android TV client uses the standard Jellyfin API above plus
`/Items/{id}/RemoteImages`? and `/Playlists`. It relies heavily on `PlaybackInfo`
and HLS. It does *not* need DLNA. So if §2.1 is implemented + M5 transcode works,
TV playback works.

---

## 3. Mapping to our domain

| Jellyfin concept | Our concept | Notes |
|---|---|---|
| User / AccessToken | `users` + session token (M6) | Reuse our auth; issue a Jellyfin-shaped token wrapping our session. |
| Item (Movie/Series/Episode) | `media_items` (+ type) | Map `Type`: Movie, Series, Season, Episode. |
| MediaSource | `media_files` + ffprobe streams | One source per file (or per version). |
| MediaStream | `media_streams` (M4/M5) | codec/lang/index. |
| Image | artwork cache (`/data/cache`) | Serve bytes; respect `maxWidth` via on-the-fly resize or pre-generated variants. |
| Playback position | `playback_progress` | Already per-user. |
| Genres/People | `genres`, `people` | Straight mapping. |

---

## 4. Implementation plan

### Phase J0 — Prerequisites (done)
- M5 transcode/HLS pipeline complete and stable (`docs/adr/0003-hls-transcoding.md`).
- M6 auth/users complete.
- Media stream model (codecs) persisted (M4/M5).

### Phase J1 — Read-only discovery + auth shim (small)
1. Add route group `/jellyfin` (or root aliases) in `internal/httpapi`.
2. Implement `/System/Info/Public`, `/System/Info`, `/Health`.
3. Implement `/Users/AuthenticateByName` delegating to our auth; return Jellyfin-shaped
   token. Store mapping token→our session.
4. Add middleware parsing `X-Emby-Authorization` / `api_key` query.
5. `GET /Items/{id}`, `GET /Items` (basic filtering, pagination) for movies/series/episodes.
6. Images endpoint serving artwork.

Deliverable: a Jellyfin client can log in and browse library + see posters.

### Phase J2 — Playback (M5 landed; adapter still needed)
1. `GET /Items/{id}/PlaybackInfo` building MediaSources from our streams.
2. Direct-play URL issuance (signed, byte-range capable) matching `/Videos/{id}/stream`.
3. HLS: adapt our M5 master playlist to Jellyfin's `/Videos/{id}/master.m3u8` + segment
   paths; or redirect. Prefer adapting paths to avoid client quirks.
4. Subtitle endpoints.

Deliverable: playback works in Jellyfin mobile + TV for direct-play and transcode cases.

### Phase J3 — Progress sync
1. `/Sessions/Playing*` endpoints updating `playback_progress`.
2. Map client's `PositionTicks` (100ns units) ↔ our ms.

### Phase J4 — Discovery & polish
1. SSDP advertiser (optional).
2. Search (`SearchTerm`) wiring to our FTS.
3. Collections/playlists (if we add them).
4. Version negotiation: report a compatible server version string.

---

## 5. Risks / open questions

- **Client version skew**: Jellyfin clients pin minimum server versions. We may need to
  spoof `Version` and implement enough of `/System/Info` to pass client gating.
  `[verify]` against real clients.
- **HLS path shapes**: clients construct segment URLs from `PlaybackInfo`'s
  `TranscodingUrl`. We control that string, so we can point at our own paths — no need
  to mimic Jellyfin's exactly, as long as relative/absolute URLs are valid. Verify with
  an actual Android TV client.
- **Auth token format**: some clients expect JWT-like or specific; Jellyfin uses opaque
  tokens. Our opaque session token should be acceptable.
- **Image resizing**: clients request `maxWidth`. Serving originals is fine but wasteful;
  add resize later (M7).
- **Scope creep**: do not implement Jellyfin's full API. Only what playback clients need.
  Track implemented endpoints in this doc.
- **Security**: token in URL (`api_key`) for media streaming is inherent to Jellyfin
  clients; ensure tokens are short-lived / scoped and HTTPS recommended. Reuse our
  signed URL mechanism where possible.

---

## 6. Testing strategy

- Unit tests for DTO mapping (Jellyfin JSON shapes).
- Integration test: spin server, hit `/System/Info/Public`, auth, list items, get
  `PlaybackInfo`, fetch a stream byte range.
- Manual verification matrix: Jellyfin mobile (iOS/Android), Jellyfin Android TV on
  Google TV, Jellyfin web client pointed at our server. Record results in GAPS.md.
- Do not commit credentials; use env vars for any real-instance probes.

---

## 7. Non-goals

- DLNA/UPnP media server (GAP-023, `GAP-NEVER`) — separate spec; Jellyfin clients
  don't need it.
- Implementing Jellyfin's plugin system, webhooks, or sync play.
- Replacing our own API or UI.

---

## 8. Decision needed

Confirm D-1 (yes, adapter after M5). If approved, schedule J1 after M6 and add a
tracking row to `GAPS.md` (GAP-031 → `GAP-LATER`/M7, note "Jellyfin API adapter").
No code changes in this commit.
