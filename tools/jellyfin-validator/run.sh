#!/usr/bin/env bash
# Runs the Jellyfin/Findroid contract validator with the local Kotlin CLI.
# No build system: Kotlin main-kts resolves the SDK from Maven Central on first run.
set -euo pipefail
cd "$(dirname "$0")"
exec kotlin validate.main.kts "$@"
