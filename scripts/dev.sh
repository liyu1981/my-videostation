#!/usr/bin/env bash
# Development runner: Go API + Vite dev server (proxying /api and /media to the API).
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p data

cleanup() {
  trap - EXIT INT TERM
  kill 0 2>/dev/null || true
}
trap cleanup EXIT INT TERM

echo ">> API  http://127.0.0.1:8080"
echo ">> UI   http://127.0.0.1:5173"
echo ">> data ./data  (MVS_DATA_DIR to override)"

go run ./cmd/mvs --listen 127.0.0.1:8080 &
api_pid=$!

# Wait for the API to become healthy before Vite starts.
for _ in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8080/api/v1/health >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$api_pid" 2>/dev/null; then
    echo "API exited early" >&2
    exit 1
  fi
  sleep 0.5
done

(cd web && pnpm dev --host 127.0.0.1 --port 5173) &
wait
