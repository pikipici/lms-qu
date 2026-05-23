# LMS Deploy Runbook

> Target: `rdpkhorur` (Ubuntu, Asia/Jakarta). Mirip pola fb-bot.
> Locked decisions referenced: #4, #9, #29, #35, #44, #53, #60.

## TL;DR — ship a change

```bash
# Local (Windows, C:\Users\pikip\Documents\program\lms)
git add -A
git commit -m "feat(...): ..."
git push origin main

# Server (rdpkhorur)
ssh rdpkhorur "cd /home/ubuntu/lms && bash deploy/deploy.sh --remote"
```

`deploy/deploy.sh --remote` melakukan:
1. `git fetch + reset --hard origin/main`
2. `npm install + npm run build` (frontend → `frontend/out`)
3. `go build` (server, seed-admin, reset-admin)
4. `migrate up` (kalau `migrate` CLI tersedia)
5. `sudo systemctl restart lms-api`
6. Polling `/api/v1/readyz` 10×2s; fail kalau gak pernah 200.

Verify dari laptop:
```bash
ssh -L 8200:127.0.0.1:8200 rdpkhorur
# buka http://localhost:8200
```

---

## First-time server setup

### 1. Base packages

```bash
sudo apt update
sudo apt install -y nodejs npm postgresql git build-essential curl
# Go 1.22+ — pakai tarball atau snap
sudo snap install go --classic
# golang-migrate
curl -L https://github.com/golang-migrate/migrate/releases/download/v4.17.1/migrate.linux-amd64.tar.gz | tar xz
sudo mv migrate /usr/local/bin/
```

### 2. PostgreSQL

```bash
sudo systemctl enable --now postgresql
sudo -u postgres psql <<'SQL'
CREATE USER lms WITH PASSWORD 'change-me-strong-password';
CREATE DATABASE lms OWNER lms;
\q
SQL
```

### 3. Clone & .env

```bash
sudo mkdir -p /home/ubuntu/lms
sudo chown ubuntu:ubuntu /home/ubuntu/lms
git clone git@github.com:<user>/lms.git /home/ubuntu/lms
cd /home/ubuntu/lms
cp .env.example .env
nano .env   # isi DATABASE_URL, JWT_SECRET_KEY (>=32 byte random), ENV=production, AUTOMIGRATE=false
chmod 600 .env
```

JWT secret generation:
```bash
openssl rand -hex 32   # output → JWT_SECRET_KEY
```

### 4. First build

```bash
cd /home/ubuntu/lms/backend
go mod download
go build -o bin/lms-api ./cmd/server
go build -o bin/seed-admin ./cmd/seed-admin
go build -o bin/reset-admin ./cmd/reset-admin

cd /home/ubuntu/lms/frontend
npm install
npm run build       # menghasilkan frontend/out/
```

### 5. Migrate

```bash
cd /home/ubuntu/lms
migrate -path backend/migrations -database "$(grep ^DATABASE_URL .env | cut -d= -f2-)" up
```

### 6. systemd unit

```bash
sudo cp deploy/systemd/lms-api.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now lms-api
sudo systemctl status lms-api --no-pager
```

`ExecStartPost` melakukan polling `/readyz` jadi unit baru active kalau service benar-benar siap (#44).

### 7. Seed admin pertama (#17, #53)

```bash
cd /home/ubuntu/lms
ADMIN_EMAIL=admin@sekolah.id \
ADMIN_PASSWORD='ganti-secepatnya-XYZ' \
ADMIN_NAME='Administrator' \
./backend/bin/seed-admin
```

> Fase 0 stub: hanya validasi config + DB. Insert user beneran ada di Fase 1.

### 8. Tunnel & test

```bash
# Dari laptop
ssh -L 8200:127.0.0.1:8200 rdpkhorur
# Buka http://localhost:8200
curl -s http://127.0.0.1:8200/api/v1/healthz
curl -s http://127.0.0.1:8200/api/v1/readyz
```

---

## Frontend env strategy (#60)

`NEXT_PUBLIC_API_BASE` di-bake **at build time**. Static export tidak baca env runtime.

| Env | Value | File |
|---|---|---|
| Production | `/api/v1` (same-origin) | `.env` di root, atau export sebelum `npm run build` |
| Dev (local) | `http://localhost:8200/api/v1` | `frontend/.env.development.local` |

Ubah base URL → wajib rebuild FE.

---

## Rollback

```bash
ssh rdpkhorur
cd /home/ubuntu/lms
git log --oneline -n 10
git reset --hard <sha>
bash deploy/deploy.sh --remote
```

Migration rollback:
```bash
migrate -path backend/migrations -database "$DATABASE_URL" down 1
```

---

## Backup & Restore Drill (#51)

Runtime facts:
- PostgreSQL listens on port `5435`, DB `lms`, user `lms`.
- Source `.env` before running commands so `DATABASE_URL` includes the password and `sslmode=disable`.
- Backups are written outside the repo to `/home/ubuntu/lms-backups`.

Install cron:
```bash
ssh rdpkhorur
sudo install -d -o ubuntu -g ubuntu -m 750 /home/ubuntu/lms-backups/daily /home/ubuntu/lms-backups/monthly
sudo tee /etc/cron.d/lms-backup >/dev/null <<'CRON'
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Daily rolling backups, retain 30 days.
0 2 * * * ubuntu cd /home/ubuntu/lms && set -a; . ./.env; set +a; pg_dump "$DATABASE_URL" | gzip -9 > /home/ubuntu/lms-backups/daily/lms_$(date +\%F).sql.gz
30 2 * * * ubuntu find /home/ubuntu/lms-backups/daily -type f -name 'lms_*.sql.gz' -mtime +30 -delete

# Monthly archive, retain 1 year.
0 3 1 * * ubuntu cd /home/ubuntu/lms && set -a; . ./.env; set +a; pg_dump "$DATABASE_URL" | gzip -9 > /home/ubuntu/lms-backups/monthly/lms_$(date +\%Y-\%m).sql.gz
30 3 1 * * ubuntu find /home/ubuntu/lms-backups/monthly -type f -name 'lms_*.sql.gz' -mtime +366 -delete
CRON
sudo chmod 644 /etc/cron.d/lms-backup
```

Manual backup smoke:
```bash
ssh rdpkhorur
cd /home/ubuntu/lms
set -a; . ./.env; set +a
mkdir -p /home/ubuntu/lms-backups/daily
pg_dump "$DATABASE_URL" | gzip -9 > /home/ubuntu/lms-backups/daily/lms_manual_$(date +%F_%H%M%S).sql.gz
ls -lh /home/ubuntu/lms-backups/daily/lms_manual_*.sql.gz | tail -1
```

Restore drill to disposable DB only — never restore into live `lms`:
```bash
ssh rdpkhorur
cd /home/ubuntu/lms
set -a; . ./.env; set +a
BACKUP=/home/ubuntu/lms-backups/daily/<backup-file>.sql.gz
DRILL_DB=lms_restore_drill_$(date +%Y%m%d_%H%M%S)

createdb -h localhost -p 5435 -U lms "$DRILL_DB"
gunzip -c "$BACKUP" | psql -h localhost -p 5435 -U lms -d "$DRILL_DB" -v ON_ERROR_STOP=1
psql -h localhost -p 5435 -U lms -d "$DRILL_DB" -c "SELECT COUNT(*) AS users_count FROM users;"
psql -h localhost -p 5435 -U lms -d "$DRILL_DB" -c "SELECT COUNT(*) AS kelas_count FROM kelas;"
dropdb -h localhost -p 5435 -U lms "$DRILL_DB"
```

Restore drill pass criteria:
- `gunzip -t <backup>` exits 0.
- `psql -v ON_ERROR_STOP=1` restore exits 0.
- Sanity queries for critical tables (`users`, `kelas`) return counts without errors.
- Disposable DB is dropped after verification.

Log each drill in `dogfood-output/fase8/backup-restore-drill.md` with backup filename, restore DB name, query counts, and cleanup result. Test restore in staging/disposable DB minimal 1x before go-live (#51 + #11 risk).

---

## Logs

```bash
journalctl -u lms-api -f --no-pager -n 200
journalctl -u lms-api --since '10 min ago' | grep request_id=
```

Setiap response punya header `X-Request-ID`. User report bug → minta request_id, lalu grep di journal.

---

## Emergency: admin lock-out (#53)

Kalau admin satu-satunya kena `locked` atau lupa password:

```bash
ssh rdpkhorur
cd /home/ubuntu/lms
./backend/bin/reset-admin --email admin@sekolah.id
# Prompt password baru via TTY.
# Atau pass --password explicit:
./backend/bin/reset-admin --email admin@sekolah.id --password 'temp-NEW-9876'
```

Trust boundary: SSH access ke server. Tidak ada self-service recovery.

---

## Cleanup Tasks (Fase 8)

Conservative rollout rule: cleanup starts as **dry-run only**. Do not hard-delete DB rows or R2 objects until dry-run counts look sane for several days and a fresh backup restore drill has passed.

Already active in app process:
- ImportJob preview expiry hourly: `status=preview AND expires_at < now()` becomes `expired`; raw CSV R2 object is best-effort deleted.
- ImportJob credentials eviction hourly: `status=completed AND completed_at + 1h < now()` nulls credentials CSV handle; R2 credentials file is best-effort deleted.

Planned dry-run scopes:

| Scope | Retention | Dry-run output | Hard-delete gate |
|---|---:|---|---|
| `login_attempts` | 30 days | count rows older than cutoff grouped by day/status | after 3 clean dry-runs |
| `refresh_tokens` | expired/revoked + 7 days | count expired/revoked rows and oldest `expires_at` | after auth smoke still passes |
| `hasil_soal_bab` soft-deleted rows | 1 year | count rows + linked jawaban/assignment counts | after nilai/rekap smoke still passes |
| `hasil_ujian` soft-deleted rows | 1 year | count rows + linked jawaban/assignment counts | after nilai/rekap smoke still passes |
| `submission` attachments in archived kelas | archived + 1 year | count object keys by prefix and total bytes if available | after R2 orphan report verified |
| R2 orphan objects | no DB reference | list candidate keys only, no delete | manual review of sample keys |

Manual dry-run SQL probes:
```bash
ssh rdpkhorur
cd /home/ubuntu/lms
set -a; . ./.env; set +a
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
SELECT 'login_attempts_old' AS scope, COUNT(*) AS candidate_count
FROM login_attempts
WHERE created_at < now() - interval '30 days';

SELECT 'refresh_tokens_expired_revoked' AS scope, COUNT(*) AS candidate_count
FROM refresh_tokens
WHERE (expires_at < now() OR revoked_at IS NOT NULL)
  AND COALESCE(revoked_at, expires_at) < now() - interval '7 days';

SELECT 'hasil_soal_bab_deleted_old' AS scope, COUNT(*) AS candidate_count
FROM hasil_soal_bab
WHERE deleted_at IS NOT NULL
  AND deleted_at < now() - interval '1 year';

SELECT 'hasil_ujian_deleted_old' AS scope, COUNT(*) AS candidate_count
FROM hasil_ujian
WHERE deleted_at IS NOT NULL
  AND deleted_at < now() - interval '1 year';
SQL
```

Dry-run pass criteria:
- Counts are logged to `dogfood-output/fase8/cleanup-dry-run.md` with timestamp and commit SHA.
- Candidate counts are explainable by retention policy; unexpected spikes block hard-delete.
- A backup exists from the same day and disposable restore drill has passed.
- App smoke after dry-run is still green.

Implementation recommendation:
- Add a separate `internal/cleanup` package with `RunOnce(ctx, opts)` returning structured counts.
- Start with CLI/admin-trigger dry-run (`--dry-run=true`) before background goroutine.
- Require explicit env flag for destructive mode, e.g. `CLEANUP_DESTRUCTIVE=true`; default false in production.
- For R2 cleanup, list candidates and sample first; delete only keys with exact DB negative lookup and category prefix allowlist (`tugas/`, `soalbab/`, `soal-bank/`, `materi/`, `submission/`, `import/`).
