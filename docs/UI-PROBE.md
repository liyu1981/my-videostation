# Video Station UI Probe — Findings & Re-creation Notes

Probe of the user's live instance, to extract look-and-feel that we can
re-create **without copying Synology assets**.

---

## 0. TL;DR

| Question | Answer |
|---|---|
| Can we see the app UI without a DSM session? | **No.** All `.html` routes return DSM's 404 (even `/webman/index.html`). |
| What *is* publicly served? | The app CSS (~1.25 MB total), fonts, and all icon/illustration PNGs. |
| Did we get enough for a design system? | **Yes** — exact colors, type scale, layout metrics, card behaviour, icon inventory. |
| What's still missing? | Rendered screenshots, exact navigation labels, and per-screen DOM. Needs a DSM login (see §9). |
| Video Station build probed | **3.0.7-2512** on DSM **7.1.1-42962** (DS218+, `apollolake`, enu) |

Nothing Synology-copyrighted is committed to this repo. Raw assets were inspected in
`/home/yli/vs-probe/` (local only, not in git). This document contains only
measurements and distilled design intent.

---

## 1. Probe method & evidence

Unauthenticated probes that succeeded:

```
GET /webman/3rdparty/VideoStation/style.css                 200  1,027,330 B
GET /webman/3rdparty/VideoStation/controller/ui/style.css   200    219,302 B
GET /webman/3rdparty/VideoStation/font/Roboto-Regular.ttf   200    145,348 B
GET /webman/3rdparty/VideoStation/images/2x/**              200    155/191 assets
GET /webapi/query.cgi?api=SYNO.API.Info&method=query&query=all  200  (1,195 APIs)
GET .../SYNO.Core.Desktop.UIString&method=getjs             200  1,480,233 B
```

Failed / blocked:

```
/index.html, /webman/index.html, /controller/ui/index.html  → 404 (all HTML gated)
/webapi/entry.cgi?api=SYNO.Core.System&method=info          → {"error":{"code":119}} (needs SID)
/video/, /video/index.html                                  → 404
```

Signals used:

- `@import "controller/ui/style.css?v=3.0.7-2512"` → **app version**.
- Class prefixes `syno-vs2-*` (app) and `syno-vc-*` (video controller/player).
- `title`/`description`/`summary` rules and poster geometry → grid spec.
- 1,195 API names → capability inventory (cross-checked in `GAPS.md`).
- Icon asset names + a rendered contact sheet (155 assets) → iconography audit.

So: **the CSS is effectively the app's design source of truth**, because Video Station is
an ExtJS 3.4 app with a large hand-written stylesheet — almost all visual decisions are
in there.

---

## 2. Design tokens (measured, not guessed)

### 2.1 Color

| Role | Value | Where it's used (verified in CSS) |
|---|---|---|
| App/field background | `#222426` | inputs, text fields, base surface |
| Panel/window surface | `#2C3034` | menus, combo lists, dropdowns, dialogs |
| Deep surface | `#2A2C30` | hero/rotating data view background |
| Raised surface | `#333639` | tile rows, badges (text color on yellow chip) |
| Surface hover | `#414549` | hovered/selected tile + list row |
| Border / divider | `#272A2E`, `#393C3F` | hero frame (4 px), block backgrounds |
| Hairline | `rgba(255,255,255,.10)`, top `.20` | poster frame border, list separators |
| Text primary | `#F2F2F2` | poster titles, row titles, buttons |
| Text heading | `#E9E9E9` | detail title, hero meta |
| Text secondary | `#CFCFCF` | taglines, info lines, certificate chips |
| Text tertiary | `#ABABAB` | descriptions, dates, season tabs |
| Text disabled | `#686868` / `#666` | inactive controls |
| **Brand red** | `#D13B3B` | **active/selected state**, season underline, selection border, recording, "today" |
| Accent cyan | `#00D4FF` | **watch progress**, tag hover, links in detail |
| Teal | `#009DBD` | "last watched" dot |
| Amber | `#D1A53B` | "unwatched (new)" dot |
| Highlight yellow | `#E6C929` (bg) + `#333639` (text) | "Watched" status chip |
| Link/secondary blue | `#19A1FF`, `#2D89C9` | DSM-level links/primary buttons |

Two-accent system is intentional: **red = state/selection/brand**, **cyan = progress +
interactive highlight**. Amber/teal are semantic watch-state dots. This is worth keeping —
it gives immediate "what's selected / what's in progress" reads at poster-grid density.

### 2.2 Typography

```
Roboto                 400/700   body, titles, buttons
Roboto Slab            bold      rating/certificate chips (e.g. "PG-13", IMDb star)
Handwritten Crystal v2 -         home-video titles / personal-video covers
fallback               verdana, tahoma, arial, helvetica, sans-serif
```

Observed sizes (px):

| Size | Usage |
|---|---|
| 36 | Detail page title |
| 30 | Hero (rotating banner) title |
| 24 | Detail info bar (year / rating / duration) |
| 20 | Home row section titles, library names |
| 18 | Detail tagline |
| 16 | Section subtitle ("Summary"), season labels, certificate chips, player option labels, hero tagline |
| 14 | Poster card title, list row title, body/summary text |
| 12 | Card description/date, list episode/date, badges |

Line heights observed: 20 px for 14 px titles, 20 px for 12 px descriptions. Tight.

### 2.3 Layout metrics (grid, spacing, elevation)

| Element | Spec |
|---|---|
| Window header bar (`syno-vc-header-panel`) | **50 px** tall, dark gradient |
| Navigation toolbar (`nav-toolbar`) | **48 px** tall |
| Poster card (grid view) | **120 × 180** (2:3), cell **120 × 224**, margin **0 20 14 0** |
| Landscape card (episodes) | **180 × 120** (3:2), cell 180 × 164, margin **0 17 12 0** |
| Tile-view row | **150 px** tall, thumb **100 × 150**, padding **12 20 12 12** |
| List-view row | **36 px** tall, padding **0 16**, icon 24 px, title 14, episode column 60 px |
| Home hero (regular) | poster **226 × 340**, backdrop **528 × 340**, frame 4 px `#272A2E`, radius 2, right margin 56 |
| Home hero (large mode) | poster **360 × 540**, backdrop **840 × 540** |
| Home hero collapsed | poster **60 px** (96 px large mode) |
| Detail poster | **270 × 388**, play button **76 × 76** centered (top 156 / left 97) |
| Detail page padding | `20 0 0 68px` (left gutter 68) |
| Home page padding | `0 0 0 56px` |
| Season bar (TV show) | **48 px**, active tab underlined with `#D13B3B` |
| Watch progress bar | **3 px**, pinned to poster bottom, cyan on 50 % black |
| Watched dot | **12 px**, bottom-right (8/4), amber or teal |
| Card shadow | `0 2px 2px rgba(0,0,0,.3)` |
| Card border | 1 px `rgba(255,255,255,.1)`, hover/selected **2 px `#D13B3B`** |
| Radius | 2 px (cards, chips) — near-square, "media library" feel |
| Hover scrim | bottom-up gradient to `rgba(0,0,0,.8)` over 90 px |
| Hero scrim | left→right `rgba(0,0,0,.5)` → transparent |
| Motion | action buttons slide up `translateY(-36px)` over ~0.2 s; home hero collapse 1 s |

Spacing rhythm is roughly 4/8/12/16/20/28/36/56/68. Gutters are generous (20 px horizontal)
because cards are small; vertical rhythm is tight (14 px).

---

## 3. Screen anatomy (reconstructed from CSS + API surface)

### 3.1 Shell

```
┌───────────────────────────────────────────────────────────────┐
│ header (50px): logo · library picker · search · queue · settings│
├───────────────────────────────────────────────────────────────┤
│ nav-toolbar (48px): category tabs / breadcrumb pathbar        │
│   Movies · TV Shows · Home Videos · TV Recording · Collections │
├───────────────────────────────────────────────────────────────┤
│ body                                                          │
│   ┌ home ──────────────────────────────────────────────────┐  │
│   │ rotating hero (226×340 poster stack + 528×340 backdrop)│  │
│   │ section rows: icon(28) + title(20px) + ‹ › (24px)      │  │
│   │   poster row height 274 · landscape row height 212     │  │
│   └────────────────────────────────────────────────────────┘  │
│   ┌ browse ────────────────────────────────────────────────┐  │
│   │ options bar: sort · filter · view mode (poster/list/tile)│ │
│   │ grid 120×180 cards, hover actions, selection bar        │  │
│   └────────────────────────────────────────────────────────┘  │
│   ┌ detail ────────────────────────────────────────────────┐  │
│   │ poster 270×388 + 76px play · title 36 · tagline 18      │  │
│   │ info bar 24 (year · stars · duration · cert chip)       │  │
│   │ action row: watchlist · favorite · playlist · share      │  │
│   │ tabs/sections: episodes, cast, recommendations          │  │
│   └────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────┘
player window: full-bleed video + auto-hiding control bars
```

Notes:
- The **hero is a rotating stack of posters** (poster image + wide backdrop panel that
  expands on hover), not a full-width billboard. Distinctive; expensive in CSS but cheap
  to modernize as a 16:9 backdrop hero with the poster as a floating card.
- Rows are **horizontally sliding** with 24 px prev/next buttons, height fixed per row type.
- The nav has **two bars**: a category bar and a path bar (breadcrumbs), plus a separate
  action bar that appears in selection mode (`navbar-select-actionbar`). Selection mode
  swaps the whole toolbar.
- A `library-icon` (28 px) prefixes each home row and is keyed by media type:
  `library`, `playlist`, `movie`, `tvshow`, `home_video`, `tv_record`.

### 3.2 Detail page (movie)

- Left: 270×388 poster with a 76 px circular play button overlay, 3 px progress line at the
  poster's bottom edge, 1 px frame.
- Right: title 36/`#E9E9E9`, tagline 18/`#CFCFCF`, info bar at 24 px (year with 24 px right
  margin, 5-star rating at 24 px per star, duration, certificate chip with
  `Roboto Slab` bold and 1 px `#CFCFCF` border, radius 2, padding `0 8`).
- Below: action chips `view-btn` (1 px border, radius 2, 12 px text) for versions/qualities,
  then a summary block (label 16, body 14), genre tags that turn cyan on hover.
- Action icons (28 px sprite): watchlist, favorite, playlist/collection, public sharing.
- Prev/next item arrows float at the bottom corners of the page (36 px).

### 3.3 TV show

- Same left poster (270×388) but the right side is an episode browser:
  **season bar 48 px** with text tabs; selected tab gets a red underline, unselected are
  `#ABABAB`.
- Episodes are **landscape cards 180×120** (3:2) with the same hover-action pattern.
- Back/next season arrows in the season bar.

### 3.4 Player

- Control bar contains: playlist, previous, seek back, play/pause, seek forward, next,
  volume, playback-rate, subtitle, audio track, fit/fullscreen, "streaming/remote" (cast),
  screenshot.
- Options bar above shows title + `player-option` labels at 16 px, playback-rate panel,
  volume popup, subtitle search dialog.
- Auto-hide panels: `hide-panels` sets `cursor:none` on the whole window (classic
  media-player behaviour).
- The window class list still contains `vlc` (legacy naming from the old VLC-based player).

### 3.5 Other screens present

| Screen | Evidence |
|---|---|
| Home videos | `home_video` library type, handwritten font, date sort |
| TV recording + DTV wizard | `wizard-dtv`, `wizard_welcome/transcoding/end`, `schedule` view |
| Collections / playlists | `icon_category_playlist`, `.view-playlist`, smart playlists (`icon_indicator_smart_playlist`) |
| Sharing panel | `vc-sharing-*` classes: poster, screenshot, filename, info, play icon, empty state |
| Settings | `icon_tab_setting`, `icon_tab_transcoding`, fieldsets, 4-step transcoding wizard |
| Job/progress | `syno-vs2-progress`, `offline-conversion-status-dataview`, `syno-vs2-toast` |
| Parental control | `icon_indicator_parental_on/off`, `icon_default_parental_on_xl` |
| Cast/remote | `icon_btn_device`, `icon_btn_remote`, `signal-panel`, `VideoController.*` APIs |
| Empty states | 320 px gray illustrations: `no_data`, `lock`, `fav`, `share`, `player_error` |

---

## 4. Card & selection behaviour (the core interaction)

Poster card state machine, as measured:

| State | Visual |
|---|---|
| Rest | poster, 1 px `rgba(255,255,255,.1)` frame, 3 px progress line if partially watched, 12 px watch dot if new/last-watched |
| Hover | 90 px bottom gradient scrim; three 20 px buttons slide up: **add** (left), **play** (center), **more** (right); frame becomes 2 px `#D13B3B` |
| Selected | `multi_select.png` overlay (120×180) + red 2 px frame; toolbar switches to `navbar-select-actionbar` |
| Playlist edit mode | 36 px buttons (edit / more) instead of 20 px |
| Tile view | 4 px red left bar (28 px tall) instead of a full frame |
| List view | row bg `rgba(51,54,57,.8)` → `rgba(65,69,73,.8)` on hover/selected, red 4 px left mark |

Accessibility liabilities we should **not** copy: hover-only affordances, 12 px text,
36 px rows, colour-only selection, no visible focus ring in the stylesheet.

---

## 5. Iconography

- 155 assets in the public set; **24 px grid**, exported at `1x`/`2x`, drawn as **1 px
  strokes**, mostly monochrome white-gray, with a **red or cyan accent** on state.
- Multi-state icons are **vertical sprites** (e.g. 24×96: normal / hover / active /
  disabled), applied via `background-position: 0 -24px` etc. We should replace this with
  `<lucide>` components + CSS `currentColor` states.
- Media-type icons are the **only fully coloured/illustrated** glyphs
  (`category_movie`, `category_tv_show`, `category_video`, `category_tv_recording`,
  `category_library`, `category_playlist`, `category_calendar`, `category_dongle`).
- Empty states use **large flat illustrations** (320×320) in gray, one accent element.
- Logo: play-triangle mark + "station" wordmark (Synology lockup). **Do not reuse.**

Suggested mapping for our icon set (Lucide names):

| VS glyph | Semantic | Lucide replacement |
|---|---|---|
| category_movie | Movies library | `clapperboard` |
| category_tv_show | TV library | `tv` |
| category_video | Home videos | `video` |
| category_tv_recording | DVR | `radio-tower` / `cast` |
| category_library | Library | `library` |
| category_playlist | Collections | `list-video` |
| indicator_watchlist | Watchlist | `bookmark` |
| indicator_fav | Favorites | `heart` |
| indicator_loop | Loop/shuffle | `repeat` |
| indicator_lock / parental_on | Parental lock | `lock` |
| btn_scan / reindex | Scan library | `refresh-cw` / `radar` |
| btn_streaming / device / remote | Cast | `cast` |
| btn_screenshot | Capture frame | `camera` |
| btn_tick | Watched | `check` |
| indicator_signal | Connection quality | `signal` |
| btn_playback_rate | Speed | `gauge` |

---

## 6. Terminology observed (for parity of labels)

Library types: **Movie**, **TV Show**, **Home Video**, **TV Recording**, plus
**Playlist/Collection** and **Smart Playlist**.
Item states: **Watched**, **Unwatched**, **Last watched**, **Watchlist**, **Favorite**,
**Shared**.
Actions: **Play**, **Add to**, **More**, **Edit video info**, **Change video info**
(manual re-match), **Search subtitles**, **Offline conversion**, **Public sharing**,
**Re-index**, **Scan**, **Filter**, **Sort**, **View mode: poster/list/tile**.

Exact on-screen English strings still need a logged-in capture (see §9).

---

## 7. What to keep, what to modernize

### Keep (it *is* Video Station to its users)

1. **Information architecture**: Home → category → library → item → season/episode.
2. **The four library types** incl. home videos and recordings, as first-class concepts.
3. **Poster-first browsing** with 2:3 posters and 3:2 episode stills.
4. **Watch-state language**: 3 px progress line, amber "new", teal "in progress", red "active".
5. **Row-based home** with horizontally sliding shelves and per-row library icons.
6. **Manual "Change video info"** affordance, always reachable from the item.
7. **Dense metadata detail page**: year, rating, runtime, certification, codecs, tracks.
8. **Immediate "Continue watching"** and per-title resume.

### Modernize (do not reproduce)

| VS pattern | Our replacement |
|---|---|
| ExtJS 3.4 + 1.2 MB hand-written CSS + sprite sheets | Tailwind 4 tokens + CSS vars + Lucide SVG |
| Hover-only action buttons, no focus states | Always-visible on focus-within; keyboard-first; visible rings |
| 12 px secondary text, 36 px rows | 13–14 px minimum, 44–48 px rows |
| Red hover border (reads as error) | Red only for **active/selected/recording**; hover = elevation + ring |
| Fixed 120 px column grid | Responsive `minmax()` grid: 160/184/208/232 px poster widths |
| Rotating poster-stack hero (340 px tall, 1 s collapse animations) | 16:9 backdrop hero with floating poster + progress, no auto-rotate |
| Modal-heavy metadata editing | Inline side panel, field-level locks, optimistic save |
| Sprite/PNG icons, 1x/2x duplication | 24 px SVG, `currentColor`, tree-shaken |
| 2 px radius, near-square cards | 8 px radius cards, 12 px panels (cinematic but modern) |
| Dark-only, fixed blue-gray | Dark-first with light theme, neutral-slate base, tinted surfaces |

### Proposed modern token set (starting point for `web/src/styles/theme.css`)

```css
@theme {
  /* surfaces (VS-derived, slightly cooler/deeper) */
  --color-bg:            #0F1115;
  --color-surface:       #16191E;
  --color-surface-2:     #1E2229;
  --color-surface-3:     #262B33;
  --color-hairline:      rgb(255 255 255 / 0.08);
  --color-hairline-strong: rgb(255 255 255 / 0.16);

  /* text */
  --color-fg:            #F2F4F6;
  --color-fg-heading:    #E9EDF1;
  --color-fg-muted:      #A9B1BB;
  --color-fg-dim:        #737B86;

  /* semantic (from VS, renamed for clarity) */
  --color-active:        #E5484D;  /* was #D13B3B: selection, live, record */
  --color-progress:      #22D3EE;  /* was #00D4FF: playhead, progress */
  --color-new:           #D1A53B;  /* unwatched */
  --color-continue:      #009DBD;  /* in progress */
  --color-rating:        #E6C929;  /* stars */

  --radius-card: 8px;
  --radius-panel: 12px;
  --shadow-card: 0 2px 8px rgb(0 0 0 / 0.35);
}
```

Type scale: 12 / 13 / 14 / 16 / 20 / 24 / 32 / 40 (Inter or Roboto Flex + Roboto Slab only
for rating chips). Poster grid: `repeat(auto-fill, minmax(184px, 1fr))`, gutter 16/24,
2:3 aspect via `aspect-ratio`. Episode stills: 16:9 (modern) instead of 3:2, with a
`screenshots` fallback.

---

## 8. What this changes in our plan

1. **Icon strategy** — add an explicit icon map (Lucide) and forbid bitmap icons.
   *Action:* `docs/DESIGN.md` (or §7 here) becomes the reference for M1 UI work.
2. **Home page** — VS's rotating hero is a signature; we should ship *a* hero in M2, but as
   a static 16:9 backdrop with poster overlay (cheap, no JS animation).
3. **Selection model** — adopt the "selection mode with swapped toolbar" pattern; it is
   how VS users do bulk edits and is a real workflow.
4. **Watch-state dots + 3 px progress line** — cheap, high recognizability. Adopt exactly
   (semantics), restyle (radius, colour tokens).
5. **Density** — VS is denser than modern web apps; we should keep a *comfortable* and a
   *compact* density toggle (Jotai atom), defaulting to comfortable.
6. **Naming** — keep "Home Video", "Watchlist", "Collection", "Change video info" wording so
   migrating users feel at home.
7. **Player controls** — the VS control set (prev/next, seek, rate, subtitle, audio,
   fit/fullscreen, cast, screenshot) is a good checklist for M4/M5 UI completeness.

---

## 9. Remaining probe work (needs an authenticated session)

All HTML/JS is behind DSM auth, so the following still needs one logged-in pass:

1. Screenshots of: Home, Movies grid, TV Shows, show detail (seasons/episodes), movie
   detail, player with controls, settings › library, settings › transcoding, task monitor,
   selection mode, share dialog, parental control, DTV/schedule.
2. Exact English UI strings (`innerText` dump) for label parity.
3. Computed styles/DOM for the two toolbars (so we can reproduce information density, not
   just colours).
4. The **metadata edit / "Change video info"** dialog flow, field by field.
5. Whether the library list is per-user or global, and how permissions render.

**Fastest path** (needs a temporary DSM account, ideally read-only admin):

```
1. I log in with SYNO.API.Auth (v6, session cookie, no token persistence)
2. Open ...?launchApp=SYNO.SDS.VideoStation.AppInstance in headless Chromium
3. Walk the screens above; capture PNG + innerText + computed stylesheet values
4. Store screenshots in docs/ui-probe/screens/ (local reference), keep only
   measurements + labels in git
```

Fallback without credentials: you open the app, and for each screen run the snippet in
DevTools console that I provide (dumps innerText, key element box sizes, computed
colors/typography) and share the JSON; screenshots optional.

---

## 10. Provenance & legal

- Measurements above are facts about a UI; they are fine to use.
- **Do not** copy: `style.css`, sprites/PNGs, fonts (Handwritten Crystal has unclear
  licensing), the logo, or Synology's wording/illustrations.
- Re-create iconography with Lucide (ISC) + custom SVGs; use Inter/Roboto (Apache/OFL) or
  system UI; ship our own wordmark.
- Keep parity in *structure and semantics*, innovate in *execution*.
