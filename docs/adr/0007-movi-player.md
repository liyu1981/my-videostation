# ADR 0007 — Adopt Movi Player for web playback

Status: accepted · Date: 2026-11-18

## 1. Context

The web player was a custom React shell over a native `<video>` element, with
`hls.js` for the legacy HLS path. Files whose container/codecs the browser could
not demux (Matroska, HEVC, VP9-in-MKV, DTS, …) had to be transcoded server-side.

Movi Player (Apache-2.0) is a browser player built on **FFmpeg WASM + WebCodecs +
Canvas**. It demuxes MKV/HEVC/AV1/VP9 and renders them without server
transcoding, exposes an HTMLMediaElement-like API, ships a React wrapper, and
includes controls, subtitles, multi-audio, chapters and HLS/DASH fallbacks.

A browser test against this project's range-streaming endpoint played both an
H.264/AAC MP4 and a **VP9/AAC Matroska** file (`/api/v1/media/{id}`), with
`currentTime` advancing and no console errors.

## 2. Decision

Use **Movi Player** as the web playback engine:

- `web/src/components/Player.tsx` renders `MoviPlayer` from
  `movi-player/react` and keeps the app shell: close/back handling, queue and
  previous/next, next-up countdown, resume/progress writes, and the poster/title.
- Playback uses the **original media URL** (`/api/v1/media/{fileId}`), so MKV,
  HEVC, VP9, AV1 and HDR files play without a server transcode.
- On a Movi error, the player falls back to the server's cached MP4 variant via
  `POST /items/{id}/prepare` (same prepare/poll path as ADR 0006).
- The player chunk is **lazy-loaded** with `React.lazy`; the main SPA bundle does
  not include Movi/WASM.
- The server-side HLS session manager remains for the **Jellyfin adapter** and
  for the fallback variant path; the web UI no longer uses hls.js.
- Queue/next-up, progress saving and resume stay in our React shell rather than
  Movi's playlist, so the existing API and watch-state semantics are unchanged.

## 3. Consequences

- **Bundle:** the player chunk is ~11 MB minified (embedded WASM) and is only
  fetched when the player opens; the main bundle stays ~0.5 MB. The slim build
  (external `movi.wasm`) is a future optimisation.
- **Browser support:** Chrome/Edge 110+, Safari 18+, Firefox 130+ for WebCodecs;
  Movi has a native fallback where possible, but MKV/HEVC playback needs those
  engines.
- **Feature changes:** Movi owns the control bar, subtitle rendering/settings and
  audio-track menu. The old custom subtitle delay/cue styling, server quality
  ladder and Chromecast/DLNA controls are no longer part of the web player.
  Chromecast/DLNA remain available through the server-side cast APIs, but the
  in-player cast menu was removed with the custom shell.
- **Fallback:** files Movi cannot decode still get the cached MP4 variant
  (H.264/AAC) via `prepare`, so the server pipeline remains a safety net.
- **Dependency:** `movi-player` 0.4.1 (Apache-2.0) plus its Shaka/dash
  dependencies; `hls.js` is no longer a web dependency.

## 4. Alternatives

- **Video.js**: mature, but its HLS/VHS path does not demux Matroska; it would
  not play the VP9/MKV case without the same server work.
- **Mediabunny**: a capable demux/mux/decode toolkit, but not a player; turning
  it into one means building controls, buffering, seeking and rendering.
- **Server WebM remux**: cheap and universal-ish, but adds a variant profile and
  still needs server conversion; Movi removes the server round-trip for web.
