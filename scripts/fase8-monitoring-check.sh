#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-${E2E_BASE_URL:-http://127.0.0.1:8200}}"
SERVICE_NAME="${SERVICE_NAME:-lms-api}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-5}"

echo "[fase8-monitoring] base url: $BASE_URL"

echo "[fase8-monitoring] healthz"
curl -fsS --max-time "$TIMEOUT_SECONDS" "$BASE_URL/api/v1/healthz" >/dev/null

echo "[fase8-monitoring] readyz"
curl -fsS --max-time "$TIMEOUT_SECONDS" "$BASE_URL/api/v1/readyz" >/dev/null

if command -v systemctl >/dev/null 2>&1; then
  echo "[fase8-monitoring] systemd service: $SERVICE_NAME"
  systemctl is-active --quiet "$SERVICE_NAME"
  systemctl is-enabled --quiet "$SERVICE_NAME" || echo "[fase8-monitoring] WARN: $SERVICE_NAME is not enabled"
else
  echo "[fase8-monitoring] SKIP: systemctl not available"
fi

echo "[fase8-monitoring] PASS"
