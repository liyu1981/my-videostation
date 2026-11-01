# Video Station UI Probe — Verified Findings & Re-creation Notes

Live probe of the user's Synology Video Station, done **read-only** (no metadata
edits, no saves, no scans, no playback). This supersedes the earlier CSS-only pass:
everything below was measured from a rendered, authenticated session unless marked
otherwise.

| Item | Value |
|---|---|
| Device | DS218+ (`apollolake`, Celeron J3355, 10 GB), DSM **7.1.1-42962 Update 9** |
| Video Station | **3.0.7-2512** (ExtJS 3.4 + Vue 2 islands, `syno-vs2-*` / `syno-vc-*`) |
| Viewport probed | 1920×1080 (app maximized) and 1280×800 |
| Raw artifacts | `/home/yli/vs-probe/screens/` (42 PNGs + DOM dumps) — **outside git**, local reference only |
| Writes performed | None to media/metadata. Only login sessions, in-session view toggles and read-only metadata searches. |

---

## 1. Shell geometry (measured)

```
1920×1080 maximized
┌──────────────────────────────────────────────────────────────────────────┐
│ top toolbar 48px   logo 170×32 · home 28 · search field 214×48 · user/help/queue/settings (28px icons) │
├───────────────┬──────────────────────────────────────────────────────────┤
│ sidebar       │ navbar 48px: category dropdown 136×26 at x=256 · right:   │
│ 200px         │ selection(28) view-mode(28) sort(28) at x=1788/1834/1880    │
│               ├──────────────────────────────────────────────────────────┤
│ "Library" 32  │ content canvas starts y=96, left gutter 56 (home) /      │
│ item 32px     │ 68 (detail); cards 120×180 + 20/14 gutter                │
│ icon 24 @x14  │                                                          │
│ text 14 @x38  │                                                          │
└───────────────┴──────────────────────────────────────────────────────────┘
```

| Element | 1920 (large mode) | 1280 (regular mode) |
|---|---|---|
| Hero container | 1608 × 548 | 968 × 348 |
| Hero poster / backdrop | 360×540 / 840×540 | 226×340 / 528×340 |
| Hero collapsed poster | 96 px | 60 px |
| Home row (poster) | 294 px = 36 toolbar + 242 body | same |
| Home row (landscape) | 232 px = 36 + 180 + borders | same |
| Poster card / cell | 120×180, cell 120×224 | same (fixed) |
| Landscape card / cell | 180×120, cell 180×164 | same |
| Tile row | 1608×175, thumb 100×150, padding 12 20 12 12 | same |
| List row | 1608×37, title 14, year right-aligned | same |
| Detail poster | 270×388, play button 76×76, actions 28 px row below | same |
| Season bar | 49 px, active tab = red box | same |
| Sidebar | 200 px (fixed) | 200 px |
| Top toolbar / navbar | 48 / 48 | 48 / 48 |

The only responsive change is the **hero mode switch** (large ↔ regular) plus column
count in the poster grid. Everything else is fixed-pixel. This is the single
biggest thing to modernize (fluid grid, scene-relative type), while keeping the
fixed left rail as the navigation anchor.

## 2. Colour (pixel-sampled, not stylesheet-inferred)

| Role | Hex | Evidence |
|---|---|---|
| Top toolbar | `#212427` (gradient `#24272b`→`#202325`) | pixel + computed |
| Content canvas | **`#393C3F`** | pixel at empty grid area |
| Sidebar / list surface | `#333639` | pixel |
| Selected sidebar row | `#404347` | pixel |
| Raised row / tile | `rgba(51,54,57,.8)`, hover `rgba(65,69,73,.8)` | computed |
| Default poster plate | `#272A2C` | pixel (poster placeholder) |
| Dialog surface | `#393C3F` | pixel |
| Text primary | `#F2F2F2` | computed (card titles, sidebar) |
| Text heading | `#E9E9E9` | detail title |
| Text secondary | `#CFCFCF` | taglines, info values |
| Text tertiary | `#ABABAB` | years, dates, inactive tabs |
| **Brand/active red** | `#D13B3B` | selection border+badge, season tab, dialog title, active page |
| **Progress cyan** | `#00D4FF` | watch bar |
| New/unwatched | `#D1A53B` dot · `#E6C929` chip bg with `#333639` text | computed |
| Rating stars | `#E6C929` | hero + detail |
| Primary button | `#2D89C9` (blue "OK") | settings/edit dialogs |
| Hairline | `rgba(255,255,255,.10)`, top `.20`, separators `.30` | computed |
| Cert chip | 1 px `#CFCFCF`, radius 2, 16 px Roboto Slab bold | detail top-right + hero |

Note: the *canvas is lighter than the chrome* (`#393C3F` vs `#212427` top bar), the
opposite of most modern dark apps. Worth keeping if we want the "Video Station feel",
but it costs contrast; a modern take is a darker canvas + lighter cards.

## 3. Typography (measured)

```
Roboto 400/700  — UI, titles          Roboto Slab bold — rating/cert chips
13px default UI text · 14px card/list titles · 12px secondary (years, dates, episode)
16px section labels / season tabs · 18px tagline · 20px home row titles
24px detail info-bar · 30px hero title · 36px detail title
```

Notable: **body text is 13 px and secondary is 12 px** — below modern minimums.
Card titles are single-line ellipsis with `ext:qtip` tooltips (ExtJS tooltip, not
native `title`), and long summaries get a **"View all"** button.

## 4. Screen inventory (all captured)

| # | Screen | File | What it establishes |
|---|---|---|---|
| 01 | Home | `01-home.png` | Rotating poster-stack hero + 3 rows (TV Show, Movie, landscape "Recently Added") |
| 02 | Home card hover | `02-home-poster-hover.png` | Bottom gradient + 20 px add/play/more, red 2 px frame |
| 03 | Hero hover | `03-home-hero-hover.png` | Active hero widens to backdrop with title/tagline/stars |
| 04 | Movies grid | `04-movies-grid.png` | 11 columns, 676 items, pagination, empty-poster cards for unparsed files |
| 05/06 | View menu | `06-view-menu.png` | **Thumbnail / Tile / List** only |
| 07 | List view | `07-movies-list.png` | 37 px rows, hover actions, right-aligned year |
| 08/42 | Tile view | `08-movies-tile.png`, `42-tile-1920.png` | 175 px rows, 100×150 thumb, 80 px summary block |
| 09 | Movie detail | `09-movie-detail.png` | Full-bleed backdrop, poster 270×388, 36 px title, 24 px info bar, chips 1080p/AAC 5.1, cast inline, reference links, subtitle/audio dropdowns |
| 10/11 | Detail scrolled | `10-…`, `11-…` | Cast, recommendations, summary, back/next arrows bottom corners |
| 13 | TV shows grid | `13-tvshows-grid.png` | Same card system |
| 14 | Show detail | `14-tvshow-detail.png` | Season bar 1–5 + episode stills (16:9-ish), "Episode N" |
| 15 | Home Video | `15-home-video.png` | Landscape cards, filename-as-title, date (MM/DD/YYYY) |
| 16 | Playlist | `16-playlist.png` | Three system collections: **Favorites / Watchlist / Shared Videos** |
| 17 | TV Recording | `17-tv-recording.png` | Empty state |
| 18 | Settings | `18-settings-*.png` | Tabs: **Library / Parental Control / Video Info Plugin / DTV / Privileges / Advanced / Options**; library tree with 2 paths (`/homeshare/video/movie`, `/video/movie`) |
| 19 | Selection mode | `19-selection-mode.png` | Red border + red check bar; toolbar swaps to cancel/playlist/share/watched/unwatched/edit/delete/convert |
| 20/28 | Search | `28-search-results.png` | Global search with "All" scope, results grid |
| 21/29 | Empty state | `29-search-empty.png` | "No data / No videos available" |
| 22/23 | Card hover + menu | `23-card-more-menu.png` | View media info, Edit video info, Search for subtitles, Public sharing, Mark as watched, Delete, Offline Convert |
| 24 | Category menu | `24-category-menu.png` | **Recently Added / Recently Watched / Recently Released / All / By Folder / By Channel / By Program / Timeline** |
| 25 | Filter | `25-filter-menu.png` | Facet list starting with Cast (very long), plus others |
| 26 | Settings tabs | `26-settings-video-info-plugin.png` | TMDB as "System default", per-type plugin list, connection status, **drag-to-reorder priority**, Add/Test/Edit/Delete |
| 32 | View media info | `32-media-info.png`, `38-…` | Overview / Video / Audio; exact field list (below) |
| 33 | Edit video info | `33-edit-video-info.png` | Video Info / Poster / Backdrop; plugin search vs. existing; field set + **Lock/Unlock** |
| 35 | Subtitle search | `35-subtitle-search.png` | Language / File name / Source; "No match found" (no provider configured) |
| 36 | Detail action menu | `36-detail-actionbar-menu.png` | Adds **Correct video info** and **Delete file** |
| 37/41 | 1280 layout | `41-home-1280.png` | Hero falls back to regular size |

Not captured (deliberately): player screens (playback would write watch progress),
public-share dialog (creating a link is a write), DVR/schedule (would need tuner state),
bulk delete/convert confirmations.

## 5. Component specs

### 5.1 Poster card (the atom of the app)

```
cell 120×224
├─ thumb 120×180  (2:3, cover, 1px rgba(255,255,255,.1) frame)
│   ├─ gradient (hover, 90px, to rgba(0,0,0,.8))
│   ├─ left-btn "+"  20×20 at 8/8, hidden until hover
│   ├─ center-btn "▶" 20×20 centered
│   └─ right-btn "…" 20×20 right
├─ watch bar 3px at y=177: cyan fill / 50% black track
├─ title 14px #F2F2F2, 112px wide, 8px left pad, ellipsis
└─ description 12px #ABABAB, 20px tall
hover/selected → frame becomes 2px #D13B3B
selection → full-card overlay + red bottom bar with white check
watched dots 12px bottom-right: #D1A53B (new) / #009DBD (in progress)
```

### 5.2 List row

`1608×37`, bg `rgba(51,54,57,.8)` (hover `rgba(65,69,73,.8)`), 1 px bottom hairline,
title 14 px left at x+16, year 12 px right-aligned, hover actions `+ ▶ …` and a
4 px red left bar when selected.

### 5.3 Tile row

`1608×175`, padding `12 20 12 12`, 100×150 thumb, title 14, summary 12 px clamped to
80 px, 4 px red left selection bar, watch bar 100 px wide under the thumb.

### 5.4 Toolbars

- **Top**: logo (a Synology lockup: play-triangle mark + "Vídeo station" wordmark, 170×32),
  home button 28 px, search field 214×48 (magnifier + "Keyword" placeholder + funnel at
  right), then user dropdown, help, offline-conversion queue, settings — all 28 px icons
  with 40 px hit cells.
- **Navbar**: category dropdown (title + chevron, 136×26) on the left; on the right the
  action cluster (selection, view mode, sort). In selection mode the whole cluster is
  replaced by 8 action icons.
- **Breadcrumb**: `Recently Added › The King's Speech` replaces the category dropdown on
  detail pages; detail also adds a 2-button actionbar (cast device, overflow) at the far right.

### 5.5 Dialogs (distinctive, worth adapting)

- 1 px **red top accent line**, centered title in red 13 px, close `×` at right.
- Tabs with red underline for the active tab (Library, Video Info, Poster…).
- Footer: gray **Cancel** + blue **OK** (right-aligned); info dialogs get a single **Close**.
- Sizes: edit 684×572, media info 684×~500, settings 888×594.
- Form rows: 13 px gray label column (~110 px) + full-width input with 1 px border,
  token/chip inputs for Genre and Cast (chip with `×`, dropdown chevron), date field with
  calendar button, 1 px separators between sections.
- Toolbar row inside dialogs: `Search from Video Info Plugin` (primary) ·
  `Select from existing videos` (toggle) · `Reset` (right).

### 5.6 Pagination & counts

Centered `‹ ‹‹ 1 2 3 ›› ›` (28 px buttons, active page is a red square), bottom-right
`676 items` with an info icon; thin horizontal scrollbar above it in list/tile views.

## 6. Field lists (directly reusable for our data model/UI)

**View media info › Overview**
`File name · File path · Duration · Size · Resolution · Container · Video codec ·
Audio codec · Video bitrate`

**› Video**
`Video codec · Resolution · Rotation · Frame per second · Profile · Level`

**› Audio** (repeats per track)
`Audio - #N · Audio codec · Audio channel · Sample rate · Language`

**Edit video info › Video Info**
Mode: `Search from Video Info Plugin` / `Select from existing videos` / `Reset`
Fields: `Title · Tagline · Release date · Classification · Rating · Genre (chips) ·
Cast (chips) · Writer (chips) · Director (chips) · Lock status: Lock/Unlock · Summary ·
Source`
Tabs: `Video Info · Poster · Backdrop` — poster/backdrop offer `Original`, `Enter URL`,
`Select from folders`, `Select` (plus a pointer to Settings › Advanced › Video Cover Settings).

**Subtitle search**: `Language · File name · Source`, empty state "No match found."

**Settings › Library**: tree of library → paths (a library can hold **multiple paths**),
toolbar `+ / search / filter / upload`.
**Settings › Video Info Plugin**: table `Video Info Source · Connection status · Apply`,
grouped by `Movie` and `TV Show`, TMDB listed as "(System default)".

## 7. Labels observed (verbatim)

```
Library · Playlist · Movie · TV Show · Home Video · TV Recording
Recently Added · Recently Watched · Recently Released · All · By Folder ·
By Channel · By Program · Timeline
Thumbnail · Tile · List · Selection Mode · Sort order
Favorites · Watchlist · Shared Videos
Watched · Unwatched · View all
View media info · Edit video info · Correct video info · Search for subtitles ·
Public sharing · Mark as watched · Mark as unwatched · Delete · Delete file ·
Offline Convert · Offline Conversion
Season · Episode N · Summary · Genre · Duration · Cast · Writer · Director ·
Reference · Subtitles · Audio Tracks · Classification · Rating · Tagline ·
Release date · Lock status
Library / Parental Control / Video Info Plugin / DTV / Privileges / Advanced / Options
Video Info Source · Connection status · Apply · Test Connection
```

Localization note: this library contains zh titles; the UI stays enu, so titles render
as-is. Search matched English and CJK titles.

## 8. Corrections to our earlier assumptions

| Assumption | Reality |
|---|---|
| "VS has no metadata lock" | **Wrong.** `Edit video info` has `Lock status: Lock/Unlock`. Our per-field locks are still finer-grained, but item-level locking is parity, not an advantage. |
| "VS has no poster management" | **Wrong.** Poster/Backdrop tabs with Original / URL / from-folder selection. |
| "VS scrapers = TMDB + TVDB + Douban" | Here only **TMDB** is installed ("System default") for both Movie and TV; other providers are installable plugins. |
| "VS has no manual re-match" | It has two flows: **Edit video info** (search/select from plugin, or existing) and **Correct video info** (from the detail overflow menu). |
| "View modes are poster/list" | Three modes: **Thumbnail / Tile / List**. |
| "Collections are user playlists only" | There are three built-in collections (Favorites, Watchlist, Shared Videos) under **Playlist**. |
| "Home hero is a poster row" | It is a **collapsing stack**: active item expands to backdrop+meta, siblings collapse to 60/96 px slivers, auto-advancing. |

## 9. Design decisions this unlocks

Adopt (they *are* the product to its users):

1. Fixed 200 px left rail + 48 px top bar + 48 px context bar. Cheap, familiar, works.
2. Poster-first grid with 2:3 cards and hover-revealed actions; **red** selection frames
   and red check-bar in selection mode.
3. Three watch-state signals: 3 px cyan progress line, amber "new" dot, teal dot.
4. **Category menu** semantics (Recently Added / Watched / Released / All / By Folder /
   Timeline) — implement as URL-state filters, not ExtJS menus.
5. **Selection mode swaps the toolbar** — keep this pattern, add keyboard shortcuts and
   a sticky bottom action bar for accessibility.
6. Media info split into Overview / Video / Audio with the exact field list above.
7. Edit dialog split into Video Info / Poster / Backdrop, with plugin-search vs. existing
   choice and a lock toggle (we upgrade it to per-field locks).
8. Detail page anatomy: poster + play overlay + unwatched chip + 4 social actions +
   inline metadata + cast + reference links + track dropdowns + summary + "View all".
9. Breadcrumb + bottom-corner back/next arrows for item-to-item navigation.
10. System collections surfaced first in Playlist (Favorites / Watchlist / Shared).

Modernize (keep semantics, change execution):

| VS | Ours |
|---|---|
| 13/12 px body text, 37 px rows | 14/13 px, 44–48 px rows, density toggle |
| Fixed 120 px columns, 11 columns at 1920 | `repeat(auto-fill, minmax(172–200px,1fr))` |
| Hover-only actions, no focus ring | `:focus-within` reveals actions; 2 px focus ring |
| Red 2 px hover border (reads as error) | Hover = elevation + hairline; red only for selected/active/recording |
| ExtJS modal soup, 3 dialog types for identity | One side panel with tabs (Info / Art / Files) + inline validation |
| Poster-stack hero, 1 s collapse animation | Static 16:9 backdrop + floating 2:3 poster, no auto-rotate |
| Sprite icons at 1x/2x | Lucide 24 px SVGs, `currentColor`, tree-shaken |
| Fixed pixel layout | Fluid grid + container queries; type scale via tokens |
| No empty-state guidance ("No data") | Actionable empty states ("Add a library", "Scan now") |
| `ext:qtip` tooltips | Native `title` + accessible descriptions |

## 10. Parity checklist for M1–M3 (derived from what we saw)

- [ ] Libraries with **multiple paths**, type = Movie / TV Show / Home Video / TV Recording
- [ ] Grid: Thumbnail / Tile / List, persisted per user
- [ ] Category filters: Recently Added / Watched / Released / All / By Folder / Timeline
- [ ] Sort (title, year, date added, rating, last watched) + facet filter (genre, cast, year, rating)
- [ ] Global search across libraries with "All" scope and empty state
- [ ] Pagination + item counts
- [ ] Poster card: hover actions, watch bar, new/in-progress dots, selection overlay
- [ ] Selection mode: toolbar swap, bulk watched/unwatched, add to playlist, delete (admin), convert
- [ ] Detail: poster + play, unwatched chip, favorites/watchlist/playlist/share actions,
      media badges (resolution/audio/codec), certification chip, cast/writer/director,
      external reference links, subtitle/audio track dropdowns, "View all" summary,
      breadcrumb, prev/next item
- [ ] Show detail: season tabs, episode stills, episode numbering, "next up"
- [ ] Media info dialog (Overview/Video/Audio, per-track audio)
- [ ] Edit metadata: fields, chips, poster/backdrop (URL / folder / generated), lock
- [ ] Identify/Correct flow ("Search from Video Info Plugin", select from existing)
- [ ] System collections (Favorites, Watchlist, Shared) before user collections
- [ ] Home Video: landscape cards, filename title, date sort
- [ ] Settings: Library, Video Info Plugin (with priority order + connection test),
      Parental Control, Privileges, Advanced (cover settings), Options
- [ ] User menu, offline conversion queue, help

## 11. Still unprobed

1. **Player UI** — needs a playback session (writes watch progress + transcode temp files).
   We have the CSS spec (controls: prev/play-pause/next, seek ±, volume, rate, subtitle,
   audio, fit/fullscreen, cast, screenshot, auto-hide + `cursor:none`).
2. **Public share dialog** — opening it likely creates a share record.
3. **Privileges / Parental Control / Advanced / Options** tabs — captured as images but
   their inner controls need a closer read (`18-settings-*`, `26-settings-*`).
4. **Favorites/Watchlist contents** — empty for the temp user.
5. DVR screens — no tuner configured.

If we want these, the cheapest path is a user-side capture (play a 10-second clip, open
share dialog) rather than me writing to the instance.

## 12. Provenance & legal

- Screenshots and DOM dumps live in `~/vs-probe/screens/` (not committed) and contain the
  user's library titles.
- Do **not** copy Synology CSS, sprite PNGs, fonts (Handwritten Crystal has unclear
  licensing), the logo/wordmark, or UI text wholesale.
- Re-create icons with Lucide (ISC) + custom SVGs; use Inter or Roboto (Apache/OFL);
  ship our own wordmark. Keep *structure and vocabulary*, change *execution*.

## Geometry

Video Station is almost entirely square-cornered: dialogs, buttons, inputs, cards, chips
and the hero use 1–3 px radii; only avatars, play buttons and progress dots stay circular.
Our theme follows that (`--radius-card` / `--radius-panel` = 2 px, the `rounded-*`
utilities are overridden to 1–3 px, `rounded-full` is kept for circles).
