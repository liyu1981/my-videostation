# ADR 0006 — Background transcode cache (replaces on-demand HLS)

Status: accepted · Date: 2026-11-12 · Supersedes the delivery model in ADR 0003
Implementation status: **partially implemented** — migration steps 1–3 of §8
(schema, `transcode` job, variant builder, prepare API, web player prepare/poll)
are in place. The Jellyfin adapter still uses HLS until a variant is ready,
eviction/budget wiring and removal of the live session path remain.

## 1. Context

M5 delivered on-demand HLS sessions: when a file cannot be direct-played, ffmpeg
starts a live session, encodes/copies streams into fMP4 segments, and the browser
plays them through hls.js. That works, but the model has structural problems:

- **Seek + stream copy is not safe.** A copied stream can only start at a
  keyframe while a re-encoded stream seeks accurately; mixing the two on a
  non-zero start desynchronises audio and video (or starts at the wrong
  position). Full transcode is the only mode that seeks correctly today.
- **Segments follow the source GOP when video is copied.** A 10 s source GOP
  gives 10 s segments, so startup latency and seek granularity are dictated by
  the file rather than by us.
- **Every viewer pays again.** Two users watching the same title start two
  ffmpeg processes; the global session cap (default 2) turns a family into
  `429 too_many_sessions`.
- **Nothing survives a restart.** The cache is deleted at startup because every
  playlist is invalid, so a NAS reboot forces a full re-encode.
- **The browser needs a second runtime.** hls.js is a ~578 kB / 180 kB gzip
  dependency that only exists because we deliver live HLS.

The alternative we want is the one media servers have used for years: convert
once, in the background, and serve the resulting file like any other video.

## 2. Decision

**Transcoding becomes a durable background job whose output is a normal file.**

- When playback needs a transcode, enqueue a `transcode` job in the existing
  `jobs` queue. A worker converts the file as fast as the configured worker
  allows (no `-readrate` pacing; background work is expected to use CPU).
- The job writes **one progressive MP4 variant per requested profile**
  (`-movflags +faststart`) under
  `<data>/cache/transcodes/<fileID>/<variantKey>.mp4`. Media folders stay
  read-only.
- Per-stream copy decisions are unchanged from ADR 0003 §2: copy H.264 video
  and AAC/MP3 audio when possible, encode HEVC/VC-1/MPEG-2/AC3/DTS/TrueHD;
  subtitle burn-in produces a separate variant. Because the whole file is
  converted with one timeline, **A/V sync and seeking are correct by
  construction**.
- A completed variant is served as a static file with HTTP range support
  (the existing `/api/v1/media/{fileId}` path, extended with a variant id).
  There is no per-play ffmpeg process, no HLS playlist and no hls.js for file
  playback.
- The web player asks the server to **prepare** playback: direct-play files
  start immediately; otherwise it shows conversion progress (from the job) and
  starts the static file when the variant is ready.

## 3. Variant model

A variant is identified by everything that changes the bytes:

```
key = hash(
  fileID, mtime, size,
  maxHeight,              // 0 = source height
  audioIndex,             // selected track
  burnSubtitle,           // embedded index or external subtitle id; -1 = none
  videoCodecProfile,      // encoder profile / pixel format / HDR policy
  containerProfile        // mp4, H.264 + AAC output
)
```

Output: `MP4` with `-movflags +faststart`, video H.264 (8-bit, yuv420p),
audio AAC 160 kbps (stereo downmix above 2ch) or a copied AAC/MP3 track.
`maxHeight` below the source height forces a video encode (copy cannot scale),
exactly as today.

Only variants that were actually requested are generated. The quality ladder
still offers Auto/1080p/720p/… but selecting a quality enqueues that variant
instead of restarting a live session.

## 4. Jobs, workers and deduplication

- New job type `transcode`, payload:
  `{fileId, variantKey, maxHeight, audioIndex, burnSubtitle, ...}`.
- **Deduplicate on `variantKey`**: a ready variant is returned immediately; an
  active job is joined (same job id) instead of starting a second ffmpeg.
- Worker concurrency: `MVS_TRANSCODE_WORKERS` (default **1** on NAS-class
  hardware, configurable). Conversion jobs share the durable queue with scans
  and enrichment but have their own concurrency limit; playback-preparing jobs
  may take priority over background pre-generation.
- Progress: parse `-progress pipe:1` into `jobs.progress` + message (frame/time/
  speed) and surface it through the existing job API/SSE.
- Cancellation: deleting the job stops ffmpeg and removes the partial file.
- Restart: incomplete variants are deleted or marked failed at startup; the
  next playback request enqueues a fresh job.
- Failure: one bad file fails only its job; the item remains playable through
  the legacy HLS fallback until the migration is complete.

## 5. Storage and eviction

```
<data>/cache/transcodes/<fileID>/<variantKey>.mp4
```

- A `transcode_variants` table records
  `(file_id, mtime, size, variant_key, path, bytes, duration_ms, state,
  created_at, last_used_at, last_error)`. It is the lookup for "is this ready?"
  and the source for eviction.
- Source change (`mtime`/`size`) invalidates and deletes its variants.
- Cache budget: `MVS_TRANSCODE_CACHE_GB` (default 20). Evict least-recently-used
  finished variants above the budget; never evict an active job.
- Manual "clear transcode cache" belongs with the existing cache controls
  (GAP-055).

## 6. Request and delivery flow

### Web player

1. `GET /api/v1/playback/{itemId}?...` returns
   `{state: "direct" | "ready" | "queued" | "running", url?, jobId?, progress?}`.
2. `direct` → play the file as today.
3. `queued`/`running` → show "Preparing playback… x%" (job progress) and poll
   the playback endpoint; when `ready`, play the variant URL.
4. The player no longer loads hls.js for file playback. HLS remains only as a
   transitional fallback behind a config flag while the migration is in
   progress.

### Jellyfin adapter (native clients)

Native clients expect `PlaybackInfo` to return a playable source immediately.
Transitional plan:

- If a variant is ready → return it as `DirectStreamUrl` (static MP4, range).
- If not → enqueue the job and **keep returning the existing HLS
  `TranscodingUrl`** until the variant exists. HLS is therefore retained for
  the adapter during migration; once the variant is ready, subsequent
  `PlaybackInfo` calls return the static file.
- After the migration stabilises, add a prepare/long-poll endpoint and switch
  the adapter to static URLs only.

This keeps Findroid/Wholphin/Moonfin working while the delivery model changes.

## 7. What is removed, what stays

**Removed after migration:** `playback.Manager` session lifecycle (spawn, idle
reaping, session cap, `429 too_many_sessions`), `GET /hls/*`, per-session
transcode directories, hls.js.

**Kept:** `BuildPlan`'s per-stream copy/encode decisions (renamed
`BuildVariantPlan`), encoder profiles, subtitle burn-in, audio selection,
`EvaluateDirectPlay`, direct-play range streaming, job queue/SSE, the Jellyfin
adapter's source model.

## 8. Migration plan

1. **Schema + job**: `transcode_variants` table, `transcode` job handler,
   variant builder using the existing plan logic; output written atomically
   (`.part` → rename).
2. **Playback API**: `GET /playback/{itemId}` grows `state/url/jobId/progress`;
   static variant serving with range requests.
3. **Web player**: prepare/poll flow; keep HLS fallback behind
   `MVS_TRANSCODE_HLS_FALLBACK=true` for one release.
4. **Jellyfin adapter**: prefer ready variants; keep HLS while jobs run.
5. **Remove** session manager, `/hls/*`, hls.js, and the fallback flag after a
   release with no regressions.

## 9. Consequences and risks

- **First play is slower.** A full conversion must finish before playback
  (`+faststart` puts the moov atom at the end of the conversion, so a partial
  file is not seekable). Mitigations: progress UI, queue priority for the
  requested variant, worker count, optional pre-generation of popular items.
  If early playback becomes essential, a fragmented MP4 served from a growing
  file is the fallback design, but it complicates range/seek handling and is not
  v1.
- **Disk usage grows.** Budget + LRU eviction; variants are disposable.
- **CPU is bursty.** Conversion runs at full speed when queued; `nice`/`ionice`
  or a pause-during-playback policy can be added if NAS users complain.
- **Quality/audio variants multiply storage.** Generate only what is requested.
- **Long files block a worker.** Default one worker means a 2 h movie delays
  other conversions; the queue makes this visible, and `MVS_TRANSCODE_WORKERS`
  is the escape hatch.
- **No live TV.** This model applies to file playback; live/recording streaming
  would need a separate decision.

## 10. Alternatives considered

- **Keep on-demand HLS** (status quo): correct only for full transcode on seek;
  per-viewer CPU; session cap; no restart reuse. Rejected.
- **Hybrid: HLS while converting, then static file**: keeps first play fast but
  retains both pipelines and the seek/sync problem; used only as a transitional
  adapter fallback, not as the target.
- **Two-phase trim then partial transcode**: still keyframe-bound and adds I/O;
  the single-timeline file avoids the whole class of sync bugs.
- **Pre-transcode every file on scan**: predictable playback but enormous
  storage/CPU cost for a library nobody may watch. Rejected; convert on demand
  and cache.

## 11. Testing

- Unit: variant plan per-stream copy/encode (existing `plan_test.go` cases,
  renamed), variant key stability, dedupe.
- Integration: generate clips with unsupported video/audio; run the real job;
  `ffprobe` the output (codecs, duration, first video/audio PTS within 100 ms);
  range requests; restart reuse; eviction; cancellation leaves no partial file.
- Adapter: ready variant returned as `DirectStreamUrl`; HLS fallback while
  queued.
- Browser: direct vs prepared playback, progress UI, seek after conversion
  `[verify]`.
