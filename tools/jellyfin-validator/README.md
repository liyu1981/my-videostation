# Jellyfin client contract validators

Three validators walk the real call sequences of the Jellyfin clients we support,
against a live server:

| Script | Client | Transport |
|---|---|---|
| `run.sh` | Findroid 1.2.0 (`jellyfin-sdk-kotlin` 1.9.4) | Kotlin SDK |
| `run-wholphin.sh` | Wholphin, Android TV (`jellyfin-sdk-kotlin` 1.7.1) | Kotlin SDK |
| `moonfin.sh` | Moonfin (Flutter; its own Dart API layer) | curl + jq |

They catch API contract failures (missing required JSON fields, wrong enum values,
hyphenated UUIDs, wrong path shapes, …) **before** installing/retrying the app.

## Why

Jellyfin's Kotlin SDK models many DTO fields as *required* (no default). A missing
field makes `kotlinx.serialization` throw, and clients surface a generic
"Unknown error" even though the HTTP status was 200. The validator prints the exact
`MissingFieldException` (class + field), which is much faster than log-guessing.

## Requirements

- Java 21+ and Kotlin CLI (`kotlin`, tested with 2.4.21) for the SDK validators
- `curl` and `jq` for the Moonfin validator
- Network access on first run (Maven Central); dependencies are cached afterwards

No Gradle, no Maven, no build files — the SDK validators are single Kotlin
main-kts scripts; the Moonfin one is a plain bash script.

## Run

```bash
cd tools/jellyfin-validator

# Findroid (jellyfin-sdk-kotlin 1.9.4)
./run.sh http://192.168.0.221:6500 admin 'your-password'

# Wholphin / Android TV (jellyfin-sdk-kotlin 1.7.1)
./run-wholphin.sh http://192.168.0.221:6500 admin 'your-password'

# Moonfin (curl + jq; 71 checks)
./moonfin.sh http://192.168.0.221:6500 admin 'your-password'

# or via env
MVS_URL=http://192.168.0.221:6500 MVS_USER=admin MVS_PASS=... ./run.sh
```

Exit code is `0` only when every step passes.

## What it checks

1. `/System/Info/Public` (product name, version ≥ SDK minimum)
2. `/Branding/Configuration`, `/QuickConnect/Enabled`, `/Users/Public`
3. `POST /Users/AuthenticateByName` + Findroid's `saveAuthenticationResult`
   field access (`user.id`, `user.name`, `serverId`, `accessToken`)
4. `/Users/Me`, `/UserViews`
5. `/Items/Latest`, `/Items/Resume`, `/Shows/NextUp`, `/Items`, `/Items/{id}`
6. `/UserItems/{id}/UserData`, `/MediaSegments/{id}`
7. `POST /Items/{id}/PlaybackInfo` (Findroid's direct-play DeviceProfile) and the
   stream URL it builds (anonymous range request)
8. Anonymous artwork (`/Items/{id}/Images/Primary`)
9. `/Sessions/Capabilities/Full`, `/DisplayPreferences/{id}`,
   `/Devices/Options`, `/Sessions/Playing` + `/Progress` + `/Stopped`

## Moonfin checks

1. `/System/Info/Public`, `/System/Ping`, `/Branding/Configuration`,
   `/QuickConnect/Initiate` (must be 401 so Moonfin falls back to password login)
2. Login, `/Users/Me`, `/UserViews`
3. Home rows: `/Users/{id}/Items/Latest` (array), `/UserItems/Resume`,
   `/Shows/NextUp`
4. Library browse through `/Users/{id}/Items` with fields/filters
5. Detail through the user-scoped `/Users/{id}/Items/{itemId}`, plus
   `/Items/{id}/Ancestors`, `/Similar`, `/ThemeMedia`, `/SpecialFeatures`,
   `/MediaSegments`, user-scoped `/LocalTrailers` and `/Intros`
6. Anonymous artwork and `/Videos/{id}/stream` (byte range)
7. `PlaybackInfo` → MediaSources/streams/PlaySessionId, progress reporting,
   user data, ratings, played/favorite, display preferences, device options
8. `/Users/Configuration`, `/Localization/*`, `/Artists`
9. `/Sessions/Logout` (token must be rejected afterwards)

## Notes

- It **writes** watch state (sessions/progress) for the account it logs in with.
  Use a scratch/test account and a dev server, never a production library.
- Server and account are passed on the command line or in `MVS_*` env vars; nothing
  is stored.
- The script is intentionally not part of the Go/pnpm build. Keep it in sync with
  `docs/JELLYFIN-COMPAT.md` when the adapter surface changes.
