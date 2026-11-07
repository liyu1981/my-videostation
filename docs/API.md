# HTTP API (v1)

Base path: `/api/v1`. JSON, camelCase. Errors are `{"error":{"code","message"}}` with
matching HTTP status. No pagination cursor yet — pages are numeric.

Public endpoints: `/health`, `/config`, `/auth/status`, `/auth/setup`, `/auth/login`.
Everything else requires the `mvs_session` cookie. Endpoints marked **admin** additionally
require the `admin` role; user accounts can browse, play, edit their own watch state and
manage their own collections. Per-library permissions and content-rating ceilings filter
libraries, browse/home/search results, collections and media access; admins bypass them.
During a backup import, API requests except `/health`, `/config` and `/backup/status` return
`503 maintenance` until the server restarts with the imported data.

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Liveness + DB ping + version |
| GET | `/config` | Version and build info |
| GET | `/auth/status` | `{needsSetup}` — first-run admin setup required |
| POST | `/auth/setup` | First-run `{username,displayName,password}` → `{user}` + session cookie |
| POST | `/auth/login` | `{username,password}` → `{user}` + session cookie |
| GET | `/auth/me` | Current account `{user}` |
| POST | `/auth/logout` | Delete the current session |
| POST | `/auth/password` | Self-service `{currentPassword,newPassword}`; other sessions are signed out |
| GET | `/users` | **admin** — `{users[]}` |
| POST | `/users` | **admin** — `{username,displayName,password,role,birthYear?}` (role `admin`/`user`) |
| PATCH | `/users/{id}` | **admin** — display name, role, active flag, `birthYear`, optional `ratingCeiling` (can only tighten the age limit), optional password reset |
| DELETE | `/users/{id}` | **admin** — delete an account and its watch state/collections |
| GET | `/users/{id}/libraries` | **admin** — `{libraries:[{id,name,allowed}]}` |
| PUT | `/users/{id}/libraries` | **admin** — `{libraryIds[]}` allow-list (unlisted/new libraries denied); `{unrestricted:true}` restores all |
| GET | `/audit?limit=` | **admin** — newest activity entries (`auth`, account, library, settings, metadata) |
| GET | `/backup/status` | Public — `{maintenance, pendingImport}` for the maintenance page |
| GET | `/backup/exports` | **admin** — bundles in `<data>/export` |
| POST | `/backup/export` | **admin** — create a snapshot bundle (DB + cache + manifest) in `<data>/export` |
| GET | `/backup/imports` | **admin** — bundles in `<data>/import` |
| POST | `/backup/import` | **admin** — `{name?}`; creates a safety export, stages the newest/named bundle, enters maintenance and restarts the server |
| POST | `/cast/dlna/discover` | Discover LAN DLNA MediaRenderers → `{devices:[{id,name,controlUrl}]}` |
| POST | `/cast/dlna/play` | `{fileId,controlUrl,title}` — best-effort DLNA `SetAVTransportURI` + `Play`; returns `422 needs_transcoding` for unsupported formats |
| GET | `/settings` | **admin** — provider settings (API key never returned) plus `artwork` (thumbnail + cover options) |
| PUT | `/settings` | **admin** — `{tmdbApiKey?,language?,region?,artwork?:{thumbnails?,posterWidth?,backdropWidth?,quality?,coverPreference?}}`; artwork changes to size/quality clear the thumbnail cache |
| GET | `/artwork/thumbnails` | **admin** — `{files,bytes}` of the disposable thumbnail cache |
| POST | `/artwork/thumbnails/rebuild` | **admin** — clear the cache and enqueue a `thumbnails` job that pre-generates variants → `202 {job}` |
| DELETE | `/artwork/thumbnails` | **admin** — clear the thumbnail cache → `{files,bytes}` |
| GET | `/libraries` | Libraries with paths + item counts |
| POST | `/libraries` | **admin** — create `{name,type,language?,paths[],autoScanMinutes?,watchEnabled?,excludeGlobs?,includeGlobs?,minFileSizeMb?,skipExtras?}`; validates absolute, existing dirs and rule globs; **queues the initial scan** (auto-scan defaults to 360 min, watcher on) |
| GET | `/libraries/{id}` | One library |
| PATCH | `/libraries/{id}` | **admin** — partial update: rename / language / replace paths / `autoScanMinutes` / `watchEnabled` / scan-rule fields (omitted fields are kept; `paths` replaces the list). A change to paths or rules automatically queues a scan |
| DELETE | `/libraries/{id}` | **admin** — remove library and derived data (never touches media files) |
| POST | `/libraries/{id}/scan` | **admin** — enqueue a scan job → `202 {job}`; optional body `{force:true}` re-probes every file |
| POST | `/libraries/{id}/enrich` | **admin** — enqueue metadata matching for unmatched items → `202 {job}` |
| GET | `/jobs?limit=` | **admin** — recent jobs |
| GET | `/jobs/{id}` | **admin** — job status/progress/message |
| GET | `/jobs/{id}/logs?limit=` | **admin** — persisted log lines (`info`/`warn`/`error`) |
| DELETE | `/jobs/{id}` | **admin** — delete one task and its logs |
| DELETE | `/jobs` | **admin** — delete all finished tasks and their logs → `{deleted}` |
| GET | `/home` | `{continue,recent,shelves[]}` for the landing page |
| GET | `/browse` | `?kind=&libraryId=&sort=&page=&perPage=&q=&metadataStatus=&category=&genre=&year=&watched=&folder=` → `{items,page,perPage,total,totalPages}` |
| GET | `/libraries/{id}/facets?kind=` | Genre/year counts for filters |
| GET | `/libraries/{id}/folders` | Folder prefixes with file counts |
| GET | `/fs?path=` | **admin** — read-only server folder browser for the library path picker: `{path,parent?,roots[],entries[]}`; directories only, hides `@eaDir`/`#recycle` |
| GET | `/collections` | Collections (Favorites, Watchlist, user) with item counts |
| POST | `/collections` | `{name}` |
| PATCH | `/collections/{id}` | `{name}` (system collections rejected) |
| DELETE | `/collections/{id}` | Delete a user collection |
| GET | `/collections/{id}/items?page=` | Items in a collection |
| POST | `/collections/{id}/items` | `{itemIds[]}` |
| DELETE | `/collections/{id}/items/{itemId}` | Remove one item |
| POST | `/items/batch` | `{action: watched\|unwatched\|add_collection\|remove_collection, itemIds[], collectionId?}` |
| GET | `/search?q=&limit=` | FTS5 across title, original title, overview, people |
| GET | `/items/{id}` | Detail: item + genres + people + files + locks + children (seasons/episodes), including the current user’s watch state |
| GET | `/items/{id}/children?kind=` | Seasons for a show, episodes for a season |
| GET | `/items/{id}/neighbors` | Previous/next item in browse order (`prev`, `next`) |
| POST | `/items/{id}/reclassify` | **admin** — `{kind: movie\|episode\|home_video, showTitle?, seasonNumber?, episodeNumber?}` — manually assign the item's type. `episode` attaches it to (or creates) a show/season in the same library; the choice is remembered (`kind_locked`) so rescans do not undo it. Clears the stale provider match. Returns the updated detail |
| POST | `/items/{id}/rescan` | **admin** — Queue a folder-scoped rescan (`{force?}`): a show re-walks its own folder (new seasons included), a season/episode/movie its folder. Only files under that folder are re-read; the rest of the library is untouched. `409 no_files` when the item has no files |
| PATCH | `/items/{id}` | **admin** — metadata patch (`title, year, overview, tagline, genres[], locks[], locked, …`) |
| POST | `/items/{id}/progress` | `{positionMs, durationMs, played}` (upsert for the current user) |
| PUT | `/settings` | **admin** — `{tmdbApiKey?, language?, region?}` (empty key clears it) |
| GET | `/items/{id}/matches?q=&year=&language=` | **admin** — Provider candidates, best first; `language` overrides the provider search language (default `en-US`). For episodes and seasons the query searches **TV shows** (default: the parent show's title); home videos are searched as **movies** (so a misclassified episode can still be identified). `year` is only applied when explicitly passed; the item's stored year is not used implicitly |
| POST | `/items/{id}/match` | **admin** — `{providerId, language?, season?, episode?}` — manual match; `language` fetches the provider details in that language (overrides locks). For episodes `providerId` is the **show id**; optional `season`/`episode` select which episode to fetch (default: the item's numbers). For seasons `providerId` is the show id and `season` selects the season |
| POST | `/items/{id}/refresh?language=` | **admin** — Re-fetch from the stored provider id (respects locks); optional language override. Episodes use their stored show id + season/episode |
| GET | `/items/{id}/duplicate` | **admin** — Another same-title item in the library (`{item}`), or `{}` (movies, home videos and shows) |
| POST | `/items/{id}/merge` | **admin** — `{fromId}` — move the other item's files/credits/locks/progress into this item, then delete it; for shows, seasons/episodes are merged by season/episode number |
| POST | `/items/{id}/keep-separate` | **admin** — `{otherId}` — mark both items so scans never auto-merge them |
| POST | `/items/{id}/apply-local` | **admin** — `{sourceId}` — copy another same-kind item's metadata (title/overview/tagline/date/rating/artwork/genres/cast) onto this item; used by "Select from existing videos" |
| POST | `/items/{id}/art` | **admin** — `{kind:"poster"|"backdrop", url}` — download + cache artwork; an empty `url` clears it |
| POST | `/items/{id}/art/upload` | **admin** — multipart `kind=poster|backdrop` + `file` (JPEG/PNG/WebP/GIF, ≤20 MB) — store a user upload in the art cache |
| GET | `/items/{id}/poster` | Cached/sidecar artwork, guarded to library roots |
| GET | `/items/{id}/backdrop` | Sidecar backdrop |
| GET | `/items/{id}/playback` | Player payload: stream URL, signed `cast` URL/compatibility, resume position, subtitle tracks (image tracks flagged `burnIn`), audio tracks (`direct` per track), next episode, and `directPlay`/`hls`/`reasons`/`qualities` for choosing direct play vs HLS |
| GET | `/media/{fileId}` | Direct play, byte ranges (`Range: bytes=`), `206`; signed `?cast=` tokens allow cast devices without a session cookie |
| POST | `/playback/{itemId}/transcode` | Start or reuse an HLS session: `{fileId?,startMs?,audioIndex?,subtitleIndex?,externalSubtitleId?,maxHeight?,mode?}` → `{sessionId,playlistUrl,startMs,durationMs,mode,height,reasons[]}`. Reuses an identical live session; a different `startMs`/audio/burn-in/height starts a new job |
| GET | `/playback/sessions/{sessionId}` | Session status: `{mode,startMs,durationMs,encodedMs,speed,height,running,error?}` |
| DELETE | `/playback/sessions/{sessionId}` | Stop the job and delete its segments (`204`); idle sessions are reaped automatically |
| GET | `/playback/capabilities` | Direct-play codec matrix, available encoders and the quality ladder |
| GET | `/hls/{sessionId}/index.m3u8` | Session playlist (cookie auth; segments follow relative URLs) |
| GET | `/hls/{sessionId}/{init.mp4\|segNNNNN.m4s}` | HLS fMP4 init/segment, byte-range capable |
| GET | `/media/{fileId}/subtitle?ext=ID` | Sidecar subtitle as WebVTT; optional `&delayMs=±n` shifts cue times |
| GET | `/media/{fileId}/subtitle?embedded=INDEX` | ffmpeg-extracted embedded subtitle as WebVTT (cached); optional `&delayMs=±n` |

## Examples

```bash
# create a library (the initial scan is queued automatically)
curl -X POST localhost:6500/api/v1/libraries \
  -H 'Content-Type: application/json' \
  -d '{"name":"Movies","type":"movie","paths":["/mnt/media/movies"]}'

# optional: scan again (e.g. after changing paths)
curl -X POST localhost:6500/api/v1/libraries/1/scan
curl localhost:6500/api/v1/jobs/1

# browse + search
curl 'localhost:6500/api/v1/browse?kind=movie&sort=title&page=1&perPage=48'
curl 'localhost:6500/api/v1/search?q=king'

# edit metadata, lock fields, record progress
curl -X PATCH localhost:6500/api/v1/items/2 \
  -H 'Content-Type: application/json' \
  -d '{"overview":"…","genres":["Drama"],"locks":["title","overview"]}'

curl -X POST localhost:6500/api/v1/items/2/progress \
  -H 'Content-Type: application/json' \
  -d '{"positionMs":12000,"durationMs":120000,"played":false}'
```

## Item shapes

`Item`: `id, libraryId, parentId, kind (movie|show|season|episode|home_video|extra),
title, year, overview, runtimeMs, rating, contentRating, seasonNumber, episodeNumber,
addedAt, updatedAt, locked, played, positionMs, posterUrl?, backdropUrl?, fileCount`.

`ItemDetail` adds `genres[], people[{name,role,character}], files[], locks[], children[],
libraryName`.

`MediaFile`: `id, path, relPath, size, container, durationMs, bitrate, width, height,
videoCodec, hdr, audioStreams[{codec,channels,layout,sampleRate,language,title,default}],
subtitleStreams[{codec,language,title,forced}], missing, streamUrl`.

## Notes

- `kind=show` for TV libraries; episodes are reached through show → season → episode.
- Searches are prefix matches (`"king"*`) AND-ed across tokens.
- `metadataStatus` is one of `unmatched`, `matched`, `manual`, `local` or `failed`.
  `local` means metadata came from a `.nfo`/`.vsmeta` sidecar (`provider` shows which).
- Progress is per user; M1 has a single built-in user (`id=1`).
- Local accounts use a first-run admin setup. There is no OIDC/LDAP/2FA yet; use a
  reverse proxy/Tailscale for remote access, not a direct internet-exposed port.
