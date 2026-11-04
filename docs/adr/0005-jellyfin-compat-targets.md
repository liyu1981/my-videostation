# ADR 0005 — Jellyfin compatibility: scope and pinned targets

Status: accepted · Date: 2026-11-04

Closes the "should we spoof a Jellyfin server?" question (GAPS D-1) with a concrete
contract. Implementation plan: `docs/JELLYFIN-COMPAT.md`.

## 1. Decision

Expose a **subset** of the Jellyfin REST API from the same binary, mounted at the
root (`/System`, `/Users`, `/Items`, `/Videos`, ...), as an adapter over our own
store/services. Keep `/api/v1` as the canonical API; the adapter never becomes the
source of truth.

We do **not** implement Jellyfin's plugin/config/playlist/live-TV surface, and the
Jellyfin **web client** is out of scope for v1.

## 2. Pinned targets

- **Claimed server version:** `10.10.7` (stable line). Bump deliberately; every bump
  requires re-running the client matrix.
- **Qualified clients:** Jellyfin Android (mobile) and Android TV / Google TV first,
  Jellyfin iOS second. Support is defined as "these builds work"; other clients are
  best-effort.
- **Auth:** opaque session tokens (same token format as our `mvs_session` cookie),
  30-day expiry, accepted via `X-Emby-Authorization: MediaBrowser ... Token="..."`,
  `X-Emby-Token`, or `?api_key=`. Tokens are ordinary `sessions` rows — no second
  auth system.
- **IDs:** 32 lowercase hex, structurally reversible (kind + numeric id). Stable
  across restarts; clients treat them as opaque.
- **Playback:** HLS via our M5 session pipeline. `DeviceProfile` is ignored in v1;
  we transcode unless our own direct-play matrix says the file is browser-native.
  This trades CPU for a much smaller capability matrix.

## 3. Consequences

- Client updates are a compatibility risk: the adapter is impersonating a specific
  server version. The endpoint matrix in `docs/JELLYFIN-COMPAT.md` §11 is the
  supported surface and must be updated when a client needs a new endpoint.
- `api_key` in URLs is inherent to the protocol; HTTPS/reverse proxy is the
  documented recommendation, and query tokens must never be logged.
- Transcode concurrency matters: family use needs `MVS_TRANSCODE_SESSIONS` above the
  default 2 once playback lands.
- We can close GAP-030/031 without shipping or maintaining native apps, but we owe
  users an honest statement that this is a compatibility layer, not a Jellyfin
  server.

## 4. Rejected alternatives

- **Fork/wrap Jellyfin's server** — defeats the single-binary, SQLite, no-.NET goal.
- **Only document a Jellyfin sidecar** (`jellyfin + our files`) — two servers, two
  libraries, two scan pipelines; worse than maintaining an adapter.
- **Implement the full API including the web UI surface** — large, mostly unused by
  mobile/TV clients; revisit only if web-client support is requested.
