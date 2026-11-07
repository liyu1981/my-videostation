# Endpoint inventory (tracking)

> **Generated** by `pnpm endpoints` from the route registrations in
> `internal/httpapi/*.go`. Do not hand-edit the tables; add/remove a route in Go
> and re-run the script. This is a tracking document, not a compatibility promise:
> the canonical API is `/api/v1` (see `docs/API.md`), the root Jellyfin paths are
> the client adapter (see `docs/JELLYFIN-COMPAT.md`).

Auth legend:

- **public** — no authentication
- **session** — `mvs_session` cookie (SPA)
- **admin** — session + `admin` role
- **token|session** — Jellyfin token (`X-Emby-Token`, `X-Emby-Authorization`,
  `X-MediaBrowser-Token`, `?api_key=`) **or** the SPA cookie
- **token|session|cast** — additionally a signed `?cast=` token (cast receivers)

Totals: **175** registered routes (plus the SPA fallback at `/`).

## Canonical API — health, config, auth (8)

| Method | Path | Auth |
|---|---|---|
| GET | `/api/v1/health` | public |
| GET | `/api/v1/config` | public |
| GET | `/api/v1/auth/status` | public |
| POST | `/api/v1/auth/setup` | public |
| POST | `/api/v1/auth/login` | public |
| GET | `/api/v1/auth/me` | session |
| POST | `/api/v1/auth/logout` | session |
| POST | `/api/v1/auth/password` | session |

## Canonical API — libraries, scanning, jobs (15)

| Method | Path | Auth |
|---|---|---|
| GET | `/api/v1/libraries` | session |
| POST | `/api/v1/libraries` | admin |
| GET | `/api/v1/libraries/{id}` | session |
| PATCH | `/api/v1/libraries/{id}` | admin |
| DELETE | `/api/v1/libraries/{id}` | admin |
| POST | `/api/v1/libraries/{id}/scan` | admin |
| POST | `/api/v1/libraries/{id}/enrich` | admin |
| GET | `/api/v1/libraries/{id}/facets` | session |
| GET | `/api/v1/libraries/{id}/folders` | session |
| GET | `/api/v1/jobs` | admin |
| GET | `/api/v1/jobs/{id}` | admin |
| GET | `/api/v1/jobs/{id}/logs` | admin |
| DELETE | `/api/v1/jobs/{id}` | admin |
| DELETE | `/api/v1/jobs` | admin |
| GET | `/api/v1/fs` | admin |

## Canonical API — browse, items, metadata, artwork (29)

| Method | Path | Auth |
|---|---|---|
| GET | `/api/v1/settings` | admin |
| PUT | `/api/v1/settings` | admin |
| GET | `/api/v1/providers` | admin |
| GET | `/api/v1/artwork/thumbnails` | admin |
| DELETE | `/api/v1/artwork/thumbnails` | admin |
| POST | `/api/v1/artwork/thumbnails/rebuild` | admin |
| POST | `/api/v1/items/batch` | session |
| GET | `/api/v1/home` | session |
| GET | `/api/v1/browse` | session |
| GET | `/api/v1/search` | session |
| GET | `/api/v1/items/{id}` | session |
| PATCH | `/api/v1/items/{id}` | admin |
| GET | `/api/v1/items/{id}/children` | session |
| POST | `/api/v1/items/{id}/rescan` | admin |
| POST | `/api/v1/items/{id}/reclassify` | admin |
| GET | `/api/v1/items/{id}/neighbors` | session |
| GET | `/api/v1/items/{id}/matches` | admin |
| GET | `/api/v1/items/{id}/duplicate` | admin |
| POST | `/api/v1/items/{id}/merge` | admin |
| POST | `/api/v1/items/{id}/keep-separate` | admin |
| POST | `/api/v1/items/{id}/apply-local` | admin |
| POST | `/api/v1/items/{id}/match` | admin |
| POST | `/api/v1/items/{id}/refresh` | admin |
| POST | `/api/v1/items/{id}/art` | admin |
| POST | `/api/v1/items/{id}/art/upload` | admin |
| POST | `/api/v1/items/{id}/progress` | session |
| GET | `/api/v1/items/{id}/poster` | session |
| GET | `/api/v1/items/{id}/backdrop` | session |
| GET | `/api/v1/items/{id}/playback` | session |

## Canonical API — collections, users, audit, backup, cast (21)

| Method | Path | Auth |
|---|---|---|
| GET | `/api/v1/users` | admin |
| POST | `/api/v1/users` | admin |
| PATCH | `/api/v1/users/{id}` | admin |
| DELETE | `/api/v1/users/{id}` | admin |
| GET | `/api/v1/users/{id}/libraries` | admin |
| PUT | `/api/v1/users/{id}/libraries` | admin |
| GET | `/api/v1/audit` | admin |
| POST | `/api/v1/cast/dlna/discover` | session |
| POST | `/api/v1/cast/dlna/play` | session |
| GET | `/api/v1/backup/status` | public |
| GET | `/api/v1/backup/exports` | admin |
| POST | `/api/v1/backup/export` | admin |
| GET | `/api/v1/backup/imports` | admin |
| POST | `/api/v1/backup/import` | admin |
| GET | `/api/v1/collections` | session |
| POST | `/api/v1/collections` | session |
| PATCH | `/api/v1/collections/{id}` | session |
| DELETE | `/api/v1/collections/{id}` | session |
| GET | `/api/v1/collections/{id}/items` | session |
| POST | `/api/v1/collections/{id}/items` | session |
| DELETE | `/api/v1/collections/{id}/items/{itemId}` | session |

## Canonical API — playback, streaming, transcoding (7)

| Method | Path | Auth |
|---|---|---|
| GET | `/api/v1/media/{fileId}` | token\|session\|cast |
| POST | `/api/v1/playback/{itemId}/transcode` | session |
| GET | `/api/v1/playback/sessions/{session}` | session |
| DELETE | `/api/v1/playback/sessions/{session}` | session |
| GET | `/api/v1/playback/capabilities` | session |
| GET | `/hls/{session}/{file...}` | token\|session |
| GET | `/api/v1/media/{fileId}/subtitle` | session |

## Jellyfin — discovery, auth, users, views (20)

| Method | Path | Auth |
|---|---|---|
| GET | `/System/Info/Public` | public |
| GET | `/System/Info` | token\|session |
| GET | `/System/Ping` | public |
| GET | `/Branding/Configuration` | public |
| GET | `/QuickConnect/Enabled` | public |
| POST | `/QuickConnect/Initiate` | public |
| GET | `/QuickConnect/Connect` | public |
| POST | `/QuickConnect/Authorize` | public |
| GET | `/Users/Public` | public |
| POST | `/Users/AuthenticateByName` | public |
| GET | `/Users/Me` | token\|session |
| GET | `/Users/{userId}` | token\|session |
| GET | `/Users/{userId}/Views` | token\|session |
| GET | `/UserViews` | token\|session |
| GET | `/DisplayPreferences/{name}` | token\|session |
| GET | `/Localization/Cultures` | token\|session |
| POST | `/DisplayPreferences/{name}` | token\|session |
| GET | `/Localization/Countries` | token\|session |
| GET | `/Localization/Options` | token\|session |
| GET | `/Localization/ParentalRatings` | token\|session |

## Jellyfin — images (anonymous, opaque ids) (7)

| Method | Path | Auth |
|---|---|---|
| GET | `/Users/{userId}/Images/{imageType}` | public |
| GET | `/Items/{itemId}/Images/{imageType}` | public |
| GET | `/Items/{itemId}/Images/{imageType}/{index}` | public |
| GET | `/items/{itemId}/Images/{imageType}` | public |
| GET | `/items/{itemId}/Images/{imageType}/{index}` | public |
| GET | `/UserImage` | public |
| GET | `/Items/{itemId}/Images` | token\|session |

## Jellyfin — playback (5)

| Method | Path | Auth |
|---|---|---|
| GET | `/Videos/{itemId}/AdditionalParts` | token\|session |
| GET | `/Items/{itemId}/PlaybackInfo` | token\|session |
| POST | `/Items/{itemId}/PlaybackInfo` | token\|session |
| GET | `/Videos/{itemId}/stream` | public |
| GET | `/Videos/{itemId}/{rest...}` | token\|session |

## Jellyfin — progress, sessions, played/favorites, devices (20)

| Method | Path | Auth |
|---|---|---|
| GET | `/UserItems/{itemId}/UserData` | token\|session |
| POST | `/UserItems/{itemId}/UserData` | token\|session |
| POST | `/UserPlayedItems/{itemId}` | token\|session |
| DELETE | `/UserPlayedItems/{itemId}` | token\|session |
| POST | `/Devices/Options` | token\|session |
| GET | `/Sessions` | token\|session |
| POST | `/Sessions/Logout` | token\|session |
| POST | `/Sessions/Capabilities` | token\|session |
| POST | `/Sessions/Capabilities/Full` | token\|session |
| GET | `/socket` | token\|session |
| POST | `/Sessions/Playing` | token\|session |
| POST | `/Sessions/Playing/Progress` | token\|session |
| POST | `/Sessions/Playing/Stopped` | token\|session |
| POST | `/Sessions/Playing/Ping` | token\|session |
| POST | `/Users/{userId}/PlayedItems/{itemId}` | token\|session |
| DELETE | `/Users/{userId}/PlayedItems/{itemId}` | token\|session |
| POST | `/Users/{userId}/FavoriteItems/{itemId}` | token\|session |
| DELETE | `/Users/{userId}/FavoriteItems/{itemId}` | token\|session |
| POST | `/UserFavoriteItems/{itemId}` | token\|session |
| DELETE | `/UserFavoriteItems/{itemId}` | token\|session |

## Jellyfin — series, seasons, episodes (5)

| Method | Path | Auth |
|---|---|---|
| GET | `/Shows/{itemId}/NextUp` | token\|session |
| GET | `/Shows/Upcoming` | token\|session |
| GET | `/Shows/NextUp` | token\|session |
| GET | `/Shows/{itemId}/Seasons` | token\|session |
| GET | `/Shows/{itemId}/Episodes` | token\|session |

## Jellyfin — items, browse, facets, search (32)

| Method | Path | Auth |
|---|---|---|
| GET | `/Users/{userId}/Items` | token\|session |
| GET | `/Users/{userId}/Items/{itemId}` | token\|session |
| GET | `/Users/{userId}/Items/{itemId}/LocalTrailers` | token\|session |
| GET | `/Users/{userId}/Items/{itemId}/Intros` | token\|session |
| GET | `/MediaSegments/{itemId}` | token\|session |
| GET | `/Items` | token\|session |
| GET | `/Items/{itemId}` | token\|session |
| GET | `/Search/Hints` | token\|session |
| GET | `/Items/Suggestions` | token\|session |
| GET | `/Items/Resume` | token\|session |
| GET | `/Users/{userId}/Items/Resume` | token\|session |
| GET | `/Items/Latest` | token\|session |
| GET | `/Users/{userId}/Items/Latest` | token\|session |
| GET | `/Genres` | token\|session |
| GET | `/Users/{userId}/Genres` | token\|session |
| GET | `/Persons` | token\|session |
| GET | `/Studios` | token\|session |
| GET | `/Items/Filters` | token\|session |
| GET | `/Items/Filters2` | token\|session |
| GET | `/UserItems/Resume` | token\|session |
| GET | `/Items/{itemId}/Intros` | token\|session |
| GET | `/Items/{itemId}/Similar` | token\|session |
| GET | `/Items/{itemId}/Ancestors` | token\|session |
| GET | `/Items/{itemId}/InstantMix` | token\|session |
| GET | `/Items/{itemId}/ThemeMedia` | token\|session |
| GET | `/Items/{itemId}/LocalTrailers` | token\|session |
| GET | `/Items/{itemId}/SpecialFeatures` | token\|session |
| GET | `/Items/{itemId}/ThemeSongs` | token\|session |
| GET | `/Items/{itemId}/ThemeVideos` | token\|session |
| GET | `/Items/{itemId}/Download` | token\|session |
| DELETE | `/Items/{itemId}` | token\|session |
| POST | `/Items/{itemId}/RemoteSearch/Subtitles/{subtitleId}` | token\|session |

## Other (6)

| Method | Path | Auth |
|---|---|---|
| POST | `/Users/AuthenticateWithQuickConnect` | public |
| POST | `/UserItems/{itemId}/Rating` | token\|session |
| DELETE | `/UserItems/{itemId}/Rating` | token\|session |
| POST | `/Users/Configuration` | token\|session |
| GET | `/Artists` | token\|session |
| GET | `/Artists/AlbumArtists` | token\|session |

## Dispatch details

- `GET /Videos/{itemId}/{rest...}` handles `master.m3u8` (HLS manifest),
  `stream.<ext>` and `<source>/Subtitles/<index>/Stream.<format>`; anything else is 404.
- `GET /socket` is reserved for the Jellyfin realtime socket and returns `400`
  (no WebSocket API yet).
- `GET /` and unknown non-API paths serve the embedded React SPA.
- Lowercase `/items/{itemId}/Images/...` are aliases kept for Findroid, which
  builds image URLs with a lowercase prefix.
