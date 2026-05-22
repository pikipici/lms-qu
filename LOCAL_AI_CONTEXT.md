# LMS — Local AI Context

> Quick context buat AI sessions. Update tiap kali ada keputusan / state baru.

## TL;DR
- LMS multi-guru, admin-controlled, berbasis Bab.
- Stack: Go Fiber + GORM + PostgreSQL / Next.js 14 static export + shadcn/ui.
- Local: pure coding + git only (Windows). Build/test/run di rdpkhorur via SSH.

## Paths
- Local repo: `C:\Users\pikip\Documents\program\lms`
- GitHub: `git@github.com:pikipici/lms-qu.git` (remote `origin`, primary truth)
- Server target: `rdpkhorur:/home/ubuntu/lms`
- Server bare repo (git remote `server`): `rdpkhorur:/home/ubuntu/git-repos/lms.git`
- SSH alias: `rdpkhorur` (assumed configured)
- Browser preview tunnel: `ssh -L 8200:127.0.0.1:8200 rdpkhorur` → http://localhost:8200
- Local shell aliases (Git Bash `~/.bashrc` + PowerShell `profile.ps1`):
  - `lmstun` — open background SSH tunnel `localhost:8200 → rdpkhorur:8200` (FE+API both served by lms-api binary)
  - `lmstun-fg` — foreground tunnel (Ctrl+C kills)
  - `lmstun-open` — open tunnel + auto-launch browser
  - `lmstun-status` — list active tunnel ssh.exe (PowerShell-backed even in bash)
  - `lmstun-kill` — kill all 8200 tunnel processes (PowerShell-backed)
  - `lms-ssh` — `ssh rdpkhorur` shortcut (bash); PowerShell uses `lms-shell rdpkhorur`

## Server runtime facts
- PostgreSQL: **port 5435** (bukan default 5432). DB `lms`, user `lms`.
- DATABASE_URL format: `postgres://lms:<password>@localhost:5435/lms?sslmode=disable`
- `migrate` CLI v4.17.1 di `/usr/local/bin/migrate`.
- Go 1.24, Node 20.
- Server dir setelah clone bare: `/home/ubuntu/lms` (tracking remote `server` → bare repo).
- Push flow:
  - `git push origin main` — push ke GitHub (truth)
  - `git push server main` — trigger deploy mirror ke bare repo
  - di server: `cd /home/ubuntu/lms && git pull origin main && cd backend && go build -o bin/lms-api ./cmd/server && sudo systemctl restart lms-api`
  - **Penting**: `lms-api` systemd ngeload bin dari `/home/ubuntu/lms/backend/bin/lms-api`. Build ke path lain (`/tmp/...`) gak diambil; restart cuma re-launch bin lama.

## Working agreements
- Local = no runtime deps installed. Tidak ada `go run`, `npm install`, `psql` di local.
- Push code lokal → ssh ke rdpkhorur → `git fetch && reset --hard` → build → restart systemd.
- Verifikasi build/test selalu di rdpkhorur. Hasil dilaporkan balik ke chat.
- Roadmap & locked decisions: `.kiro/steering/lms-roadmap.md` (v0.11.8 — Fase 4 ✅ CLOSED 14/14, **Fase 5 ✅ CLOSED 15/15 + UX/QA pass `22d2095`**, **Fase 6 plan ✅ DECOMPOSED 15 task locked #83-#88 + 6.A.1 ✅ DONE `3371e30` + 6.B 3/3 ✅ CLOSED `f50e7f2`+`76de898`+`ceaf86b` + 6.C 2/2 ✅ CLOSED `ede3194` + 6.D 3/4 ✅ DONE `7d465bf`+`d2ecef9`+`205be54`**).
- 88 locked decisions (v0.10.0 add #76-#82 Fase 5; v0.11.0 add #83 sub-fase split 6.A-6.G, #84 Bank Soal scope per-guru pribadi, #85 Ujian source mode manual/random + SourceConfigJSON, #86 Random pool seed Ujian deterministic, #87 Timer cron Ujian reuse goroutine SoalBab, #88 Coverage gate Fase 6 70%), 10 open decisions, ~7 hari estimasi Fase 6.

## Phase tracker
- [x] Fase 0 — Setup (DONE, smoke test passed, migrate 000001_init applied)
- [x] Fase 1 — Auth & Admin Panel (DONE: 1.A-1.H + 1.I, backend admin domain CLOSED, FE auth/admin shell/pengguna/audit-log/login-attempts shipped)
- [x] Fase 2 — Kelas, Enrollment, Bulk Import (DONE 20/20: 2.A.1, 2.A.2, 2.B FULL, 2.C FULL, 2.D FULL 6/6, 2.E FULL FE Admin Import 3/3 DONE 2026-05-21 commit `0f3772e`)
- [x] Fase 3 — Bab & Materi + Pengumuman (✅ DONE 17/17 = 100% complete; 3.A backend 4/4 DONE, 3.B FE Guru 2/2 DONE, 3.C Materi BE 4/4 CLOSED commit `caad20a`, 3.D Materi FE 2/2 CLOSED commits `eeca652` + `d08df3f`, 3.E Bab Siswa 2/2 CLOSED commits `c0d795a` + `3a69ddb`, 3.F Pengumuman 3/3 CLOSED: 3.F.1 BE commit `cf8c5bc` migration 000007 + CRUD endpoints + 18/18 tests, 3.F.2 + 3.F.3 FE combined commit `1ab48f7` — guru CRUD list+filter+compose+edit+archive+delete + siswa read-only kelas-wide + bab-scoped, lint cleanup `6d3cc6f`)
- [x] Fase 4 — Tugas (✅ DONE 14/14 = 100% complete; closed in this session: 4.A BE 4/4 ✅ migration `b6a2cf9` + CRUD `dc7d237` + attachment `55fb86a` + duplicate `3600188` (R2 CopyObject mirror bab pattern); 4.B FE Guru 2/2 ✅ `c4acf54`; 4.C BE 4/4 ✅ migration 000009 + repo `55296be` + submit/get/list/grade `6200d16` smoke E2E pass; 4.D FE Siswa 2/2 ✅ `9f8e9d0`+`fe50c66` SubmissionPanel + tugas detail page + `5d160b6`+`9d5eda2` ListMine endpoint + `6f49e14` /siswa/tugas riwayat lintas kelas page; 4.E FE Guru Review 2/2 ✅ `0775ead` SubmissionReviewList + GradeSubmissionDialog + `a4f14a4` pending-counts BE + `34aff41` sidebar badge + dashboard card. Locked #70-#75)
- [x] Fase 5 — Soal Bab ✅ CLOSED 2026-05-22 (15/15 = 100% locked #76-#82 — 5.A foundation 1/1 + 5.B BE CRUD/image/bulk 3/3 + 5.C BE Setting+Latihan 2/2 + 5.D BE Ulangan FULL 4/4 + 5.E BE Hasil consolidated 1/1 + 5.F FE Guru 2/2 + **5.G FE Siswa 2/2 ✅** commits `c83a15e`+`d63124d`+`928401b`+`57eb504`+`dabbdf1`+`7b9edd5`+`d6c808d`+`0346609`+`32f63ae`+`d822d46`+`5067f0a`+`d262ea3`+`2587526`+`8c55651`+`8c74e38`+`4195efa`+`e0fcb66`+`1716fab`+`6c10d19`. **UX/QA pass post-close**: `22d2095` fix (durasi 360→300, image presign refresh 12m, autosave retry 2x exp backoff, dead `'expired'` HasilStatus dropped + dedupe) + `2de273c` docs/dogfood report. HTTP smoke 49/50 + extra 8/8 hijau. Dogfood report di `dogfood-output/fase5/report.md`.)
- [ ] Fase 6 — Ulangan Harian (cross-bab) — plan ✅ DECOMPOSED 15 task locked #83-#88; estimasi 7 hari. **6.A foundation 1/1 ✅ DONE** commit `3371e30` — migration 000011 + 7 model + repo skeleton, 6 tables (bank_soal, ujian, ujian_soal, hasil_ujian, jawaban_ujian, event_ujian) + 17 indexes, schema_version `000011_ujian`. **6.B BankSoal 3/3 ✅ CLOSED** — 6.B.1 `f50e7f2` CRUD per-guru pribadi (smoke 26/26) + 6.B.2 `76de898` image upload 6-slot R2 prefix `soal-bank/` + WebP→JPEG fallback + presign inline 15m + atomic-swap old-key cleanup + version orthogonal (smoke 40/40) + 6.B.3 `ceaf86b` bulk paste pipe-delimited 8 kolom + tag default per-batch + 7 reason codes + cap 200 line + escape `\|` literal (smoke 27/27). **6.C Ujian setup 2/2 ✅ CLOSED** — 6.C.1+6.C.2 `ede3194` CRUD/duplicate (junction copied via SetUjianSoalIDs, R2 image keys SHARED via BankSoal pribadi guru locked #84) + source mode dispatch manual/random (locked #85) + PreviewSource + active-attempts guard (smoke 52/52). **6.D Ujian flow 3/4** — 6.D.1 `7d465bf` Start deterministic seed locked #86 + Items anti-cheat strip (smoke 28/28); 6.D.2 `d2ecef9` answer save delayed grade locked #76 mirror (UPSERT JawabanUjian is_benar=NULL+poin_dapat=0; anti-cheat soal_in_pool guard; smoke 32/32); 6.D.3 `205be54` submit + auto-grade tx + advisory lock locked #87 (single-tx grade pakai banksoal.Jawaban as truth; pg_advisory_xact_lock("hasil-submit:"||id); idempotent already_submitted; late-submit grace 5s 410 submit_after_grace; concurrent-safe winner+loser; smoke 32/32 incl concurrent 2 parallel + DB lifecycle + audit single-event). storage.CategoryBankSoal added. Sisa Fase 6: 6.D.4 timer cron 30s + 6.E Hasil consolidated 1/1 + 6.F FE Guru 2/2 + 6.G FE Siswa 2/2.
- [ ] Fase 7 — Rekap Nilai + Activity Feed
- [ ] Fase 8 — Polish + E2E

## Critical conventions
- Timezone: server lock `Asia/Jakarta`, FE tampil WIB explicit.
- Storage: **Cloudflare R2** (S3-compatible) — bucket `lms-prod` (live) / `lms-dev` (workspace), object key `<kategori>/<uuid>.<ext>` (kategori = tugas|soal|materi|submission|import). Akses lewat presigned GET URL (TTL 15m).
- Auth: JWT access 15m stateless + refresh 7d stateful (RefreshToken table, rotation, reuse detection).
- Rate limit: `/auth/login` 5/15m per (IP+email), global 120/min per IP, refresh 10/min, kelas/join 10/min, upload 30/min.
- Optimistic concurrency: `Version` field di Bab/Kelas/SoalBab/UlanganBabSetting/Soal/Ujian.
- Submit transition: `SELECT FOR UPDATE` + cek status di transaction, idempotent.
- Health: `/api/v1/healthz` (liveness, no DB), `/api/v1/readyz` (DB ping + R2 `HeadBucket` cached 30s).
- Request ID: middleware bikin `X-Request-ID` di semua request, propagate ke slog.

## Deploy
- Single Go binary serve API + static FE di port 8200.
- systemd unit: `lms-api.service` (User=ubuntu, EnvironmentFile=.env, Restart=always).
- Tidak ada Nginx (mirip fb-bot).
- Backup: pg_dump cron daily, retain 30 hari rolling + monthly archive 1 tahun.
- Cleanup daily cron: orphan files, ImportJob expired, LoginAttempt >30d, RefreshToken expired >7d, Submission file kelas archived +1y.
- **Deploy script:** `deploy/deploy.sh --remote` (existing). Flow: build BE binaries (lms-api+seed-admin+reset-admin) → build FE static → migrate up (idempotent) → systemctl restart lms-api → curl `/api/v1/readyz` confirm. **Wajib source `.env` dulu sebelum invoke** karena migrate butuh `DATABASE_URL`. Idempotent untuk FE-only commits (migrate up = no change).
  - Eksekusi: `ssh rdpkhorur 'cd /home/ubuntu/lms && set -a; . ./.env; set +a; bash deploy/deploy.sh --remote'`
  - BE deploy gotcha: stop service dulu sebelum cp binary kalau build manual ke `/tmp` (script `deploy.sh` handle ini sendiri).

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
