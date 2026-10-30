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
| PATCH | `/libraries/{id}` | Rename / change language / replace paths |
| DELETE | `/libraries/{id}` | Remove library and derived data (never touches media files) |
| POST | `/libraries/{id}/scan` | Enqueue a scan job → `202 {job}` |
| GET | `/jobs?limit=` | Recent jobs |
| GET | `/jobs/{id}` | Job status/progress/message |
| GET | `/home` | `{continue,recent,shelves[]}` for the landing page |
| GET | `/browse` | `?kind=&libraryId=&sort=&page=&perPage=&q=` → `{items,page,perPage,total,totalPages}` |
| GET | `/search?q=&limit=` | FTS5 across title, original title, overview, people |
| GET | `/items/{id}` | Detail: item + genres + people + files + locks + children (seasons/episodes) |
| GET | `/items/{id}/children?kind=` | Seasons for a show, episodes for a season |
| PATCH | `/items/{id}` | Metadata patch (`title, year, overview, tagline, genres[], locks[], locked, …`) |
| POST | `/items/{id}/progress` | `{positionMs, durationMs, played}` (upsert) |
| GET | `/items/{id}/poster` | Sidecar artwork, guarded to library roots |
| GET | `/items/{id}/backdrop` | Sidecar backdrop |
| GET | `/media/{fileId}` | Direct play, byte ranges (`Range: bytes=`), `206` |

## Examples

```bash
# create a library and scan it
curl -X POST localhost:8080/api/v1/libraries \
  -H 'Content-Type: application/json' \
  -d '{"name":"Movies","type":"movie","paths":["/mnt/media/movies"]}'

curl -X POST localhost:8080/api/v1/libraries/1/scan
curl localhost:8080/api/v1/jobs/1

# browse + search
curl 'localhost:8080/api/v1/browse?kind=movie&sort=title&page=1&perPage=48'
curl 'localhost:8080/api/v1/search?q=king'

# edit metadata, lock fields, record progress
curl -X PATCH localhost:8080/api/v1/items/2 \
  -H 'Content-Type: application/json' \
  -d '{"overview":"…","genres":["Drama"],"locks":["title","overview"]}'

curl -X POST localhost:8080/api/v1/items/2/progress \
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
- Progress is per user; M1 has a single built-in user (`id=1`).
- No authentication in M1; the server binds to `127.0.0.1` by default.
