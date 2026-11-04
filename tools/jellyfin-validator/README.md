# Jellyfin / Findroid contract validator

Runs the **exact SDK version Findroid 1.2.0 uses** (`jellyfin-sdk-kotlin` 1.9.4)
against a server and walks Findroid's real call sequence. It catches API contract
failures (missing required JSON fields, wrong enum values, hyphenated UUIDs, …)
**before** installing/retrying the Android app.

## Why

Jellyfin's Kotlin SDK models many DTO fields as *required* (no default). A missing
field makes `kotlinx.serialization` throw, and clients surface a generic
"Unknown error" even though the HTTP status was 200. The validator prints the exact
`MissingFieldException` (class + field), which is much faster than log-guessing.

## Requirements

- Java 21+ (only the runtime/compiler already installed)
- Kotlin CLI (`kotlin`, tested with 2.4.21)
- Network access on first run (Maven Central); dependencies are cached afterwards

No Gradle, no Maven, no build files — a single Kotlin main-kts script.

## Run

```bash
cd tools/jellyfin-validator
./run.sh http://192.168.0.221:6500 admin 'your-password'

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

## Notes

- It **writes** watch state (sessions/progress) for the account it logs in with.
  Use a scratch/test account and a dev server, never a production library.
- Server and account are passed on the command line or in `MVS_*` env vars; nothing
  is stored.
- The script is intentionally not part of the Go/pnpm build. Keep it in sync with
  `docs/JELLYFIN-COMPAT.md` when the adapter surface changes.
