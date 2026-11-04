# Online trailers — workaround

**Status:** fetching/streaming trailers from a provider (YouTube, TMDb videos, …)
is a non-goal for v1 (`GAP-011` in `docs/GAPS.md`). Local trailer files are
fully supported and continue to be indexed as extras.

## Why it is not in the app

- **Privacy / third-party embed.** Playing a YouTube trailer means loading a
  Google iframe or proxy through the server; we do not want a media library to
  phone home or embed an ad/tracking surface by default.
- **Terms of service.** Bulk extraction of provider video URLs and re-hosting
  them is not something a self-hosted app should take on.
- **Availability is unreliable.** Provider trailer keys expire, are region
  locked, or are missing entirely; a "Play trailer" button that often fails is
  worse than an honest offline workflow.
- **Offline-first.** The library is meant to work on a LAN without internet
  access.

## What already works: local trailers as extras

Any file the naming parser recognises as a trailer, teaser, featurette or other
extra is attached to the folder's item and listed in the item's **Extras**:

```
Movies/Heat (1995)/Heat (1995).mkv
Movies/Heat (1995)/Heat (1995)-trailer.mkv     → extra "Trailer"
Movies/Heat (1995)/Extras/Featurette.mkv        → extra "Featurette"
TV/Show (2020)/Season 1/Show.S01E01-trailer.mkv → extra of that episode
```

Extras are hidden from browse, play in the normal player (direct play or the M5
HLS pipeline) and respect per-library scan rules — including the **Skip trailers
and featurettes** toggle in the library form if you would rather not index
them.

## Adding trailers you found elsewhere

Download the trailer yourself (browser, `yt-dlp`, or a disc extra) and drop it
next to the media with one of the recognised names above, then rescan. This
keeps the file local, playable offline, and independent of any provider.

If you only want the trailer for a handful of titles, that is the whole
workflow; no configuration is needed.

## What would change our mind

A provider integration that is opt-in per server, privacy-preserving (no
third-party iframes; direct URL only when the provider offers one), respects
terms of service, and degrades to "no trailer" without breaking the page. Until
then, trailers are local files.
