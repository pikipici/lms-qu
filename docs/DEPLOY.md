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

## Backup (#51)

```cron
# /etc/cron.d/lms-backup
0 2 * * * ubuntu pg_dump -U lms lms | gzip > /home/ubuntu/backups/lms_$(date +\%F).sql.gz
0 3 * * 0 ubuntu find /home/ubuntu/backups -name 'lms_*.sql.gz' -mtime +30 -delete
```

Test restore di staging minimal 1× sebelum go-live (#51 + #11 risk).

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
