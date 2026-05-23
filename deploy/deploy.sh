#!/usr/bin/env bash
# deploy/deploy.sh — ship a change to rdpkhorur.
#
# Usage (local laptop):
#
#   bash deploy/deploy.sh
#
# Or, equivalently, via SSH from anywhere:
#
#   ssh rdpkhorur "cd /home/ubuntu/lms && bash deploy/deploy.sh --remote"
#
# Pattern mirrors fb-bot (locked decision #9).

set -euo pipefail

REMOTE_HOST="${REMOTE_HOST:-rdpkhorur}"
REMOTE_DIR="${REMOTE_DIR:-/home/ubuntu/lms}"
BRANCH="${BRANCH:-main}"

run_remote=false
for arg in "$@"; do
  case "$arg" in
    --remote) run_remote=true ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

build_and_restart() {
  cd "$REMOTE_DIR"
  echo "[deploy] git fetch + reset..."
  git fetch origin "$BRANCH"
  git reset --hard "origin/$BRANCH"

  echo "[deploy] frontend build..."
  ( cd frontend && npm install --silent && npm run build )

  echo "[deploy] backend build..."
  ( cd backend && go build -o bin/lms-api ./cmd/server )
  ( cd backend && go build -o bin/seed-admin ./cmd/seed-admin )
  ( cd backend && go build -o bin/reset-admin ./cmd/reset-admin )
  ( cd backend && go build -o bin/cleanup-dry-run ./cmd/cleanup-dry-run )

  echo "[deploy] migrate up..."
  if command -v migrate >/dev/null 2>&1; then
    migrate -path backend/migrations -database "${DATABASE_URL:?DATABASE_URL not set in env}" up
  else
    echo "[deploy] WARNING: 'migrate' CLI not installed; skipping migrate up." >&2
  fi

  echo "[deploy] systemctl restart lms-api..."
  sudo systemctl restart lms-api

  echo "[deploy] verifying readyz..."
  for i in 1 2 3 4 5 6 7 8 9 10; do
    if curl -fsS http://127.0.0.1:8200/api/v1/readyz >/dev/null; then
      echo "[deploy] readyz OK"
      exit 0
    fi
    sleep 2
  done
  echo "[deploy] readyz never came up; check journalctl -u lms-api" >&2
  exit 1
}

if [ "$run_remote" = true ]; then
  build_and_restart
  exit
fi

# Local laptop path — just shell out to SSH.
echo "[deploy] dispatching to $REMOTE_HOST:$REMOTE_DIR (branch=$BRANCH)"
ssh "$REMOTE_HOST" "cd $REMOTE_DIR && bash deploy/deploy.sh --remote"
