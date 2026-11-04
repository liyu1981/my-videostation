# Disc images (ISO / BDMV / VIDEO_TS) — workaround

**Status:** non-goal for v1 (`GAP-003` in `docs/GAPS.md`). This page documents the
supported path so an ISO library is not a dead end.

## Why it is not supported

Optical images are not "just another container". A disc has a title structure
(menus, multiple playlists, angle/edition variants, chapter maps, per-title audio
and subtitle streams), and ffmpeg's `dvd://`, `bluray://` and ISO demuxers expose
that as many input programs rather than one video. Serving them correctly means
modelling playlists and navigation in our scanner, player, and transcode session
key (which currently assumes one continuous stream per file). That is a product
in its own right, not a container parser.

Note also that most Blu-ray images are AACS-encrypted; we will not ship or
document circumvention of that protection.

## Recommended workaround: rip once, scan the MKV

1. Rip the disc (or mount the ISO) to a single MKV per title with your tool of
   choice (MakeMKV is the common one; it keeps the lossless streams and lets you
   pick the right playlist, audio and subtitle tracks).
2. Name the result normally, e.g. `Movies/Heat (1995)/Heat (1995).mkv`, and drop
   the file into a scanned library folder.
3. Rescan. Everything else works out of the box: HEVC/AC3/DTS titles play through
   the M5 HLS pipeline (see `docs/adr/0003-hls-transcoding.md`), including audio
   switching and PGS subtitle burn-in.

## ffmpeg-only fallback (unencrypted DVDs and ISOs)

For an unencrypted `VIDEO_TS` folder or a DVD ISO, ffmpeg can usually remux the
main title without re-encoding:

```bash
# pick the longest title (ffprobe lists durations per title)
ffprobe -v error -show_entries program=program_id:format=duration "dvd:///path/to/VIDEO_TS"

# remux the main title (adjust -map if the disc has multiple angles)
ffmpeg -i "dvd:///path/to/VIDEO_TS" -map 0:v:0 -map 0:a -map 0:s? \
       -c copy "Heat (1995).mkv"
```

ISO files can be mounted read-only (`mount -o loop,ro file.iso /mnt/disc`) and
treated as the same `dvd://` / `bdmv://` source. Expect these caveats:

- title/angle selection is manual; the container order is not the disc menu,
- some DVDs need re-encoding (interlaced or broken timestamps),
- chapters are not preserved as item metadata in v1.

## What would change our mind

A dedicated "disc image support" design that models playlists as first-class
titles (scan → title list → per-title playback sessions), plus a decision on
encrypted discs. Until then, ripping is the supported workflow.
