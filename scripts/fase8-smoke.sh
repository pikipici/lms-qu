#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${E2E_BASE_URL:-http://127.0.0.1:8200}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/frontend"

log() {
  printf '[fase8-smoke] %s\n' "$*"
}

log "base url: $BASE_URL"

log "healthz"
curl -fsS "$BASE_URL/api/v1/healthz" >/dev/null

log "readyz"
curl -fsS "$BASE_URL/api/v1/readyz" >/dev/null

log "login route contains exported form"
curl -fsS "$BASE_URL/login" | grep -q 'nama@sekolah.id'

log "frontend typecheck"
(cd "$FRONTEND_DIR" && npm run typecheck)

log "playwright discovery"
(cd "$FRONTEND_DIR" && npx playwright test --list)

log "playwright login smoke"
(cd "$FRONTEND_DIR" && E2E_BASE_URL="$BASE_URL" npx playwright test login-smoke.spec.ts)

log "PASS"
