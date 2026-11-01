# ADR 0002 — Add TheTVDB as a second metadata provider

Status: **deferred** — provider mechanism in place; the TVDB client was prototyped and removed because getting an API key is not user-friendly. Revisit if a plugin/key flow is added.

## Context

TMDb is our only metadata provider. Its TV data is sometimes incomplete or wrong
(wrong episode titles/ordering, missing translations, poor artwork for regional
dramas), and TMDb occasionally has no record for a valid series id. Video Station
lets the user install several "Video Info Plugins" and choose the source per item,
so we need a second provider and a way to keep more than one provider id per item.

## Goals

1. Search, match and refresh from **TheTVDB** for series, seasons and episodes
   (movies are out of scope for the first cut; TVDB v4 also covers movies, add later).
2. Keep TMDb as the default; allow TVDB as a **fallback** and as a **manual choice**.
3. Store both provider ids on an item (TMDb + TVDB) without breaking existing rows.
4. Per-search language, provider selector in the search dialog, and the existing
   best-effort fill when a provider details request fails.
5. No change to the media-read-only / DB-authoritative rules.

## TVDB API v4 notes

- Auth: `POST /v4/login {apikey, pin?}` → JWT (~30 days). Cache the token in memory,
  re-login on 401. A user-supplied API key (and optional PIN) lives in settings.
- Search: `GET /v4/search?query=&type=series&limit=` → `tvdb_id`, `name`, `year`,
  `overview`, `image_url`, `primary_language`.
- Series: `GET /v4/series/{id}` → name, overview, year, firstAired, image,
  content ratings, genres, artwork.
- Episodes: `GET /v4/series/{id}/episodes/default?season=&episode=&page=` →
  name, overview, aired, image, runtime, seasonNumber, number, absoluteNumber.
  `default` = aired order; DVD/absolute ordering is a later option.
- Seasons: season metadata/artwork via the season's episodes + `/seasons/{id}/extended`.
- Artwork: `/series/{id}/artworks?type=poster|background`; pick language/score.
- Languages: TVDB uses 3-letter codes (`eng`, `zho`, `jpn`, …). Map from our
  `en-US`, `zh-CN`, `zh-TW`, `ja-JP`, …; fall back to English.
- Rate limits: token cache, 3 retries with backoff (reuse the TMDb retry helper),
  respect `Retry-After`.

## Data model

Migration `0012_item_provider_ids.sql`:

```sql
CREATE TABLE item_provider_ids (
    item_id    INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    provider   TEXT    NOT NULL,             -- tmdb | tvdb
    provider_id TEXT   NOT NULL,
    created_at TEXT    NOT NULL,
    PRIMARY KEY (item_id, provider)
);
```

- Backfill from `media_items.provider` / `provider_id` where non-empty.
- Keep `media_items.provider` / `provider_id` as the **primary/applied** provider
  (compatibility, detail page "Reference", refresh default).
- Matching stores/updates the mapping row for the chosen provider and sets the
  primary provider to it.
- Artwork paths stay on the item; the provider id table is only identity.

## Settings

- `metadata.tvdb_api_key` (v4 key), optional `metadata.tvdb_pin`.
- `metadata.provider_priority` (default `tmdb,tvdb`) for automatic enrichment.
- Settings UI: masked TVDB key (+PIN), provider order, "Test connection".
- The per-search language stays in the search dialog (remembered in localStorage);
  no default-language field in Settings.

## API

- `GET /providers` → `[{ name, configured, kinds: ["movie","show","season","episode"] }]`.
- `GET /items/{id}/matches?provider=&q=&year=&language=` — provider optional; when
  omitted, query the configured priority and tag each candidate with `provider`.
- `POST /items/{id}/match { provider, providerId, language, season?, episode? }`.
- `POST /items/{id}/refresh?provider=` — refresh using the stored id for that provider.
- Auto-enrich: try providers in priority order, stop at the first confident match;
  record the provider id mapping. A later manual match can switch providers.

## UI

- Search dialog: **Provider** selector (TMDb / TheTVDB) next to Language; the
  results table already has a Source column — show the selected provider there.
- Language list maps to the provider's codes (TVDB 3-letter).
- The existing best-effort fill applies when a details call fails (404): populate
  the editor from the search candidate and let the user save or discard.
- Detail page "Reference" shows the applied provider; the overflow refresh uses it.

## Matching

- TVDB series search by name/year; reuse the normalized-name + year + popularity
  scoring; manual choice always overrides.
- Episodes/seasons: search the **series** (default query = show title), then fetch
  by season/episode number from the selected provider.
- Store confidence/status as today (`matched`/`manual`), provider in `provider`.

## Rollout (status)

1. ⛔ TVDB client — prototyped (login/token, search, series, episodes, seasons, artwork)
   and removed; kept only as this design note.
2. ✅ `item_provider_ids` migration + backfill; provider-aware match/refresh APIs.
3. ✅ Provider registry, `/providers`, search-dialog provider selector + Source column
   (hidden while only one provider is registered; remembers the choice for the future).
4. ⏳ Auto-enrich fallback/priority when a second provider is added.
5. ⏳ Season/episode polish, movie support and richer artwork selection.

## Risks / open questions

- TVDB API terms/licensing and attribution; the key is user-supplied, no scraping.
- Token expiry/refresh and rate limits.
- Language code mapping and missing translations (fallback to English).
- Which provider id is "primary" when both exist; manual choice wins.
- TVDB ordering (aired vs DVD) for anime/regional dramas.
- TVDB movie support and per-library provider preference (later).

## Verification

- Client unit tests with recorded fixtures (login, search, series, episodes, artwork,
  401 re-login, 429/5xx retry, language mapping).
- Handler tests: provider param, mapping table, priority/fallback, best-effort.
- UI: provider selector, TVDB results, manual match, episode/season search, refresh.
