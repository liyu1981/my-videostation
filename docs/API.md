# HTTP API (v1)

Base path: `/api/v1`. JSON, camelCase. Errors are `{"error":{"code","message"}}` with
matching HTTP status. No pagination cursor yet — pages are numeric.

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Liveness + DB ping + version |
| GET | `/config` | Version, build info, current (single) user |
| GET | `/libraries` | Libraries with paths + item counts |
| POST | `/libraries` | Create `{name,type,language?,paths[]}`; validates absolute, existing dirs |
| GET | `/libraries/{id}` | One library |
| PATCH | `/libraries/{id}` | Rename / change language / replace paths / `autoScanMinutes` |
| DELETE | `/libraries/{id}` | Remove library and derived data (never touches media files) |
| POST | `/libraries/{id}/scan` | Enqueue a scan job → `202 {job}`; optional body `{force:true}` re-probes every file |
| POST | `/libraries/{id}/enrich` | Enqueue metadata matching for unmatched items → `202 {job}` |
| GET | `/jobs?limit=` | Recent jobs |
| GET | `/jobs/{id}` | Job status/progress/message |
| GET | `/jobs/{id}/logs?limit=` | Persisted log lines (`info`/`warn`/`error`) |
| DELETE | `/jobs/{id}` | Delete one task and its logs |
| DELETE | `/jobs` | Delete all finished tasks and their logs → `{deleted}` |
| GET | `/home` | `{continue,recent,shelves[]}` for the landing page |
| GET | `/browse` | `?kind=&libraryId=&sort=&page=&perPage=&q=&metadataStatus=&category=&genre=&year=&watched=&folder=` → `{items,page,perPage,total,totalPages}` |
| GET | `/libraries/{id}/facets?kind=` | Genre/year counts for filters |
| GET | `/libraries/{id}/folders` | Folder prefixes with file counts |
| GET | `/collections` | Collections (Favorites, Watchlist, user) with item counts |
| POST | `/collections` | `{name}` |
| PATCH | `/collections/{id}` | `{name}` (system collections rejected) |
| DELETE | `/collections/{id}` | Delete a user collection |
| GET | `/collections/{id}/items?page=` | Items in a collection |
| POST | `/collections/{id}/items` | `{itemIds[]}` |
| DELETE | `/collections/{id}/items/{itemId}` | Remove one item |
| POST | `/items/batch` | `{action: watched\|unwatched\|add_collection\|remove_collection, itemIds[], collectionId?}` |
| GET | `/search?q=&limit=` | FTS5 across title, original title, overview, people |
| GET | `/items/{id}` | Detail: item + genres + people + files + locks + children (seasons/episodes) |
| GET | `/items/{id}/children?kind=` | Seasons for a show, episodes for a season |
| GET | `/items/{id}/neighbors` | Previous/next item in browse order (`prev`, `next`) |
| PATCH | `/items/{id}` | Metadata patch (`title, year, overview, tagline, genres[], locks[], locked, …`) |
| POST | `/items/{id}/progress` | `{positionMs, durationMs, played}` (upsert) |
| GET | `/settings` | Provider settings (API key is never returned) |
| PUT | `/settings` | `{tmdbApiKey?, language?, region?}` (empty key clears it) |
| GET | `/items/{id}/matches?q=&year=&language=` | Provider candidates, best first; `language` overrides the provider search language (default `en-US`). For episodes and seasons the query searches **TV shows** (default: the parent show's title). `year` is only applied when explicitly passed; the item's stored year is not used implicitly |
| POST | `/items/{id}/match` | `{providerId, language?, season?, episode?}` — manual match; `language` fetches the provider details in that language (overrides locks). For episodes `providerId` is the **show id**; optional `season`/`episode` select which episode to fetch (default: the item's numbers). For seasons `providerId` is the show id and `season` selects the season |
| POST | `/items/{id}/refresh?language=` | Re-fetch from the stored provider id (respects locks); optional language override. Episodes use their stored show id + season/episode |
| GET | `/items/{id}/duplicate` | Another same-title item in the library (`{item}`), or `{}` (movies, home videos and shows) |
| POST | `/items/{id}/merge` | `{fromId}` — move the other item's files/credits/locks/progress into this item, then delete it; for shows, seasons/episodes are merged by season/episode number |
| POST | `/items/{id}/keep-separate` | `{otherId}` — mark both items so scans never auto-merge them |
| POST | `/items/{id}/apply-local` | `{sourceId}` — copy another same-kind item's metadata (title/overview/tagline/date/rating/artwork/genres/cast) onto this item; used by "Select from existing videos" |
| POST | `/items/{id}/art` | `{kind:"poster"|"backdrop", url}` — download + cache artwork; an empty `url` clears it |
| GET | `/items/{id}/poster` | Cached/sidecar artwork, guarded to library roots |
| GET | `/items/{id}/backdrop` | Sidecar backdrop |
| GET | `/items/{id}/playback` | Player payload: stream URL, resume position, subtitle tracks, audio tracks, next episode |
| GET | `/media/{fileId}` | Direct play, byte ranges (`Range: bytes=`), `206` |
| GET | `/media/{fileId}/subtitle?ext=ID` | Sidecar subtitle as WebVTT |
| GET | `/media/{fileId}/subtitle?embedded=INDEX` | ffmpeg-extracted embedded subtitle as WebVTT (cached) |

## Examples

```bash
# create a library and scan it
curl -X POST localhost:6500/api/v1/libraries \
  -H 'Content-Type: application/json' \
  -d '{"name":"Movies","type":"movie","paths":["/mnt/media/movies"]}'

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
- No authentication in M1; the server binds to `127.0.0.1` by default.
