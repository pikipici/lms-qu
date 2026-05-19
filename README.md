# LMS

> Learning Management System multi-guru, admin-controlled, berbasis Bab.
> Stack: Go Fiber + GORM + PostgreSQL  /  Next.js 14 static export + shadcn/ui.

Status: **Fase 0 — Setup** (in progress).
Roadmap & decisions: [.kiro/steering/lms-roadmap.md](.kiro/steering/lms-roadmap.md) (v0.7.2, 60 locked decisions).

---

## Fitur (target MVP)

- **Admin**: manage user (CRUD), reset password, bulk import siswa via CSV, audit log, multi-admin promote dengan re-auth.
- **Guru**: kelas + bab (materi, soal latihan, ulangan bab, tugas), ulangan harian lintas bab, penilaian transparan, duplicate kelas/bab, activity feed, pending counters.
- **Siswa**: lihat materi per bab, kerjain latihan retry-unlimited, ikut ulangan bab (1×) + ulangan harian, submit tugas, lihat nilai breakdown per bab.
- **Anti-cheat**: timer server-side, shuffle soal/opsi, random pool, log tab-switch.
- **Recovery**: resume ulangan kalau crash/disconnect.
- **Remedial**: guru reset attempt (soft-delete + audit + soal_diff).
- **Force change password** pas login pertama.

Detail lengkap di [`lms-roadmap.md`](.kiro/steering/lms-roadmap.md) section 4-6.

---

## Quickstart

> Local laptop = pure coding + git only. Build, test, run di `rdpkhorur` via SSH.

```bash
# 1. Clone (server only)
ssh rdpkhorur
git clone git@github.com:<user>/lms.git /home/ubuntu/lms
cd /home/ubuntu/lms

# 2. Setup .env
cp .env.example .env
nano .env   # isi DATABASE_URL, JWT_SECRET_KEY (>=32 byte), ENV=production

# 3. Build + migrate + start
bash deploy/deploy.sh --remote

# 4. Verify (dari laptop)
ssh -L 8200:127.0.0.1:8200 rdpkhorur
# browser: http://localhost:8200
curl http://127.0.0.1:8200/api/v1/healthz
curl http://127.0.0.1:8200/api/v1/readyz
```

Detail: [`docs/DEPLOY.md`](docs/DEPLOY.md).

---

## Project structure

```
lms/
├── backend/                 # Go + Fiber + GORM
│   ├── cmd/
│   │   ├── server/          # API server
│   │   ├── seed-admin/      # First admin bootstrap (#17)
│   │   └── reset-admin/     # Emergency reset (#53)
│   ├── internal/
│   │   ├── config/          # env loader + validation
│   │   ├── db/              # GORM open + ping
│   │   ├── middleware/      # request-id, ratelimit, recover, logger
│   │   ├── health/          # /healthz, /readyz
│   │   └── storage/         # upload path convention (#58)
│   ├── migrations/          # golang-migrate up/down (#35)
│   └── go.mod
├── frontend/                # Next.js 14 (output: 'export')
│   ├── app/                 # App Router pages
│   ├── lib/                 # api wrapper, auth store, utils
│   ├── package.json
│   └── next.config.js
├── deploy/
│   ├── deploy.sh            # ship-a-change script
│   └── systemd/lms-api.service
├── docs/
│   ├── DEPLOY.md            # full runbook
│   └── ARCHITECTURE.md      # diagrams + flows
├── .kiro/steering/
│   └── lms-roadmap.md       # v0.7.2 living plan
├── storage/uploads/         # gitignored (runtime)
├── .env.example
├── .gitignore
├── LOCAL_AI_CONTEXT.md      # quick context for AI sessions
└── README.md
```

---

## Conventions

- **Timezone**: server lock `Asia/Jakarta`, frontend tampil WIB explicit (#29).
- **Auth**: JWT access 15m stateless + refresh 7d stateful (rotation, reuse detection) (#42).
- **Rate limit**: login 5/15m per (IP+email), global 120/min, refresh 10/min, kelas/join 10/min (#47).
- **Optimistic concurrency**: `Version` field di Bab/Kelas/SoalBab/UlanganBabSetting/Soal/Ujian (#56).
- **Storage**: `./storage/uploads/<kategori>/<uuid>.<ext>` (#58).
- **Health**: `/api/v1/healthz` (liveness) + `/api/v1/readyz` (DB + storage) (#44).
- **Request ID**: `X-Request-ID` header propagate ke slog di semua request (#49).

---

## Phase progress

```
[x] Fase 0  Setup (init repo, healthz/readyz, request-id, ratelimit, deploy skeleton)
[ ] Fase 1  Auth & Admin Panel
[ ] Fase 2  Kelas + Bulk Import
[ ] Fase 3  Bab + Materi + Pengumuman
[ ] Fase 4  Tugas + Late + Resubmit
[ ] Fase 5  Soal Bab (Latihan + Ulangan + Resume + Remedial + Random + Review)
[ ] Fase 6  Ulangan Harian (mirror Fase 5)
[ ] Fase 7  Rekap Nilai + Activity Feed + Pending Counters
[ ] Fase 8  Polish + E2E + Cleanup tasks + Coverage gate
```

Notifikasi (v0.8) dibedah sebelum Fase 4.

---

## License

Proprietary — sekolah-internal use.
