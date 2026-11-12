# Jellyfin Player Compatibility Plan

> Goal: let existing Jellyfin clients (mobile apps, Android TV / Google TV) play
> content served by MyVideoStation, without shipping native TV apps. This closes
> GAP-030/031 cheaply (see `GAPS.md`).

Status: **J0.5 + J1 + J1.5 + J2/J3 delivered** — discovery, token auth, users,
views, browse with the full list-query surface, browse facets, home rows
(`Resume`/`Latest`/`NextUp`), detail with series image fallbacks, HLS playback,
direct streaming, progress sync and played/favorite mutations. Next: J4 polish
(device profiles, playlists, SSDP) and a real-client verification pass.
Prerequisites (M5 HLS sessions, M6 accounts/sessions) are met; no hard blockers
remain. Target clients and the claimed server version are pinned in §10; the
endpoint matrix in §11 is the working checklist.

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

### Phase J0.5 — Core enablers (delivered 2026-11-04)
1. ✅ Token auth middleware: `X-Emby-Authorization`, `X-Emby-Token`, `?api_key=` accepted
   on adapter routes, `/hls/*` and `/api/v1/media/*` (native clients have no cookie).
2. ✅ `internal/jellyfin`: reversible 32-hex ID codec (user/library/item/file), stable
   server ID, DTOs.
3. ✅ `/System/Info/Public`, `/System/Info`, `/Branding/Configuration`, `/QuickConnect/Enabled`.
4. ✅ `POST /Users/AuthenticateByName`, `GET /Users/Public`, `GET /Users/Me`, `GET /Users/{id}`.
5. ✅ `GET /Users/{id}/Views`, `GET /Users/{id}/Items`, `GET /Items`, `GET /Items/{id}`,
   image endpoints, `GET /Search/Hints`.

Deliverable (HTTP-level): discovery, login, library list, item browse and artwork
work. Playback followed in J2/J3; **verification against a real client build is
still outstanding** `[verify]`.

### Phase J1 — Read-only discovery completeness (delivered 2026-11-04)
1. ✅ `ExcludeItemTypes`, `Filters=IsFavorite/IsResumable`, multiple `Genres`/`Years`,
   `GenreIds`, `PersonIds`, `OfficialRatings`, `IsFolder`; `SortBy` uses the first
   recognised key and episode/season lists are ordered by number.
2. ✅ `GET /Items/Resume`, `GET /Items/Latest`, `GET /Shows/{id}/NextUp` (also under
   `/Users/{id}/Items/...`), powering TV home rows.
3. ✅ `GET /Genres`, `GET /Persons`, `GET /Items/Filters2`, `GET /Items/Filters`;
   `GET /Studios` returns empty (we do not model studios).
4. ✅ `GET /Items/{id}/Similar`, `ThemeMedia`, `LocalTrailers`, `SpecialFeatures` and
   `GET /Items/{id}/Images`.
5. ⬜ Collection/folder browsing edges (box sets); our model has collections but no
   Jellyfin box-set views yet.

### Phase J1.5 — Detail pages (delivered 2026-11-04)
1. ✅ Series → seasons → episodes tree with `SeriesId`/`SeasonId`/`IndexNumber`.
2. ✅ `People`, `ProviderIds`, `OfficialRating`, `CommunityRating`, `ImageTags`.
3. ✅ User data (`Played`, `PlaybackPositionTicks`, `PlayCount`, `IsFavorite`).
4. ✅ Series image fallbacks on episode rows (`SeriesPrimaryImageTag`,
   `ParentBackdropItemId`/`ParentBackdropImageTags`), `ChildCount` on list rows.
   `Logo`/`Banner`/`Art` stay 404: our metadata model has poster/backdrop only.

### Phase J2 — Playback (delivered 2026-11-04)
1. ✅ `GET/POST /Items/{id}/PlaybackInfo` building MediaSources from our streams; the
   request's `DeviceProfile` is **ignored** initially (we transcode unless our own
   direct-play matrix says native) — see §10.
2. ✅ Stable manifest URL `/Videos/{id}/master.m3u8?...&StartTimeTicks=...` that starts or
   reuses an M5 session at the requested offset and returns the playlist with segment
   URIs rewritten to absolute `/hls/{session}/...` paths (no redirects).
3. ✅ Direct-play URL `/Videos/{id}/stream?static=true&api_key=...` (range capable).
4. ✅ Subtitle tracks: `DeliveryUrl` for text tracks (side-loadable), and any requested
   track is burned in (embedded text extracted to WebVTT, bitmap via overlay).

Deliverable: playback works at the HTTP level for direct-play and transcode cases;
real-client playback is still `[verify]`.

### Phase J3 — Progress sync (delivered 2026-11-04)
1. ✅ `/Sessions/Playing[/Progress|/Stopped|/Ping]` updating `playback_progress`;
   `Stopped` marks played at ≥95% and stops the session.
2. ✅ `PositionTicks` (100 ns) ↔ ms.
3. ✅ Played/favorite mutations (`/PlayedItems/{id}`, `/FavoriteItems/{id}`).

### Phase J4 — Discovery & polish (remaining)
1. ⬜ SSDP advertiser (optional; manual server URL always works).
2. ✅ Search wired to our FTS (`/Search/Hints`, `SearchTerm`).
3. ⬜ Collections/playlists endpoints (`/Playlists`, box-set views).
4. ✅ Version negotiation: pinned `12.4.0` (§10).
5. ⬜ Device-profile-aware playback decisions (stop transcoding files the client can
   direct-play); bitrate ladder.
6. ⬜ Real-client verification matrix (Jellyfin Android, Android TV, iOS) `[verify]`.

---

## 5. Risks / open questions

- **Client version skew**: Jellyfin clients pin minimum server versions. We claim a
  pinned version (§10) and test against pinned client builds. `[verify]` against real
  clients — this remains the main ongoing maintenance cost.
- **Device profile**: ignored in v1 of the adapter, so clients that could direct-play
  MKV/HEVC will still get HLS. Costs CPU, buys simplicity; revisit after J2. `[verify]`
  CPU headroom on NAS hardware with two TVs + a phone.
- **Transcode session cap**: `MVS_TRANSCODE_SESSIONS=2` is too low for family use; raise
  to 3–4 (or per-token caps) when J2 ships. Session eviction is LRU by segment
  request, so an actively playing session is not evicted first.
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

---

## 9. Readiness audit (2026-11-04)

No hard blockers. Everything the adapter needs already exists; the five enablers in
§3 (J0.5) are small and adapter-local except the token middleware.

| Need | Existing implementation | Adapter work |
|---|---|---|
| Mint an `AccessToken` from username/password | `auth.VerifyPassword` + `store.CreateSession(tokenHash)` + `auth.NewSessionToken()` | `POST /Users/AuthenticateByName` returns the raw token; no schema change |
| Token auth for native clients | Cookie sessions only (`s.auth`); `/hls` is cookie-auth; `/api/v1/media` is cookie or signed `?cast=` | Flexible middleware (cookie **or** token) for adapter routes, `/hls/*`, `/api/v1/media/*` |
| Per-user permissions enforced in queries | `store.WithUserRole/LibraryRestricted/Rating` → `accessScope` in every item query | Set the same context from the token's user |
| Stream facts (codecs, indices) | `media_files.container/video_codec/audio_streams/subtitle_streams`; playback DTO | DTO mapping only |
| Transcode with audio/subtitle/quality/seek | M5 `POST /playback/{id}/transcode` (`audioIndex`, `subtitleIndex`, `maxHeight`, `startMs`) | Session-backed manifest URL |
| Artwork + stable tags | `/items/{id}/poster|backdrop`, `internal/artwork` thumbnails, source mtime | `ImageTags` from path+mtime hash |
| Resume / watched / next-up / continue watching | `playback_progress`, `NextEpisode`, `ContinueWatching` | Ticks conversion |
| Browse/filter/paginate/search | `store.ListItems`, FTS5 `store.Search` | Param translation |
| Durable jobs | `jobs` queue + progress | Not needed for playback |

## 10. Pinned targets & compatibility contract

- **Claimed server version**: `12.4.0` (current Jellyfin 12.x line). Returned by
  `/System/Info` and `/System/Info/Public`. It is a compatibility claim, not a real
  Jellyfin build. **This is a hard gate**: `jellyfin-sdk-kotlin` ≥ 1.9.x (used by
  Findroid, the Android TV app and Streamyfin) has `minimumVersion = 12.0.0`, so a
  10.x claim is rejected with "server version is too low". Bump deliberately and
  re-run the client matrix.
- **Product name**: `"Jellyfin Server"`, exactly. The SDK's
  `RecommendedServerDiscovery` marks any other `productName` as `BAD` and reports
  "Unsupported version or product", so this is a hard protocol constant.
  `ServerName` remains `MyVideoStation` (shown in the client UI).
- **Target clients (test matrix)**: native clients only — Jellyfin **Android TV**
  (Google TV) and **Wholphin** (Android TV, `jellyfin-sdk-kotlin` 1.7.1),
  Jellyfin **iOS/Swiftfin**, native phone apps such as **Findroid**/**Streamyfin**
  (`jellyfin-sdk-kotlin` 1.9.x), and **Moonfin** (Flutter; its own Dart API layer,
  Jellyfin 10.8+). All talk to the REST API directly.
  **The Jellyfin Android phone app is not usable against us**: it is a WebView shell
  (`WebViewFragment.loadUrl("${hostname}/")` plus JS bridges) that expects the
  **Jellyfin web client** to be served at `/`. We serve our own SPA there. Supporting
  it would mean bundling/serving jellyfin-web (GPL, huge surface) — out of scope for
  v1. The Jellyfin **web** client itself is out of scope for the same reason.
- **ID format**: 32 lowercase hex, reversible (`kind` + `id`). Clients treat IDs as
  opaque, so this is stable across restarts and does not leak our numeric ids.
- **Auth**: opaque opaque session token (same format as our cookie session), accepted
  via `X-Emby-Authorization`, `X-Emby-Token`, or `?api_key=`/`?ApiKey=`. 30-day
  expiry. Plain HTTP on a LAN means the password crosses the wire on login — document
  HTTPS/reverse proxy; identical trade-off to Jellyfin itself.
- **Out of scope**: plugin/config UI endpoints, playlist management, live TV, sync,
  QuickConnect (reports disabled: `GET /QuickConnect/Enabled` → `false`,
  `POST /QuickConnect/Initiate` → 401), lyrics, theme-media indexing, HDHR.
- **Reserved paths**: adapter routes are mounted at the root (`/System`, `/Users`,
  `/Items`, `/Items/{id}/Images`, ...). They must never collide with SPA routes; the
  SPA catch-all remains the fallback.

## 11. Endpoint coverage matrix (J0.5 scope)

`[ ]` = not implemented, `[x]` = implemented in this slice.

| Endpoint | Purpose |
|---|---|
| `[x] GET /System/Info/Public` | discovery; server id/name/version |
| `[x] GET /System/Info` | full info (auth) |
| `[x] GET /Branding/Configuration` | login screen branding defaults |
| `[x] GET /QuickConnect/Enabled` | `false` (QuickConnect is not implemented) |
| `[x] GET /Users/Public` | login-screen profile list |
| `[x] POST /Users/AuthenticateByName` | username/password → token + user |
| `[x] GET /Users/Me` | current user (auth) |
| `[x] GET /Users/{userId}` | one user (auth) |
| `[x] GET /Users/{userId}/Views` | libraries as `CollectionFolder`s |
| `[x] GET /Users/{userId}/Items` | browse/search list |
| `[x] GET /Items` | browse/search list (alternate path) |
| `[x] GET /Items/{itemId}` | item detail |
| `[x] GET /Users/{userId}/Items/{itemId}` | user-scoped detail (Moonfin) |
| `[x] GET /Users/{userId}/Items/{itemId}/LocalTrailers`, `/Intros` | user-scoped relation rows |
| `[x] GET /Items/{itemId}/Ancestors` | parent chain (series/season) |
| `[x] GET /Items/{itemId}/Images/{type}` | poster/backdrop (thumbnails) |
| `[x] GET /Items/{itemId}/Images/{type}/{index}` | poster/backdrop variants |
| `[x] GET /Search/Hints` | search suggestions |
| `[x] GET/POST /Items/{itemId}/PlaybackInfo` | MediaSources + TranscodingUrl/DirectStreamUrl + PlaySessionId |
| `[x] GET /Videos/{itemId}/master.m3u8` | session-backed HLS manifest with token-bearing segment URLs |
| `[x] GET /Videos/{itemId}/stream[.ext]` | direct play with byte ranges |
| `[x] GET /Videos/{itemId}/{source}/Subtitles/{index}/Stream.{format}` | redirect to our WebVTT endpoint |
| `[x] POST /Sessions/Playing`, `/Progress`, `/Stopped`, `/Ping` | ticks → `playback_progress`; stops the session on `Stopped` |
| `[x] POST/DELETE /Users/{id}/PlayedItems/{itemId}` | watched state |
| `[x] POST/DELETE /Users/{id}/FavoriteItems/{itemId}`, `/UserFavoriteItems/{itemId}` | favorites via the system collection |
| `[x] GET /Shows/{itemId}/Episodes` | episode list per series/season |
| `[x] GET /Items/Resume`, `/Items/Latest` (+ `/Users/{id}/Items/...`) | continue watching, recently added |
| `[x] GET /Shows/{itemId}/NextUp` | next episode after the last watched/in-progress one |
| `[x] GET /Genres`, `/Persons`, `/Studios`, `/Items/Filters`, `/Items/Filters2` | filter screens (genre/person ids round-trip) |
| `[x] GET /Items/{itemId}/Similar`, `/ThemeMedia`, `/LocalTrailers`, `/SpecialFeatures` | relation rows and stubs |
| `[x] GET /Items/{itemId}/Images` | available image list |
| `[x] GET /Shows/NextUp` | home-row next up (optionally `seriesId`-scoped) |
| `[x] GET /Shows/{seriesId}/Seasons` | series detail season list |
| `[x] GET /Shows/Upcoming` | empty result (no air-date tracking) |
| `[x] GET /Videos/{itemId}/AdditionalParts`, `/Items/{itemId}/Intros` | empty results |
| `[x] POST /DisplayPreferences/{id}` | accepted and ignored |
| `[x] GET /UserItems/Resume` | SDK alias for the resume row |
| `[x] GET /socket` | 400 (no WebSocket API yet) |
| `[x] GET /Items/{id}/ThemeSongs`, `/ThemeVideos` | empty `ThemeMediaResult` (no theme media indexing) |
| `[x] GET /Localization/Cultures` | small language list for settings |
| `[x] GET /UserImage` | 404 (no user avatars) |
| `[x] GET /Items/{id}/Download` | original file as attachment |
| `[x] POST /Items/{id}/RemoteSearch/Subtitles/{id}` | 501 (no online subtitle providers) |
| `[x] DELETE /Items/{id}` | 403 (media folders are read-only) |
| `[x] GET /Items` params | `isPlayed`, `isUnplayed`, `isFavorite`, `isResumable`, `excludeItemIds`, `nameStartsWith`, `nameLessThan`; pipe-separated `Genres`/`OfficialRatings`; `Resume` honours `includeItemTypes`/`excludeItemTypes` |
| `[x] GET /UserItems/{itemId}/UserData` | user data (played/resume/favorite) |
| `[x] POST /UserItems/{itemId}/UserData` | update played/position/favorite |
| `[x] POST/DELETE /UserItems/{itemId}/Rating` | accepted (likes are not stored separately from favourites) |
| `[x] GET /System/Ping` | unauthenticated reachability probe |
| `[x] POST /QuickConnect/Initiate`, `/Connect`, `/Authorize`, `/Users/AuthenticateWithQuickConnect` | 401 (disabled), so clients fall back to the password form |
| `[x] POST /Sessions/Logout` | revokes the caller's token |
| `[x] POST /Users/Configuration` | accepted (our settings stay under `/api/v1`) |
| `[x] GET /Localization/Countries`, `/Options`, `/ParentalRatings` | empty lists (settings/admin screens) |
| `[x] GET /Artists`, `/Artists/AlbumArtists`, `/Items/{itemId}/InstantMix` | empty results (no music library) |
| `[x] POST/DELETE /UserPlayedItems/{itemId}` | SDK played/unplayed aliases |
| `[x] GET /MediaSegments/{itemId}` | empty (no intro/outro detection) |
| `[x] POST /Devices/Options` | accepted and ignored |
| `[x] GET /Items/{id}/Images/*`, `/Videos/{id}/stream` | **anonymous** artwork/static stream (opaque ids) |

J0.5 also ships `GET /UserViews`, `GET /DisplayPreferences/{name}`,
`GET /Sessions` (empty) and `POST /Sessions/Capabilities/Full` (204).

Later slices: `PlaybackInfo`, `/Videos/*`, `/Sessions/Playing*`, `/Shows/*`,
`/Items/Resume`, `/Items/Latest`, favorites/played mutations, SSDP.

### J2/J3 implementation notes

- **PlaybackInfo** returns `MediaSources` with `SupportsDirectPlay` (our
  direct-play matrix), `SupportsTranscoding` (ffmpeg present), a
  `DirectStreamUrl` and an HLS `TranscodingUrl`. `DeviceProfile` is still ignored
  (ADR 0005). **Delivery is changing under ADR 0006:** once a cached MP4 variant
  is ready it becomes the `DirectStreamUrl`; HLS stays as the transitional
  fallback while the variant is prepared, so the client contract does not change.
- **TranscodingUrl** contains only `MediaSourceId`, `PlaySessionId` and
  `api_key`; Jellyfin clients append `StartTimeTicks`/`AudioStreamIndex`/
  `SubtitleStreamIndex` themselves, so the manifest handler defaults them and
  duplicate query keys cannot shadow the client's seek.
- **Manifest**: starts or reuses an M5 session at `StartTimeTicks`, stops the
  previous session for the same `PlaySessionId` (seek churn), then rewrites the
  playlist so `init.mp4`/`segN.m4s` are absolute `/hls/{session}/…?api_key=…`
  URLs. No redirects (ExoPlayer/Safari both resolve relative URIs without the
  query, which would drop the token).
- **Subtitles**: requested text tracks (embedded or sidecar) are extracted to the
  WebVTT cache and burned in via the `subtitles` filter; bitmap tracks use the
  overlay path. `DeliveryUrl` is still exposed so clients may side-load instead.
- **Progress**: ticks (100 ns) ↔ ms; `Stopped` marks played at ≥95% and kills the
  session bound to the `PlaySessionId`. Played/favorite mutations write to
  `playback_progress` and the user's `favorites` system collection.
- **J1/J1.5 notes**: list filters now cover `ExcludeItemTypes`, `IsFavorite`,
  `IsResumable`, `GenreIds`/`PersonIds`, multiple genres/years/ratings and
  `IsFolder`; list rows batch episode ancestors, child counts and favorite flags
  (no N+1). Episode rows carry `SeriesPrimaryImageTag` and
  `ParentBackdropImageTags` from the series art.
- **Android TV specifics** (scanned from `jellyfin/jellyfin-androidtv`): it uses
  `/UserViews`, `/Items` with `sortBy/includeItemTypes/recursive`, `/Items/Resume`,
  `/Shows/NextUp` (home row, `enableResumable=false`), `/Shows/{id}/Seasons`,
  `/Shows/{id}/Episodes?seasonId=`, `/Items/{id}/PlaybackInfo` (POST with a device
  profile), `/Sessions/Capabilities/Full`, `/DisplayPreferences/{id}`,
  `/Branding/Configuration`, plus relation stubs and a `/socket` WebSocket for push
  events (unsupported; the app functions without it). It reads episode rows with
  `StartIndex = episode number`, so season episode lists are sorted before slicing.
- **Wholphin specifics** (scanned from `damontecres/Wholphin`, SDK 1.7.1):
  `/UserViews` for libraries, `/Items` (bare) for root views, `/Items` with
  `isPlayed`/`excludeItemIds`/multiple sort keys for home rows, `/UserItems/Resume`
  (excluding episodes), `/Shows/NextUp`, `/Shows/{id}/Episodes`, `/Items/{id}` detail,
  `/Items/{id}/ThemeSongs`, `/Persons`, `/Genres`, `/Localization/Cultures`,
  `POST /Items/{id}/PlaybackInfo` + `/Videos/{id}/stream`, `/Sessions/Playing*`,
  `/UserPlayedItems`/`/UserFavoriteItems`, `/DisplayPreferences` and
  `/Items/{id}/Download`. All are covered by `tools/jellyfin-validator/wholphin.main.kts`
  (24/24 checks).
- **Findroid specifics** (scanned from `jarnedemeulemeester/findroid`): it is
  direct-play only, uses `POST /Items/{id}/PlaybackInfo` with a `DeviceProfile`,
  builds the stream URL itself via the SDK (no token → hence the anonymous
  `/Videos/{id}/stream`), loads artwork through Coil (no token → anonymous images),
  and uses `/UserData` + `/UserPlayedItems` for watch state, `/MediaSegments` for
  skip-intro, and `/Devices/Options` for device naming. All are implemented.
- **Moonfin specifics** (scanned from `Moonfin-Client/Moonfin-Core`, Flutter/Dart,
  claims Jellyfin 10.8+): it assumes Quick Connect exists on every Jellyfin server,
  so `POST /QuickConnect/Initiate` must answer **401** (disabled) for the login
  screen to switch to the password form. Detail and relation rows are read through
  user-scoped paths (`/Users/{userId}/Items/{itemId}`,
  `/Users/{userId}/Items/{itemId}/LocalTrailers|Intros`). It sends camelCase query
  names, pipe-separated `Genres`/`OfficialRatings`, and `NameStartsWith`/
  `NameLessThan` for A–Z browsing; all are handled. Likes (`/UserItems/{id}/Rating`)
  are accepted but not stored separately from favourites. Music/live TV/admin
  screens are out of scope; `/Artists`, `/Localization/Countries` etc. answer empty.
  Validated by `tools/jellyfin-validator/moonfin.sh` (71/71 checks).
- **Known gaps before TV polish**: device-profile-aware playback decisions, box
  sets/playlists, SSDP discovery, WebSocket events, and end-to-end verification
  against real client builds `[verify]`.

### J0.5 implementation notes

- **Route mount**: adapter paths are registered on the root mux next to the SPA
  catch-all (`internal/httpapi/jellyfin*.go`); `/api/v1` is untouched.
- **Token auth**: `anyAuth` accepts the SPA cookie *or* a Jellyfin token
  (`X-Emby-Authorization`, `X-Emby-Token`, `X-MediaBrowser-Token`, `?api_key=`,
  `?ApiKey=`). `/hls/*` and `/api/v1/media/*` now accept tokens too, so a client
  can stream and fetch posters with only its `AccessToken`.
- **IDs**: `internal/jellyfin` encodes `(kind, id)` as 32 lowercase hex;
  reversible, stable across restarts, no schema change.
- **Server id**: random 32-hex persisted in `settings` as `jellyfin.server_id`.
- **Browse**: `ParentId` accepts a library or item id; `IncludeItemTypes`,
  `Recursive`, `Filters=IsPlayed/IsUnplayed`, `Genres`, `Years`, `SearchTerm`,
  `SortBy`, `SortOrder`, `StartIndex`, `Limit` are mapped to `store.ListItems`.
  Series→season→episode recursion is assembled in the adapter.
- **Item detail**: MediaSources/MediaStreams are built from `media_files` +
  `external_subtitles` (text subtitles get a `DeliveryUrl`; bitmap tracks are
  marked `DeliveryMethod: Encode` for future burn-in). `UserData` carries
  played/resume state, `ImageTags` a tag derived from art path+mtime.
- **Known J0.5 gaps**: no playback info/stream URLs yet (J2), episode list rows
  omit `SeriesId` (detail resolves it), favorites filters are ignored, no
  `/Genres`/`/Persons` facets, no real device profiles.

## 11.1 Contract validation (dev tool)

`tools/jellyfin-validator/` runs `jellyfin-sdk-kotlin` **1.9.4** (the version
Findroid 1.2.0 ships) against a server and walks the Findroid call sequence:

```bash
cd tools/jellyfin-validator
./run.sh http://192.168.0.221:6500 admin 'password'
```

It reports the exact `MissingFieldException` when a DTO is incomplete, which is
how these client-visible contracts were found and fixed:

| Model | Required fields we had to add |
|---|---|
| `SessionInfoDto` | `LastPlaybackCheckIn`, `SupportsMediaControl`, `SupportsRemoteControl` |
| `PlayerStateInfo` (inside `SessionInfo.PlayState`) | `CanSeek`, `IsPaused`, `IsMuted`, `RepeatMode`, `PlaybackOrder` |
| `UserPolicyDto` | full SDK set incl. `EnableUserPreferenceAccess`, `SyncPlayAccess`, provider ids |
| `UserItemDataDto` | `Key`, `ItemId` |
| `MediaStream` | `IsInterlaced`, `IsHearingImpaired`, `IsOriginal`, `SupportsExternalStream` |
| `MediaSourceInfo` | `Type`, `ReadAtNativeFramerate`, `IgnoreDts`, `IgnoreIndex`, `GenPtsInput`, `IsInfiniteStream`, `RequiresOpening/Closing/Looping`, `SupportsProbing`, `TranscodingSubProtocol`, `HasSegments` |
| `DisplayPreferencesDto` | `RememberIndexing`, `PrimaryImageHeight`, `PrimaryImageWidth`, `CustomPrefs` |
| `SearchHint` | `Artists` |
| `BaseItemPerson` (item `People`) | `Id` (required UUID) |
| `UserDto` (SDK 1.7.1) | `HasConfiguredEasyPassword` (required in 1.7.1, optional in 1.9.4) |
| `/Genres`, `/Persons`, `/Studios` | return `BaseItemDtoQueryResult` objects (`Items`/`TotalRecordCount`/`StartIndex`), not bare arrays |
| Query parameter casing | Jellyfin clients send camelCase (`includeItemTypes`, `genreIds`, …); `url.Values` is case-sensitive, so all adapter query parsing is case-insensitive |
| Lowercase image paths | Findroid builds `items/<id>/Images/Primary?tag=…`, so the adapter mirrors item-image routes for the lowercase prefix |
| Bare `GET /Items` semantics | Findroid's `getLibraries()` reads this as the library list; it must return `CollectionFolder` views with `CollectionType`, not media items (returning movies makes every movie a fake "Mixed" library and its detail page empty) |
| `GET /Items/Suggestions` | Home "Suggestions" row; returns recent unplayed items of the requested kinds (fallback: recent items) |
| Detail `MediaSources` | Must be present and non-empty with `MediaStreams` whose `codec` is set; Findroid's `VideoMetadataParser` calls `codec.lowercase()` and `MovieViewModel` does `sources.first()` (a missing source = blank detail page) |
| Library artwork | Views include `ImageTags`/`BackdropImageTags` taken from a representative item so library cards are not blank |
| `SystemInfo` | `SupportsLibraryMonitor` |

Another contract detail: clients convert our ids to `UUID`s and send them back
**hyphenated** (`00000001-1990-…`), so `Decode` accepts both simple (32 hex) and
hyphenated notation.

## 12. Build order for J0.5

1. Token middleware + `userContext` refactor (core, testable in isolation).
2. `internal/jellyfin` ID codec + server ID + DTO structs (unit tests).
3. Info/auth/user handlers.
4. Views/browse/item/image handlers (reuse `store.ListItems`, `ItemFiles`, `serveArt`).
5. Search hints.
6. Tests: DTO round-trips, auth/token flow, browse mapping, image tags.
7. Manual probe against a real Jellyfin Android client (manual step; record results).
