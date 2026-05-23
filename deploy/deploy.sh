#!/usr/bin/env bash
# deploy/deploy.sh — ship a change to rdpkhorur (HARDENED v0.2)
#
# Usage (local laptop):
#
#   bash deploy/deploy.sh
#
# Or via SSH from anywhere:
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

# Rollback helper
rollback() {
  echo "[deploy] ROLLBACK: restoring old binary..."
  sudo systemctl stop lms-api
  if [ -f "$REMOTE_DIR/backend/bin/lms-api.bak" ]; then
    cp "$REMOTE_DIR/backend/bin/lms-api.bak" "$REMOTE_DIR/backend/bin/lms-api"
    sudo systemctl start lms-api
  else
    echo "[deploy] ERROR: no backup found for rollback!" >&2
    exit 1
  fi
  echo "[deploy] Rollback complete. Check service status."
  exit 0
}

# Pre-flight checks
pre_flight() {
  cd "$REMOTE_DIR"
  
  echo "[deploy] === PRE-FLIGHT CHECKS ==="
  
  # 1. Source .env
  echo "[deploy] sourcing .env..."
  if [ ! -f "$REMOTE_DIR/.env" ]; then
    echo "[deploy] ERROR: .env not found!" >&2
    exit 1
  fi
  set -a
  source "$REMOTE_DIR/.env"
  set +a
  
  # 2. Check required env vars
  if [ -z "${DATABASE_URL:-}" ] || [ -z "${R2_BUCKET:-}" ]; then
    echo "[deploy] ERROR: required env vars missing (DATABASE_URL, R2_BUCKET)!" >&2
    exit 1
  fi

  # Derive Cloudflare R2/AWS CLI env from the existing app env without writing secrets.
  if [ -z "${R2_ENDPOINT:-}" ]; then
    if [ -z "${R2_ACCOUNT_ID:-}" ]; then
      echo "[deploy] ERROR: R2_ENDPOINT or R2_ACCOUNT_ID required for R2 pre-flight!" >&2
      exit 1
    fi
    export R2_ENDPOINT="https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com"
  fi
  export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-${R2_ACCESS_KEY_ID:-}}"
  export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-${R2_SECRET_ACCESS_KEY:-}}"
  export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-auto}"
  if [ -z "$AWS_ACCESS_KEY_ID" ] || [ -z "$AWS_SECRET_ACCESS_KEY" ]; then
    echo "[deploy] ERROR: R2 credentials missing for AWS CLI pre-flight!" >&2
    exit 1
  fi
  
  # 3. DB reachable (2s timeout)
  echo "[deploy] checking DB connectivity..."
  if ! timeout 2 bash -c "echo '' | psql $DATABASE_URL -c 'SELECT 1' >/dev/null 2>&1"; then
    echo "[deploy] ERROR: DB unreachable! Check DATABASE_URL and postgres service." >&2
    exit 1
  fi
  
  # 4. R2 reachable
  echo "[deploy] checking R2 connectivity..."
  if ! aws --endpoint-url="$R2_ENDPOINT" s3api head-bucket --bucket "$R2_BUCKET" 2>/dev/null; then
    echo "[deploy] ERROR: R2 unreachable! Check R2 credentials and endpoint." >&2
    exit 1
  fi
  
  # 5. Port 8200 check
  echo "[deploy] checking port 8200..."
  if netstat -tuln 2>/dev/null | grep -q ':8200 '; then
    echo "[deploy] WARNING: port 8200 already in use (likely current lms-api)"
  fi
  
  # 6. Disk space check (500MB minimum)
  echo "[deploy] checking disk space..."
  local avail
  avail=$(df -BM "$REMOTE_DIR" | tail -1 | awk '{print $4}' | sed 's/M//')
  if [ "$avail" -lt 500 ]; then
    echo "[deploy] ERROR: insufficient disk space (${avail}MB < 500MB required)!" >&2
    exit 1
  fi
  
  echo "[deploy] === ALL PRE-FLIGHT CHECKS PASSED ==="
}

# Build phase
build_phase() {
  echo "[deploy] === BUILD PHASE ==="
  
  # 1. Frontend build
  echo "[deploy] frontend build..."
  ( cd "$REMOTE_DIR/frontend" && npm ci --silent && npm run build ) || {
    echo "[deploy] ERROR: frontend build failed!" >&2
    exit 1
  }
  
  # 2. Backend build (to /tmp first for verification)
  echo "[deploy] backend build..."
  local tmp_backend="/tmp/lms-api-build-$$"
  mkdir -p "$tmp_backend"
  
  ( cd "$REMOTE_DIR/backend" ) || exit 1
  go build -o "$tmp_backend/lms-api" ./cmd/server || {
    echo "[deploy] ERROR: lms-api build failed!" >&2
    rm -rf "$tmp_backend"
    exit 1
  }
  go build -o "$tmp_backend/seed-admin" ./cmd/seed-admin || true
  go build -o "$tmp_backend/reset-admin" ./cmd/reset-admin || true
  go build -o "$tmp_backend/cleanup-dry-run" ./cmd/cleanup-dry-run || true
  
  # 3. Verify binary executable
  if ! test -x "$tmp_backend/lms-api"; then
    echo "[deploy] ERROR: built binary not executable!" >&2
    rm -rf "$tmp_backend"
    exit 1
  fi
  
  echo "[deploy] === BUILD SUCCESS ==="
}

# Deploy phase
deploy_phase() {
  echo "[deploy] === DEPLOY PHASE ==="
  
  local old_binary="$REMOTE_DIR/backend/bin/lms-api"
  local new_binary="$REMOTE_DIR/backend/bin/lms-api.new"
  local backup_file="$REMOTE_DIR/backend/bin/lms-api.bak.$(date +%s)"
  
  # 1. Backup old binary
  if [ -f "$old_binary" ]; then
    cp "$old_binary" "$backup_file"
    echo "[deploy] backed up old binary to $backup_file"
  else
    echo "[deploy] WARNING: no existing binary found, skipping backup"
  fi
  
  # 2. Copy new binary
  cp "$tmp_backend/lms-api" "$new_binary"
  chmod +x "$new_binary"
  echo "[deploy] copied new binary to $new_binary"
  
  # 3. Copy frontend static
  echo "[deploy] copying frontend static..."
  mkdir -p "$REMOTE_DIR/public"
  cp -r "$REMOTE_DIR/frontend/out/*" "$REMOTE_DIR/public/" || true
  
  # 4. Migrate up (idempotent)
  echo "[deploy] migrate up..."
  if command -v migrate >/dev/null 2>&1; then
    migrate -path "$REMOTE_DIR/backend/migrations" -database "$DATABASE_URL" up
  else
    echo "[deploy] WARNING: 'migrate' CLI not installed; skipping migrate up." >&2
  fi
  
  # 5. Swap binary (atomic-ish)
  mv "$new_binary" "$old_binary"
  echo "[deploy] swapped binary to $old_binary"
  
  # 6. Restart service
  echo "[deploy] systemctl restart lms-api..."
  sudo systemctl restart lms-api
  
  # 7. Cleanup temp
  rm -rf "$tmp_backend"
  
  echo "[deploy] === DEPLOY COMPLETE ==="
}

# Verify phase
verify_phase() {
  echo "[deploy] === VERIFICATION PHASE ==="
  
  local max_attempts=10
  local attempt=1
  
  while [ $attempt -le $max_attempts ]; do
    if curl -fsS http://127.0.0.1:8200/api/v1/readyz >/dev/null 2>&1; then
      echo "[deploy] readyz OK on attempt $attempt"
      echo "[deploy] === DEPLOY SUCCESS ==="
      return 0
    fi
    
    echo "[deploy] readyz attempt $attempt/$max_attempts..."
    sleep 2
    attempt=$((attempt + 1))
  done
  
  echo "[deploy] ERROR: readyz never came up after $max_attempts attempts!" >&2
  echo "[deploy] Check: journalctl -u lms-api --no-pager" >&2
  
  # Attempt rollback
  echo "[deploy] Attempting rollback..."
  rollback
  
  return 1
}

# Main flow
main() {
  cd "$REMOTE_DIR"
  
  pre_flight
  build_phase
  deploy_phase
  verify_phase
}

# Main entry point
if [ "$run_remote" = true ]; then
  main
else
  echo "[deploy] dispatching to $REMOTE_HOST:$REMOTE_DIR (branch=$BRANCH)"
  ssh "$REMOTE_HOST" "cd $REMOTE_DIR && bash deploy/deploy.sh --remote"
fi