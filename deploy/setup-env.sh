#!/bin/bash
# One-shot LMS .env bootstrap.
# - Rotate DB password to fresh 32-char alnum
# - Generate JWT_SECRET_KEY via openssl rand
# - Write /home/ubuntu/lms/.env with mode 0600
# Idempotent guard: bail if .env already exists.
set -eu

ENV_PATH=/home/ubuntu/lms/.env
LOG=/tmp/lms-setup-env.log
exec > >(tee -a "$LOG") 2>&1
echo "=== run $(date -Iseconds) ==="

if [ -f "$ENV_PATH" ]; then
  echo "ERROR: $ENV_PATH already exists. Abort to avoid overwrite." >&2
  exit 1
fi

echo "[step] gen DB password"
NEW_DB_PASS=$(LC_ALL=C tr -dc 'a-zA-Z0-9' </dev/urandom 2>/dev/null | head -c 32 || true)
if [ -z "${NEW_DB_PASS:-}" ] || [ "${#NEW_DB_PASS}" -ne 32 ]; then
  echo "ERROR: failed to generate password" >&2
  exit 1
fi
echo "[ok] db password generated, len=${#NEW_DB_PASS}"

echo "[step] gen JWT secret"
JWT_SECRET=$(openssl rand -hex 32)
echo "[ok] jwt secret generated, len=${#JWT_SECRET}"

echo "[step] rotate DB password"
sudo -u postgres psql -p 5435 -d postgres -v ON_ERROR_STOP=1 >/dev/null <<SQL
ALTER USER lms WITH PASSWORD '${NEW_DB_PASS}';
SQL
echo "[ok] DB password rotated"

echo "[step] verify auth"
if ! PGPASSWORD="$NEW_DB_PASS" psql -h localhost -p 5435 -U lms -d lms -c 'SELECT 1' >/dev/null 2>&1; then
  echo "ERROR: rotated password failed to authenticate" >&2
  exit 1
fi
echo "[ok] auth verified"

echo "[step] write .env"
umask 077
cat > "$ENV_PATH" <<ENVFILE
ENV=production
PORT=8200
TIMEZONE=Asia/Jakarta
LOG_LEVEL=info

DATABASE_URL=postgres://lms:${NEW_DB_PASS}@localhost:5435/lms?sslmode=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME_MIN=30

AUTOMIGRATE=false

JWT_SECRET_KEY=${JWT_SECRET}
JWT_ACCESS_TTL_MIN=15
JWT_REFRESH_TTL_DAY=7
BCRYPT_COST=12

STORAGE_DIR=./storage/uploads
MAX_TUGAS_FILE_MB=20
MAX_GAMBAR_SOAL_MB=5

RATE_LIMIT_GLOBAL_PER_MIN=120
RATE_LIMIT_LOGIN_PER_15MIN=5
RATE_LIMIT_REFRESH_PER_MIN=10
RATE_LIMIT_KELAS_JOIN_PER_MIN=10
RATE_LIMIT_UPLOAD_PER_MIN=30

CORS_ALLOWED_ORIGINS=

FRONTEND_DIR=./frontend/out
ENVFILE

chmod 600 "$ENV_PATH"
chown ubuntu:ubuntu "$ENV_PATH"

echo "[ok] .env written"
ls -la "$ENV_PATH"
echo "[done]"
