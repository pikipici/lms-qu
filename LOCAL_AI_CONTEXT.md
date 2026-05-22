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
- Roadmap & locked decisions: `.kiro/steering/lms-roadmap.md` (v0.13.0 — **Fase 7 CLOSED 12/12**, head `7abb804`).
  - Fase 4 14/14, Fase 5 15/15 + UX/QA pass `22d2095`, Fase 6 15/15 + UX/QA pass `6e10888`, **Fase 7 ✅ CLOSED 12/12** v0.13.0 release `7abb804` (7.A.1+7.A.2+7.B+7.C+7.D+7.E+7.F all green, smoke 96/96 hijau).
- 94 locked decisions (v0.10.0 add #76-#82 Fase 5; v0.11.0 add #83-#88 Fase 6; v0.12.0 add #89-#94 Fase 7). 10 open decisions.
- Active focus: **Fase 8 — Polish + E2E** (production hardening, backup/restore drill, cleanup cron, coverage gate, Playwright E2E core flows, error key naming cleanup deferred from 7.F).

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
- [-] Fase 7 — Rekap Nilai + Activity Feed (✅ CLOSED 12/12 v0.13.0 release `7abb804`)
  - **Locked #89-#94**: sub-fase split 7.A-7.G; read-only at-query-time aggregator (NO `nilai_*` tables, compute on-query via repo); formula NilaiBab = avg(tugas%) + avg(soalbab%) simple; FE guru rekap routing; activity feed polling + cursor pagination; CSV export.
  - **Schema findings (locked in implementation)**: `tugas` TIDAK punya `deleted_at` & TIDAK punya `max_nilai` (pakai `nilai_setelah_penalty` NUMERIC(5,2) langsung skala 0..100 + filter `status='published'`). `hasil_ujian` punya `deleted_at` + `status` (selesai/dibatalkan/berlangsung).
  - **Postgres quirk locked**: `MAX(uuid)` SQLSTATE 42883 NOT supported → pattern 2-CTE (agg `MAX/COUNT` non-uuid + `DISTINCT ON ujian_id ORDER BY at DESC` last_attempt) + JOIN.
  - **Task 7.A.1 ✅** 2026-05-22 commits `d93de60`+`5839951`+`f6d9532` — BE nilai siswa: `internal/nilai/` package, routes `/siswa/kelas/:id/nilai` + `/siswa/nilai`. Smoke 16/16 PASS.
  - **Task 7.A.2 ✅** 2026-05-22 commit `fb8c7a5` — FE siswa rekap nilai: pages `/siswa/kelas/detail/nilai` + `/siswa/nilai` + sidebar/CTA wiring.
  - **Task 7.B ✅** 2026-05-22 commits `adf5839`+`dfd1176` — Guru rekap matrix + CSV: BE `internal/nilai/rekap.go|rekap_csv.go|user_lookup.go` + `kelasGroup.Get(/:id/rekap)` JSON/CSV; FE `components/guru/GuruRekapMatrix.tsx` + page `/guru/kelas/detail/rekap` + Download CSV. Smoke 17/17 PASS.
  - **Task 7.C ✅** 2026-05-22 commits `b537b2a` (BE) + `ce4f28b` (FE) — Activity feed guru: `internal/feed/` package (UNION ALL aggregator submission_baru/ulangan_selesai/siswa_join, opaque base64 cursor `(at_unix_micro DESC, id DESC)` per #55, default limit=20 max=50, admin sees all). FE `lib/feed-api.ts` + `components/guru/GuruFeedList.tsx` (useInfiniteQuery + 30s polling per #39 + per-event row icon+link + late/nilai badges + load-more), mounted di guru dashboard "Aktivitas terbaru" card. Smoke `/tmp/qa-7c.sh` 16/16 PASS (anon 401, siswa 403, shape, kind enum, limit clamp 5/999→50, invalid cursor 400, cursor pagination ID disjoint, guru1/guru2 kelas disjoint).
  - **Task 7.D ✅** 2026-05-22 commit `75316bd` — Pending counters consolidated 3-counter (locked #40 + #93). BE `submission/pending.go`: extend `PendingCountsResult` dgn `pending_review_ulangan` (hasil_soal_bab status=selesai mode=ulangan + ulangan_bab_setting.izinkan_review_setelah_submit=true + bab.kelas_id IN guru-kelas) + `pending_review_ujian` (hasil_ujian status=selesai deleted_at IS NULL + ujian.izinkan_review_setelah_submit=true + ujian.kelas_id IN guru-kelas). 3 query parallel pakai sync.WaitGroup share kelas-IDs filter. FE `guru-api.ts` extend types, `guru/layout.tsx` sidebar badge pakai `pending_total` (sum 3 counters), `guru/page.tsx` 4-card grid (Total Kelas, Tugas, Review Ulangan, Review Ujian) drop CTA Bikin kelas. Smoke `/tmp/qa-7d.sh` 11/11 PASS (anon 401, siswa 403, shape 3 fields ≥0 untuk guru1/guru2). Live data: guru1 ungraded=1 review_ulangan=10 review_ujian=63, guru2 zeros (no kelas).
  - **Task 7.E ✅** 2026-05-22 commits `3ed9f67`+`dc7896b` — Guru audit scope per kelas (locked #59). BE: `internal/audit/` package baru (Service+Handler+KelasFinderAdapter+types), endpoint `GET /api/v1/guru/kelas/:id/audit?action=&limit=&offset=` + `GET /api/v1/guru/audit-actions`. Hard scope `WHERE target_kelas_id=:id` + ownership guard guru. AllowedActions 47 entries match real DB emitters (kelas/bab/materi/soalbab/ulangan/tugas/submission/ujian/pengumuman lifecycle + reset + siswa membership). `auth.AuditLogFilter` +TargetKelasID +Actions []string (defense-in-depth output filter). `auth.Repo.BulkUserNames` bulk lookup actor names. FE `lib/audit-api.ts` + page `/guru/kelas/detail/audit?id=` dgn filter dropdown + offset pagination 50/page + ACTION_LABEL Bahasa + tombol "Audit log" di kelas detail. Smoke `/tmp/qa-7e.sh` 20/20 PASS (anon 401, siswa 403, cross-kelas tidak ditest karena guru2 no kelas, invalid uuid 400, not found 404, invalid action 400, valid action 200, limit clamp 999→100 + 0→400, offset -1→400, audit-actions length 44, anon 401 untuk audit-actions, entry shape valid + target_kelas_id match).
  - **Task 7.F ✅** 2026-05-22 commit `6bf3f0e` — UX/QA pass static FE↔BE contract drift audit (browser unavailable Windows, fallback static + HTTP probe). Findings 5 total: 0 Critical, 2 High (feed limit silent fallback), 2 Medium (audit error envelope drift + error key naming convention), 1 Low (pendingQ refetchIntervalInBackground). Fix #1+#2 HIGH: `feed/handler.go` strict limit parsing — parse fail or n<1 → 400 invalid_limit (sebelumnya silent fallback ke default 20). Fix #4 MEDIUM: `audit/handler.go` errResp align ke project pattern `{error, code, request_id}` (sebelumnya `{error, message}` — break FE error toast + miss request_id). Fix #5 LOW: `guru/page.tsx` pendingQ +refetchIntervalInBackground:false defensive consistency. Defer #3 MEDIUM (error key naming `invalid_id` vs `invalid_kelas_id`) ke v0.14.0 — multi-file blast radius, no behavioral break. Smoke `/tmp/qa-7f.sh` 16/16 PASS (feed limit invalid → 400, audit envelope shape verified). Regression 7C+7D+7E semua hijau no break. Report `dogfood-output/fase7/report.md`.
  - **Task 7.G ✅** 2026-05-22 release commit `7abb804` tag `v0.13.0` — Roadmap bumped v0.12.0→v0.13.0 (Fase 7 CLOSED 12/12), release notes `release-notes/v0.13.0.md` summarize 6 features + technical notes + smoke 96/96 + commit chain. Tag pushed origin+server. Active focus → Fase 8 Polish + E2E.
- [ ] Fase 8 — Polish + E2E (next active)

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
