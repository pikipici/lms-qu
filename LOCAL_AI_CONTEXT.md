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
- Roadmap & locked decisions: `.kiro/steering/lms-roadmap.md` (v0.12.0 — Fase 7 OPEN, decomposed 12 task A-G, head `dfd1176`).
  - Fase 4 14/14, Fase 5 15/15 + UX/QA pass `22d2095`, Fase 6 15/15 + UX/QA pass `6e10888`, **Fase 7 IN PROGRESS** (Task 7.A.1 ✅ BE nilai siswa, 7.A.2 ✅ FE siswa rekap, **7.B ✅ BE+FE guru rekap matrix + CSV** commit `adf5839`+`dfd1176`, smoke BE 7.A 16/16 + 7.B 17/17 hijau).
- 94 locked decisions (v0.10.0 add #76-#82 Fase 5; v0.11.0 add #83-#88 Fase 6; **v0.12.0 add #89-#94 Fase 7**: sub-fase split 7.A-7.G, read-only at-query-time aggregator (no nilai_* tables), formula NilaiBab = avg(tugas%) + avg(soalbab%) simple, FE guru rekap routing, activity feed polling+cursor, CSV export). 10 open decisions.
- Active focus: **Fase 7 Task 7.C — activity feed guru** (`GET /guru/feed?cursor=...` polling 30s, locked #39+#55).

## Phase tracker
- [x] Fase 0 — Setup (DONE, smoke test passed, migrate 000001_init applied)
- [x] Fase 1 — Auth & Admin Panel (DONE: 1.A-1.H + 1.I, backend admin domain CLOSED, FE auth/admin shell/pengguna/audit-log/login-attempts shipped)
- [x] Fase 2 — Kelas, Enrollment, Bulk Import (DONE 20/20: 2.A.1, 2.A.2, 2.B FULL, 2.C FULL, 2.D FULL 6/6, 2.E FULL FE Admin Import 3/3 DONE 2026-05-21 commit `0f3772e`)
- [x] Fase 3 — Bab & Materi + Pengumuman (✅ DONE 17/17 = 100% complete; 3.A backend 4/4 DONE, 3.B FE Guru 2/2 DONE, 3.C Materi BE 4/4 CLOSED commit `caad20a`, 3.D Materi FE 2/2 CLOSED commits `eeca652` + `d08df3f`, 3.E Bab Siswa 2/2 CLOSED commits `c0d795a` + `3a69ddb`, 3.F Pengumuman 3/3 CLOSED: 3.F.1 BE commit `cf8c5bc` migration 000007 + CRUD endpoints + 18/18 tests, 3.F.2 + 3.F.3 FE combined commit `1ab48f7` — guru CRUD list+filter+compose+edit+archive+delete + siswa read-only kelas-wide + bab-scoped, lint cleanup `6d3cc6f`)
- [x] Fase 4 — Tugas (✅ DONE 14/14 = 100% complete; closed in this session: 4.A BE 4/4 ✅ migration `b6a2cf9` + CRUD `dc7d237` + attachment `55fb86a` + duplicate `3600188` (R2 CopyObject mirror bab pattern); 4.B FE Guru 2/2 ✅ `c4acf54`; 4.C BE 4/4 ✅ migration 000009 + repo `55296be` + submit/get/list/grade `6200d16` smoke E2E pass; 4.D FE Siswa 2/2 ✅ `9f8e9d0`+`fe50c66` SubmissionPanel + tugas detail page + `5d160b6`+`9d5eda2` ListMine endpoint + `6f49e14` /siswa/tugas riwayat lintas kelas page; 4.E FE Guru Review 2/2 ✅ `0775ead` SubmissionReviewList + GradeSubmissionDialog + `a4f14a4` pending-counts BE + `34aff41` sidebar badge + dashboard card. Locked #70-#75)
- [x] Fase 5 — Soal Bab ✅ CLOSED 2026-05-22 (15/15 = 100% locked #76-#82 — 5.A foundation 1/1 + 5.B BE CRUD/image/bulk 3/3 + 5.C BE Setting+Latihan 2/2 + 5.D BE Ulangan FULL 4/4 + 5.E BE Hasil consolidated 1/1 + 5.F FE Guru 2/2 + **5.G FE Siswa 2/2 ✅** commits `c83a15e`+`d63124d`+`928401b`+`57eb504`+`dabbdf1`+`7b9edd5`+`d6c808d`+`0346609`+`32f63ae`+`d822d46`+`5067f0a`+`d262ea3`+`2587526`+`8c55651`+`8c74e38`+`4195efa`+`e0fcb66`+`1716fab`+`6c10d19`. **UX/QA pass post-close**: `22d2095` fix (durasi 360→300, image presign refresh 12m, autosave retry 2x exp backoff, dead `'expired'` HasilStatus dropped + dedupe) + `2de273c` docs/dogfood report. HTTP smoke 49/50 + extra 8/8 hijau. Dogfood report di `dogfood-output/fase5/report.md`.)
- [x] Fase 6 — Ulangan Harian (cross-bab) ✅ CLOSED 2026-05-22 (15/15 = 100% locked #83-#88 met).
  - 6.A foundation 1/1 + 6.B BankSoal CRUD/image/bulk 3/3 + 6.C Ujian setup CRUD/duplicate/source 2/2 + 6.D Ujian flow start/answer/submit/cron 4/4 + 6.E Hasil consolidated review/cancel/rekap 1/1 + 6.F FE Guru BankSoal+Ujian 2/2 + 6.G FE Siswa lobby+player/review 2/2.
  - Smoke E2E 416/416 hijau (6.B 26+40+27, 6.C 52, 6.D 28+32+32+20, 6.E 37, 6.F 14+14, 6.G 18+29).
  - Highlights: anti-cheat #76 verified (items strip `jawaban_benar`, answer no `is_benar`), review gating #81 enforced, deterministic seed #86 verified (resume same hasil_id same pool), cron 30s + advisory lock #87 mutex submit/cron tested. UjianPlayer = timer countdown + autosave 600ms + auto-submit on expire (guarded ref) + R2 presign refresh 12m + optimistic `queryClient.setQueryData`. UjianSection orchestrator 4-state (lobby ↔ playing → result → review). Page route `/siswa/kelas/detail/ujian` static export query-string `?id=K&uid=U`.
  - Coverage gate #88 ≥70% defer to Fase 8 TODO (mirror Fase 5 #82 soft fallback).
  - Commits: `3371e30`+`f50e7f2`+`76de898`+`ceaf86b`+`ede3194`+`7d465bf`+`d2ecef9`+`205be54`+`0df6f89`+`8f77dbc`+`1269846`+`446f187`+`d9012b1`+`19060d0`.
  - **UX/QA pass post-close** `6e10888` + roadmap bump `b262142`: 5 findings (1 Critical 3-way drift Go `MaxDurasiMenit=600` vs DB CHECK 300 vs FE max=360 → HTTP 500 mentah, 1 High FE form max mismatch, 1 Medium banksoal-api error mapper drift, 2 Low siswa-ujian-api alias/orphan). All fixed: BE 600→300, FE form 360→300, banksoal-api drop 3 dead arms + add 5 BE-truth arms (`payload_too_large`/`unsupported_mime`/`image_slot_empty`/`r2_unavailable`/`missing_file`), siswa-ujian-api rename `ujian_not_started`→`ujian_window_not_open` + drop redundant `timer_expired`. Boundary smoke `dogfood-output/fase6/smoke-bounds.sh` 9/9. Dogfood report `dogfood-output/fase6/report.md`.
- [-] Fase 7 — Rekap Nilai + Activity Feed (IN PROGRESS, decomposed 12 task A-G, head `dfd1176`)
  - **Locked #89-#94**: sub-fase split 7.A-7.G; read-only at-query-time aggregator (NO `nilai_*` tables, compute on-query via repo); formula NilaiBab = avg(tugas%) + avg(soalbab%) simple; FE guru rekap routing; activity feed polling + cursor pagination; CSV export.
  - **Schema findings (locked in implementation)**: `tugas` TIDAK punya `deleted_at` & TIDAK punya `max_nilai` (pakai `nilai_setelah_penalty` NUMERIC(5,2) langsung skala 0..100 + filter `status='published'`). `hasil_ujian` punya `deleted_at` + `status` (selesai/dibatalkan/berlangsung).
  - **Postgres quirk locked**: `MAX(uuid)` SQLSTATE 42883 NOT supported → pattern 2-CTE (agg `MAX/COUNT` non-uuid + `DISTINCT ON ujian_id ORDER BY at DESC` last_attempt) + JOIN.
  - **Task 7.A.1 ✅ CLOSED** 2026-05-22 commits `d93de60`+`5839951`+`f6d9532` — BE nilai siswa: package `internal/nilai/` (model+repo+service+handler), routes `GET /siswa/kelas/:id/nilai` + `GET /siswa/nilai` (cross-class aggregator), 4 query methods. Smoke `/tmp/qa-7a.sh` 16/16 PASS.
  - **Task 7.A.2 ✅ CLOSED** 2026-05-22 commit `fb8c7a5` — FE siswa rekap nilai: `lib/nilai-api.ts`, `components/siswa/SiswaNilai{BabTable,UjianList}.tsx`, pages `/siswa/kelas/detail/nilai` + `/siswa/nilai`. Sidebar+CTA wiring.
  - **Task 7.B ✅ CLOSED** 2026-05-22 commits `adf5839` (BE) + `dfd1176` (FE) — Guru rekap matrix + CSV: `internal/nilai/rekap.go` (GuruKelasRekap service, reuse aggregator + per-siswa loop bounded MVP cap 10K) + `rekap_csv.go` (RFC4180 encoder) + `user_lookup.go` (auth.Repo adapter), route `kelasGroup.Get(/:id/rekap)` dgn `?format=json|csv`. FE: `components/guru/GuruRekapMatrix.tsx` (sticky-header table siswa × bab/ujian, sub-cols Total/Ul/Tg + Best/Last/N, color-tier), page `/guru/kelas/detail/rekap?id=KID` + `Download CSV` button (auth-aware Blob save-as), CTA dari kelas detail (ScrollText icon). Smoke `/tmp/qa-7b.sh` 17/17 PASS (anon 401, siswa 403, invalid 400, shape, header counts == row counts, guru-lain 403, CSV content-type/disposition/header, unknown kelas 404). T6 admin login skip.
  - **Next**: Task 7.C — activity feed guru (`GET /guru/feed?cursor=...&limit=20`, polling 30s, opaque cursor `(at_unix_micro, id)` per locked #39+#55).
- [ ] Fase 7 (sisa) — 7.C Activity feed / 7.D Pending counters / 7.E Guru audit log / 7.F UX/QA pass / 7.G close v0.13.0
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
