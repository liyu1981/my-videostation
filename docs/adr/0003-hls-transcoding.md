# ADR 0003 — M5 transcoding: on-demand HLS sessions

Status: **superseded by [ADR 0006](0006-background-transcode-cache.md)** (playback
delivery), 2026-11-12 · Date: 2026-11-04

> ADR 0006 replaces the delivery model: transcoding becomes a background job that
> writes a cached MP4 variant, served as a static range-capable file. The
> per-stream copy/encode rules (§2) and encoder profiles (§8) carry over; the
> live session lifecycle and HLS delivery (§1, §3–§5, §7, §9) are legacy and will
> be removed after the migration. This document is kept for history.

Records how `internal/playback` and the browser player implement M5, and which
options were deliberately rejected.

## 1. HLS fMP4, single rendition per session

Output is HLS with fMP4 segments (`-hls_segment_type fmp4`, 4 s, `event`
playlist) plus a `init.mp4` map. HLS is what `PLAN.md` promised, it seeks better
than a piped MP4, and fMP4 plays in Safari natively and in Chrome/Firefox via
MSE.

A full ABR ladder would mean encoding N renditions concurrently. Instead each
session has one rendition: `maxHeight` caps the source height (the API returns
the ladder so the UI can offer "Auto / 720p / 480p …"), and changing it restarts
the session. That keeps CPU predictable on NAS-class hardware, which is the
deployment target.

## 2. Remux first, transcode only what browsers cannot play

`BuildPlan` decides per stream:

| Stream | Copy when | Otherwise |
|---|---|---|
| Video | H.264 (`h264`) and no burn-in | `libx264` (or `h264_nvenc`/`h264_qsv`) |
| Audio | AAC/MP3/MP2 and selected track exists | AAC 160 kbps, downmix to stereo above 2ch |

HEVC is intentionally re-encoded even where hardware could copy it: hls.js/MSE
HEVC support is browser-dependent, and the M5 promise is "it plays", not "it
plays when the client happens to support it". MP4 with H.264/AAC never enters
this path at all (direct play via `/api/v1/media/{fileId}`).

Container names are canonicalised from the file extension
(`playback.ContainerFromPath`) because ffprobe reports both Matroska and WebM as
`matroska,webm`.

A requested `maxHeight` below the source height forces a video re-encode even for
H.264: a stream copy cannot be scaled. When the video is copied, the reported
session height is the source height (the request is not silently "honoured").

## 3. A session is a job, keyed by its inputs

`Manager` keeps at most `MVS_TRANSCODE_SESSIONS` (default 2) live jobs.
The cache key is `(fileId, mtime, startMs bucketed to 2 s, audioIndex,
burnSubtitle, externalSubtitle, maxHeight, force)`. Consequences:

- Re-opening the same position reuses the job (no second encode).
- A seek outside the encoded window restarts ffmpeg with `-ss <target>`; the
  player debounces scrub drags so dragging does not spawn a job per pixel.
- Audio switching (GAP-024), quality changes and burn-in selection are just a
  new key, so they are implemented without a second pipeline.

Jobs idle for `MVS_TRANSCODE_IDLE_SECONDS` (default 180) are killed and their
directory removed; the cache directory is also purged at startup because every
playlist is invalid after a restart. When the cap is reached the least-recently
seen session is evicted (finished jobs first); if every slot is actively playing
the request fails with `429 too_many_sessions` rather than thrashing the CPU.

## 4. Pacing: `-readrate 1.0` with a 12 s burst, `-re` fallback

Without pacing ffmpeg encodes the whole file immediately — filling `/data` and
burning CPU for a movie the user may close after five minutes. `-readrate`
(ffmpeg ≥ 5) is detected once at startup and falls back to `-re` on older
builds. Tests disable pacing so they finish in milliseconds.

## 5. Seek semantics: input seek, session-relative timeline

Transcodes pin keyframes to segment boundaries
(`-force_key_frames expr:gte(t,n_forced*4)`) so a long source GOP does not
produce 10 s+ segments and stall startup/seeks.

`-ss` is an input option (before `-i`): fast, and for `-c copy` it snaps to the
previous keyframe (≤ one GOP of position drift, acceptable for progress saves).
The produced playlist starts at zero, so the player keeps
`absolute = session.startMs + video.currentTime` for the scrub bar, resume saving
and subtitle timing. `durationMs` on the session is `file.duration - startMs`.

## 6. Subtitle burn-in

Text subtitles that are already extractable as WebVTT stay client-side (cheap,
switchable, delayable). Bitmap subtitles (PGS/VobSub) cannot be WebVTT, so they
are burned in:

- embedded bitmap stream → `-filter_complex "[0:v:0]<filters>[pre];[pre][0:N]overlay[vout]"`.
- external sidecar → `-vf subtitles=<escaped path>`.

`-vf` and `-filter_complex` cannot coexist for one stream, so the scale/SAR/
encoder filters are folded into the same graph. Choosing a burn-in track forces
a video re-encode. Delay and cue styling do not apply to burned-in tracks, and
the UI says so.

## 7. Frontend: hls.js loaded on demand

Browsers other than Safari need MSE-based HLS demuxing, so the player
dynamically imports `hls.js` **only when a session is needed** (separate 578 kB /
180 kB gzip chunk). Safari uses native HLS (`canPlayType('application/vnd.apple.mpegurl')`).
Direct-play titles never download the chunk.

This is the one new runtime dependency of M5 and it is justified: implementing
HLS demuxing or a second streaming format is far more code and risk than a
focused, widely deployed player library.

## 8. Hardware acceleration

`MVS_TRANSCODE_ENCODER=libx264|h264_nvenc|h264_qsv` selects an encoder profile
and `MVS_TRANSCODE_HWACCEL` adds an `-hwaccel` decode hint. There is no
autodetect, no startup smoke test and no VA-API profile (VA-API needs a device
node plus `hwupload` plumbing). That is deliberate M7 scope (GAP-054): a wrong
guess produces cryptic ffmpeg failures that are worse than predictable software
encoding.

## 9. Cache and data-safety

Everything lives under `<data>/cache/transcodes/<session-id>/`; media folders
are only ever read. A session's directory is deleted when it stops, when it goes
idle, when it is evicted, or at startup. `GET /hls/...` only serves
`index.m3u8`, `init.mp4` and `segNNNNN.m4s` from that directory (no path
traversal), and requires the session cookie.
