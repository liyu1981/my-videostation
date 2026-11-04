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

- **Claimed server version:** `12.4.0` (current Jellyfin 12.x line). This is a hard
  gate: current `jellyfin-sdk-kotlin` clients (Findroid, Android TV, Streamyfin)
  require a minimum of `12.0.0` and reject a 10.x claim. Bump deliberately; every bump
  requires re-running the client matrix.
- **Product name:** `ProductName` must be exactly `Jellyfin Server` — the Jellyfin
  SDK's server discovery scores anything else `BAD` ("Unsupported version or
  product") and refuses to connect. `ServerName` stays `MyVideoStation` (that is
  what client UIs display).
- **Qualified clients:** native clients only — Jellyfin Android TV / Google TV,
  Jellyfin iOS/Swiftfin, and native third-party apps (Findroid, Streamyfin). The
  Jellyfin **Android phone app is a WebView shell that loads jellyfin-web from the
  server root**, so it cannot work unless we ship jellyfin-web; it is out of scope.
  Support is defined as "these builds work"; other clients are best-effort.
- **Auth:** opaque session tokens (same token format as our `mvs_session` cookie),
  30-day expiry, accepted via `X-Emby-Authorization: MediaBrowser ... Token="..."`,
  `X-Emby-Token`, or `?api_key=`. Tokens are ordinary `sessions` rows — no second
  auth system.
- **IDs:** 32 lowercase hex: 4-byte kind + an 8-byte Feistel-encrypted numeric id
  keyed by the persisted server id. Stable across restarts, opaque to clients, and
  non-enumerable. This is what makes the Jellyfin-style anonymous artwork/stream
  endpoints safe.
- **Playback:** HLS via our M5 session pipeline. `DeviceProfile` is ignored in v1;
  we transcode unless our own direct-play matrix says the file is browser-native.
  This trades CPU for a much smaller capability matrix. `/Items/{id}/Images/*` and
  `/Videos/{id}/stream?static=true` are anonymous (Jellyfin clients fetch them
  without auth headers); opaque ids prevent enumeration.

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
