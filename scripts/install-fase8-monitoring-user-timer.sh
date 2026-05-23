#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${BASE_URL:-${E2E_BASE_URL:-http://127.0.0.1:8200}}"
SERVICE_NAME="${SERVICE_NAME:-lms-api}"
INTERVAL="${MONITORING_INTERVAL:-5min}"
BOOT_DELAY="${MONITORING_BOOT_DELAY:-2min}"
RANDOM_DELAY="${MONITORING_RANDOM_DELAY:-30s}"
USER_SYSTEMD_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"

mkdir -p "$USER_SYSTEMD_DIR"

cat > "$USER_SYSTEMD_DIR/lms-fase8-monitoring.service" <<EOF_SERVICE
[Unit]
Description=LMS Fase 8 monitoring check (user timer)

[Service]
Type=oneshot
WorkingDirectory=$ROOT_DIR
Environment=BASE_URL=$BASE_URL
Environment=SERVICE_NAME=$SERVICE_NAME
ExecStart=/usr/bin/env bash $ROOT_DIR/scripts/fase8-monitoring-check.sh
EOF_SERVICE

cat > "$USER_SYSTEMD_DIR/lms-fase8-monitoring.timer" <<EOF_TIMER
[Unit]
Description=Run LMS Fase 8 monitoring check every $INTERVAL (user timer)

[Timer]
OnBootSec=$BOOT_DELAY
OnUnitActiveSec=$INTERVAL
RandomizedDelaySec=$RANDOM_DELAY
Persistent=true
Unit=lms-fase8-monitoring.service

[Install]
WantedBy=timers.target
EOF_TIMER

systemctl --user daemon-reload
systemctl --user enable --now lms-fase8-monitoring.timer
systemctl --user start lms-fase8-monitoring.service
systemctl --user status lms-fase8-monitoring.service --no-pager -l
systemctl --user list-timers lms-fase8-monitoring.timer --no-pager
