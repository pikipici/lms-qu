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

## Cleanup tasks (Fase 8)

Daily cron yang akan ditambah saat Fase 8 launch:
- Orphan files `storage/uploads/*`
- ImportJob status=preview & ExpiresAt<now (cleanup hourly, bukan daily)
- LoginAttempt >30 hari
- RefreshToken expired & revoked >7 hari
- HasilSoalBab/HasilUjian deleted_at >1 tahun
- Submission file kelas archived + 1 tahun

Belum aktif sampai Fase 8.
