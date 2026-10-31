# `.vsmeta` format (reverse-engineered)

Synology Video Station writes a `<video file>.vsmeta` sidecar next to each video. It is a
**protocol-buffers message with no published schema** (no magic header, no text). We parse it
with a schema-less wire reader and a recovered field map, so unknown fields are skipped
safely and future changes degrade instead of breaking.

Implementation: [`internal/vsmeta/vsmeta.go`](../internal/vsmeta/vsmeta.go).
Verified against a real 621 KB sample (007 *No Time to Die*, 2021).

## Recovered fields

| Field | Wire | Meaning | Example / notes |
|---|---|---|---|
| 1 | varint | media kind | `1` = movie (TV values unverified) |
| 2 | string | title (localised) | `007：无暇赴死` |
| 3 | string | original title | `007：无暇赴死` |
| 4 | string | tagline | `终极任务，亲眼见证。` |
| 5 | varint | year | `2021` |
| 6 | string | release date | `2021-09-29` |
| 7 | varint | unknown flag | `1` in the movie sample |
| 8 | string | overview / plot | UTF-8, CJK common |
| 9 | string | provider JSON | see below |
| 10 | message | credits | `1`=cast[], `2`=directors[], `3`=genres[], `4`=writers[] (all repeated strings) |
| 11 | string | content rating | `PG-13` |
| 12 | varint | rating × 10 | `75` → 7.5 |
| 17 | string | poster image | base64 JPEG (~60 KB) |
| 18 | string | poster MD5 | lowercase hex, matches decoded bytes |
| 21 | message | backdrop | `1`=base64 JPEG (~400 KB), `2`=MD5, `3`=unix timestamp |

Fields 13–16 and 19–20 were absent in the movie sample; they are likely TV
episode/season-specific and are currently ignored.

### Field 9 — provider JSON

```json
{"com.synology.TheMovieDb":{
  "backdrop":["https://image.tmdb.org/t/p/original/r2GAjd4rNOHJh6i6Y0FntmYuPQW.jpg"],
  "poster":["https://image.tmdb.org/t/p/w500/mXMePP0xelSJ1s69YrN7EYrwiTS.jpg"],
  "collection_id":{"themoviedb":645},
  "rating":{"themoviedb":7.5},
  "reference":{"imdb":"tt2382320","themoviedb":370172}
}}
```

Note `reference.themoviedb` is a **number** while `imdb` is a string — the parser accepts
both (`map[string]any`). Other plugin keys (e.g. `com.synology.TheTVDB`) are merged the same
way if present.

## Verification

```bash
MVS_VSMETA_SAMPLE="/path/to/file.mkv.vsmeta" \
  go test ./internal/vsmeta -run RealSample -v
```

Expected log line for the reference sample:

```
title="007：无暇赴死" original="007：无暇赴死" year=2021 rating=7.5 cert="PG-13"
cast=54 genres=[冒险 动作 惊悚] poster=60994B backdrop=397513B
refs=map[imdb:tt2382320 themoviedb:370172]
```

The poster MD5 is checked implicitly: the decoded bytes must start with `FF D8 FF` and match
the hash stored in field 18 of the sample.

## How to extend the mapping

1. Get a sample of the new shape (TV episode, home video, etc.).
2. Dump it with the throwaway decoder used during discovery (`protoc --decode_raw` also works):

   ```bash
   protoc --decode_raw < sample.vsmeta | head -40
   ```
3. Add the field to the `switch` in `Parse`, keeping unknown fields on the `skip` path.
4. Add a fixture in `vsmeta_test.go` (the file contains a tiny protobuf encoder) and, if
   possible, assert against the real sample via `MVS_VSMETA_SAMPLE`.

## Integration with the scanner

`internal/sidecar` merges sidecars for one video file:

1. `.nfo` (Jellyfin/Kodi style) — **highest priority**, field by field.
2. `.vsmeta` — fills the gaps; embedded poster/backdrop are written to
   `<data>/cache/images/vsmeta-<md5>.<ext>` (content-addressed, reused across scans).

Local metadata is stored on the item with `provider = "nfo" | "vsmeta" | "nfo+vsmeta"` and
`metadata_status = "local"`. Consequences:

- automatic TMDB matching skips these items (`local` is not `unmatched`);
- a manual **Identify** still overrides them;
- per-field locks still win;
- re-scans are idempotent — item identity follows the file path, so sidecar titles and user
  renames do not create duplicates.

Artwork from NFO is only used when it points at a **local** file; remote `<thumb>` URLs are
ignored (the `.vsmeta` embedded art covers the common case, and provider downloads arrive
through TMDB matching).
