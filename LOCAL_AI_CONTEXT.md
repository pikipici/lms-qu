# LMS — Local AI Context

> Quick context buat AI sessions. Update tiap kali ada keputusan / state baru.

## TL;DR
- LMS multi-guru, admin-controlled, berbasis Bab.
- Stack: Go Fiber + GORM + PostgreSQL / Next.js 14 static export + shadcn/ui.
- Local: pure coding + git only (Windows). Build/test/run di rdpkhorur via SSH.

## Paths
- Local repo: `C:\Users\pikip\Documents\program\lms`
- Server target: `rdpkhorur:/home/ubuntu/lms`
- Server bare repo (git remote `server`): `rdpkhorur:/home/ubuntu/git-repos/lms.git`
- SSH alias: `rdpkhorur` (assumed configured)
- Browser preview tunnel: `ssh -L 8200:127.0.0.1:8200 rdpkhorur` → http://localhost:8200

## Server runtime facts
- PostgreSQL: **port 5435** (bukan default 5432). DB `lms`, user `lms`.
- DATABASE_URL format: `postgres://lms:<password>@localhost:5435/lms?sslmode=disable`
- `migrate` CLI v4.17.1 di `/usr/local/bin/migrate`.
- Go 1.24, Node 20.
- Server dir setelah clone bare: `/home/ubuntu/lms` (tracking remote `server` → bare repo).
- Push flow: `git push server main` dari local → ssh ke server → `cd /home/ubuntu/lms && git pull` → build → restart systemd.

## Working agreements
- Local = no runtime deps installed. Tidak ada `go run`, `npm install`, `psql` di local.
- Push code lokal → ssh ke rdpkhorur → `git fetch && reset --hard` → build → restart systemd.
- Verifikasi build/test selalu di rdpkhorur. Hasil dilaporkan balik ke chat.
- Roadmap & locked decisions: `.kiro/steering/lms-roadmap.md` (v0.7.2).
- 60 locked decisions, 10 open decisions, ~7 minggu estimasi.

## Phase tracker
- [x] Fase 0 — Setup (DONE, smoke test passed, migrate 000001_init applied)
- [ ] Fase 1 — Auth & Admin Panel
- [ ] Fase 2 — Kelas, Enrollment, Bulk Import
- [ ] Fase 3 — Bab & Materi + Pengumuman
- [ ] Fase 4 — Tugas
- [ ] Fase 5 — Soal Bab
- [ ] Fase 6 — Ulangan Harian
- [ ] Fase 7 — Rekap Nilai + Activity Feed
- [ ] Fase 8 — Polish + E2E

## Critical conventions
- Timezone: server lock `Asia/Jakarta`, FE tampil WIB explicit.
- Storage path: `./storage/uploads/<kategori>/<uuid>.<ext>` (kategori = tugas|soal|materi|submission|import).
- Auth: JWT access 15m stateless + refresh 7d stateful (RefreshToken table, rotation, reuse detection).
- Rate limit: `/auth/login` 5/15m per (IP+email), global 120/min per IP, refresh 10/min, kelas/join 10/min, upload 30/min.
- Optimistic concurrency: `Version` field di Bab/Kelas/SoalBab/UlanganBabSetting/Soal/Ujian.
- Submit transition: `SELECT FOR UPDATE` + cek status di transaction, idempotent.
- Health: `/api/v1/healthz` (liveness, no DB), `/api/v1/readyz` (DB + storage check).
- Request ID: middleware bikin `X-Request-ID` di semua request, propagate ke slog.

## Deploy
- Single Go binary serve API + static FE di port 8200.
- systemd unit: `lms-api.service` (User=ubuntu, EnvironmentFile=.env, Restart=always).
- Tidak ada Nginx (mirip fb-bot).
- Backup: pg_dump cron daily, retain 30 hari rolling + monthly archive 1 tahun.
- Cleanup daily cron: orphan files, ImportJob expired, LoginAttempt >30d, RefreshToken expired >7d, Submission file kelas archived +1y.

## First admin bootstrap
1. ssh rdpkhorur
2. cd /home/ubuntu/lms/backend
3. `ADMIN_EMAIL=admin@sekolah.id ADMIN_PASSWORD='ganti-cepat-123' ./bin/seed-admin`
4. login → /me/security → ganti password.

## Emergency
- Admin lupa password & gak bisa reset: `./bin/reset-admin --email <email> --password <new>` (akses fisik server = trust boundary).

## Open decisions tersisa (perlu decision saat mendekati implementasi)
1. Notifikasi (v0.8 setelah Fase 0-3)
2. Pengumuman dismiss state (Fase 3)
3. Pending counter polling vs SSE (v0.8)
4. Bab unpublish dengan hasil — label vs hide (Fase 7)
5. JWT storage strategy: localStorage vs httpOnly cookie (v0.8 audit)
6. Self change-password — revoke ALL atau revoke OTHERS (Fase 1)
7. AuditLog partitioning (v1)
8. Share bank soal antar guru (v1)
9. Email notification (v0.9 atau v1)
10. AuditLog backfill target_kelas_id (Fase 7, default skip)
