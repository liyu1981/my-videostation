#!/usr/bin/env bash
# Runs the Wholphin (Android TV) contract validator against a server.
set -euo pipefail
cd "$(dirname "$0")"
exec kotlin wholphin.main.kts "$@"
