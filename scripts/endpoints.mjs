#!/usr/bin/env node
// Regenerates docs/ENDPOINTS.md from the route registrations in internal/httpapi.
// Run `pnpm endpoints` after adding/removing a route.
import { readdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';

const ROOT = path.resolve(import.meta.dirname, '..');
const API_DIR = path.join(ROOT, 'internal', 'httpapi');

const routeRe = /mux\.HandleFunc\("([A-Z]+) ([^"]+)",\s*(.*)\)\s*$/;

function authOf(handler) {
  if (handler.includes('s.admin(')) return 'admin';
  if (handler.includes('s.mediaAuth(')) return 'token|session|cast';
  if (handler.includes('s.anyAuth(')) return 'token|session';
  if (handler.includes('s.auth(')) return 'session';
  return 'public';
}

const routes = [];
for (const file of readdirSync(API_DIR).filter((f) => f.endsWith('.go')).sort()) {
  const src = readFileSync(path.join(API_DIR, file), 'utf8');
  for (const line of src.split('\n')) {
    const m = line.trim().match(routeRe);
    if (!m) continue;
    routes.push({ file, method: m[1], path: m[2], auth: authOf(m[3]) });
  }
}

const groups = [
  {
    title: 'Canonical API — health, config, auth',
    test: (r) => /^\/api\/v1\/(health|config|auth)(\/|$)/.test(r),
  },
  {
    title: 'Canonical API — libraries, scanning, jobs',
    test: (r) => /^\/api\/v1\/(libraries|jobs|fs)(\/|$)/.test(r),
  },
  {
    title: 'Canonical API — browse, items, metadata, artwork',
    test: (r) => /^\/api\/v1\/(home|browse|search|items|providers|settings|artwork)(\/|$)/.test(r),
  },
  {
    title: 'Canonical API — collections, users, audit, backup, cast',
    test: (r) => /^\/api\/v1\/(collections|users|audit|backup|cast)(\/|$)/.test(r),
  },
  {
    title: 'Canonical API — playback, streaming, transcoding',
    test: (r) => /^\/api\/v1\/(media|playback)(\/|$)/.test(r) || r.startsWith('/hls/'),
  },
  {
    title: 'Jellyfin — discovery, auth, users, views',
    test: (r) =>
      /^\/(System|Branding|QuickConnect|UserViews|DisplayPreferences)/.test(r) ||
      /^\/Users\/(Public|AuthenticateByName|Me)/.test(r) ||
      /^\/Users\/\{userId\}(\/Views)?$/.test(r),
  },
  { title: 'Jellyfin — images (anonymous, opaque ids)', test: (r) => r.includes('/Images') },
  {
    title: 'Jellyfin — playback',
    test: (r) => r.includes('/PlaybackInfo') || /^\/Videos\//.test(r),
  },
  {
    title: 'Jellyfin — progress, sessions, played/favorites, devices',
    test: (r) =>
      /^\/(Sessions|Devices|socket|UserPlayedItems|UserFavoriteItems)/.test(r) ||
      r.includes('/PlayedItems/') ||
      r.includes('/FavoriteItems/') ||
      /^\/UserItems\/\{itemId\}\/UserData/.test(r),
  },
  {
    title: 'Jellyfin — series, seasons, episodes',
    test: (r) => /^\/Shows\//.test(r),
  },
  {
    title: 'Jellyfin — items, browse, facets, search',
    test: (r) =>
      /^\/Items/.test(r) ||
      /^\/Users\/\{userId\}\/Items/.test(r) ||
      /^\/UserItems\/Resume/.test(r) ||
      /^\/(Genres|Persons|Studios|Search|MediaSegments)/.test(r) ||
      /^\/Users\/\{userId\}\/Genres$/.test(r),
  },
];

const used = new Set();
const sections = [];
for (const group of groups) {
  const rows = routes.filter((r) => !used.has(r) && group.test(r.path));
  rows.forEach((r) => used.add(r));
  if (rows.length) sections.push({ title: group.title, rows });
}
const leftovers = routes.filter((r) => !used.has(r));
if (leftovers.length) sections.push({ title: 'Other', rows: leftovers });

const cell = (v) => String(v).replaceAll('|', '\\|');
const lines = [
  '# Endpoint inventory (tracking)',
  '',
  '> **Generated** by `pnpm endpoints` from the route registrations in',
  '> `internal/httpapi/*.go`. Do not hand-edit the tables; add/remove a route in Go',
  '> and re-run the script. This is a tracking document, not a compatibility promise:',
  '> the canonical API is `/api/v1` (see `docs/API.md`), the root Jellyfin paths are',
  '> the client adapter (see `docs/JELLYFIN-COMPAT.md`).',
  '',
  'Auth legend:',
  '',
  '- **public** — no authentication',
  '- **session** — `mvs_session` cookie (SPA)',
  '- **admin** — session + `admin` role',
  '- **token|session** — Jellyfin token (`X-Emby-Token`, `X-Emby-Authorization`,',
  '  `X-MediaBrowser-Token`, `?api_key=`) **or** the SPA cookie',
  '- **token|session|cast** — additionally a signed `?cast=` token (cast receivers)',
  '',
  `Totals: **${routes.length}** registered routes (plus the SPA fallback at \`/\`).`,
  '',
];

for (const section of sections) {
  lines.push(`## ${section.title} (${section.rows.length})`, '', '| Method | Path | Auth |', '|---|---|---|');
  for (const r of section.rows) {
    lines.push(`| ${r.method} | \`${r.path}\` | ${cell(r.auth)} |`);
  }
  lines.push('');
}

lines.push(
  '## Dispatch details',
  '',
  '- `GET /Videos/{itemId}/{rest...}` handles `master.m3u8` (HLS manifest),',
  '  `stream.<ext>` and `<source>/Subtitles/<index>/Stream.<format>`; anything else is 404.',
  '- `GET /socket` is reserved for the Jellyfin realtime socket and returns `400`',
  '  (no WebSocket API yet).',
  '- `GET /` and unknown non-API paths serve the embedded React SPA.',
  '- Lowercase `/items/{itemId}/Images/...` are aliases kept for Findroid, which',
  '  builds image URLs with a lowercase prefix.',
  '',
);

writeFileSync(path.join(ROOT, 'docs', 'ENDPOINTS.md'), lines.join('\n'));
console.log(`✓ docs/ENDPOINTS.md (${routes.length} routes)`);
