# LMS Project — Roadmap & Living Plan

> Status: v0.7.2 — Fase 1 in progress: 1.A + 1.B + 1.C + 1.D + 1.E + 1.F FULL + 1.G FULL + 1.H FULL DONE. Backend admin domain CLOSED. FE admin: auth stack + admin shell + /admin/pengguna list/create/detail + /admin/audit-log + /admin/login-attempts. Fase 2 in progress: 2.A.1 + 2.A.2 + 2.B FULL + 2.C.1 + 2.C.2 DONE (kelas/enrollment/import_jobs schema + GORM repos + kode invite generator + CRUD endpoints + FE guru shell list/create/detail/edit/duplicate/archive + siswa join via kode invite + admin bulk-enroll). Berikut: Task 2.C.3 (FE siswa dashboard + join kelas form).
> Owner: User (guru) + Apis (assistant)
> Last updated: 2026-05-20 (Section 18: Task 2.C.2 marked done; admin bulk-enroll shipped — handler+tests+audit+rate-limit, E2E 10 scenario hijau termasuk partial success mixed bucket, role mismatch 403, archived 409, oversize 400)

## Daftar Isi
- [0. Locked Decisions](#0-locked-decisions-v072)
- [1. Goal](#1-goal)
- [2. Target Users](#2-target-users)
- [3. Tech Stack](#3-tech-stack)
- [4. Core Features (MVP)](#4-core-features-mvp)
- [5. User Flows](#5-user-flows)
- [6. Data Model (GORM)](#6-data-model-gorm)
- [7. API Endpoints](#7-api-endpoints-apiv1)
- [8. Routes / Screens (Next.js)](#8-routes--screens-nextjs)
- [9. Project Structure](#9-project-structure)
- [10. Phasing / Roadmap](#10-phasing--roadmap)
- [11. Risks / Concerns](#11-risks--concerns)
- [12. Open Decisions Tersisa](#12-open-decisions-tersisa-v072)
- [13. Deploy Strategy](#13-deploy-strategy-mengikuti-pola-fb-bot)
- [14. Frontend Development Arsenal](#14-frontend-development-arsenal--skills--agents)
- [15. Implementation Notes](#15-implementation-notes)
- [16. Current Next Step](#16-current-next-step)
- [17. First Admin Bootstrap](#17-first-admin-bootstrap)
- [18. Task-by-Task Implementation Plan (Fase 0-2)](#18-task-by-task-implementation-plan-fase-0-2)

---

## 0. Locked Decisions (v0.7.2)

| # | Keputusan | Pilihan |
|---|-----------|---------|
| 1 | Skala guru | Multi-teacher (flat, no multi-tenant) |
| 2 | Backend | Go + **Fiber** + GORM + PostgreSQL |
| 3 | Frontend | Next.js 14 + TS + Tailwind + shadcn/ui + Zustand + TanStack Query |
| 4 | Frontend build mode | **Static export** (`output: 'export'`) — di-serve oleh Go Fiber sebagai static, mirip fb-bot |
| 5 | Jenis soal ujian | Pilihan Ganda (MCQ) saja |
| 6 | Storage materi | Local disk (`./storage/uploads/`) di rdpkhorur |
| 7 | Anti-cheat MVP | Timer server-side + shuffle soal/opsi + log tab-switch (tanpa hard lock) |
| 8 | Auth | JWT (access 15m + refresh 7d), bcrypt password |
| 9 | Deploy target | rdpkhorur, mengikuti pola fb-bot (lihat Section 13) |
| 10 | Deadline | Tidak ada — santai |
| 11 | User lifecycle | **Admin-controlled, no public self-register** |
| 12 | Roles | `admin` \| `guru` \| `siswa` (3 role flat) |
| 13 | Siswa join kelas | Hybrid: admin import/create + assign, atau via kode |
| 14 | Bulk import siswa | YA di MVP (Fase 2), via CSV |
| 15 | Struktur kelas | **Berbasis Bab** — kelas terdiri dari banyak Bab, materi/soal/tugas nempel ke bab |
| 16 | Soal Bab — mode | Dual: Latihan (retry unlimited, no nilai) + Ulangan Bab (sekali, masuk nilai bab) |
| 17 | Nilai Bab — formula | Rata-rata tertimbang: `(SoalUlanganBab × bobot1 + Tugas × bobot2) / total bobot` — bobot diset guru per kelas |
| 18 | Latihan mandiri | TIDAK masuk hitungan nilai bab (formative only) |
| 19 | Ulangan harian (lintas bab) | Berdiri sendiri di "rapor kelas", tidak masuk nilai bab tertentu |
| 20 | Materi & Tugas | Boleh punya `BabID` (nullable) — bisa nempel ke bab atau berdiri bebas |
| 21 | Ulangan recovery | Resume support — siswa boleh re-login + lanjut, timer server-side jalan terus, jawaban yang udah ke-save kepake |
| 22 | Soal dengan gambar | Tiap soal boleh punya `GambarSoal` (1 gambar) + tiap opsi A-E boleh punya gambar (untuk soal "pilih gambar") |
| 23 | Edit/resubmit tugas | Siswa boleh resubmit selama belum lewat deadline & belum di-grade |
| 24 | Late submission | Per-tugas: `IzinkanLate` + `PenaltyPersen`. Default: tolak setelah deadline |
| 25 | Mark materi as read | Track via tabel `MateriRead`, untuk progress per bab |
| 26 | Remedial / reset attempt | Guru bisa reset HasilSoalBab/HasilUjian per siswa supaya bisa start ulang |
| 27 | Pengumuman per kelas | Ada — bisa per-kelas atau per-bab |
| 28 | Preview ulangan untuk guru | Ada — render mode read-only sebelum publish |
| 29 | Timezone | Server lock ke `Asia/Jakarta` (WIB). Frontend tampilkan WIB explicit |
| 30 | Soft delete | Kelas + Bab pakai `ArchivedAt` (archive); hard delete cuma admin |
| 31 | Password awal user baru | Admin bisa ketik manual ATAU klik "Generate" (8 char acak alfanumerik). Password ditampilkan SEKALI di modal sukses, admin kasih tau user manual (chat/papan tulis/print). Plaintext gak disimpan, langsung di-hash bcrypt. |
| 32 | Force change password | User wajib ganti password pas login pertama. Field `MustChangePassword` di User. Set `true` saat admin create / reset password. Login sukses tapi semua endpoint kecuali `/auth/me` & `/auth/change-password` return 403 sampai diganti. Frontend redirect paksa ke `/me/security`. |
| 33 | Review jawaban setelah ulangan submitted | Per-ulangan setting: `IzinkanReviewSetelahSubmit` (bool, default `false`) + `WaktuBukaReview` (nullable timestamp). Logika: kalau `true` -> review terbuka langsung setelah submit. Kalau ada `WaktuBukaReview` -> review terbuka setelah waktu itu. Default: cuma tampil skor total. |
| 34 | Random pool — Ulangan Bab | Tambah `JumlahSoalRandom` (nullable int) di UlanganBabSetting. Kalau diisi: per siswa cuma dapat N soal random dari pool mode=ulangan. Kalau null: semua soal mode=ulangan (default). |
| 35 | Database migration | **golang-migrate/migrate** (versioned SQL files di `backend/migrations/`). Production: `migrate up`. Dev: GORM AutoMigrate diaktifkan via flag (`-automigrate=true`) untuk iterasi cepat. Setiap perubahan schema = 1 migration file commit. |
| 36 | Login security | Rate limit 5 gagal/15 menit per `(IP, email)` pakai Fiber middleware (in-memory store cukup untuk MVP). Akun `locked` setelah 10 gagal kumulatif (admin reset). Tiap login attempt (success + fail) masuk `AuditLog` dengan IP + UserAgent. |
| 37 | Status Bab | Field `Status` di Bab: `draft` (default) / `published` / `archived`. Siswa cuma lihat `published`. Guru bisa transisi: draft -> published -> archived (atau back ke draft). |
| 38 | Duplicate kelas/bab/ulangan | Endpoint `POST /kelas/:id/duplicate`, `POST /bab/:id/duplicate`, `POST /ulangan/:id/duplicate`. Copy isi (materi, soal, tugas tanpa submission, ulangan tanpa hasil). Kelas: regenerate kode invite, no enrollment carry. Status hasil duplicate: `draft`. |
| 39 | Activity feed guru | Polling 30s di dashboard guru: GET `/guru/feed?since=...` -> 20 event terbaru (submission masuk, ulangan selesai, siswa join). |
| 40 | Pending counters | Sidebar guru badge: `ungraded_submissions`, `pending_review_ulangan`. Dipakai untuk pengingat. GET `/guru/pending-counts`. |
| 41 | Forgot password | Halaman `/lupa-password`: cuma instruksi "Hubungi admin sekolah/guru wali kelas untuk reset password". Tidak ada PasswordResetRequest table di MVP — admin reset manual via dashboard. |
| 42 | Session/JWT revocation | Refresh token disimpan di DB (`RefreshToken { jti, user_id, issued_at, expires_at, revoked_at, ip, user_agent }`). Access token tetap stateless 15m. Logout / suspend / lock / change-password / admin reset password → revoke semua refresh token user (kecuali current device saat self change-password, opsional). Refresh request cek `revoked_at IS NULL` + rotate (issue jti baru, mark old revoked). Compromised token mitigation. |
| 43 | Submit concurrency | Transition `HasilSoalBab` / `HasilUjian` dari `berlangsung → submitted/expired` pakai `SELECT ... FOR UPDATE` di dalam transaction + cek status sebelum update. Auto-grade jalan dalam transaction yang sama. Idempotent: kalau status udah `submitted/expired`, return hasil yang ada (no double grade). Background job timer-expire pakai advisory lock per row. |
| 44 | Health/readiness split | `/api/v1/healthz` (liveness, return 200 selalu kalau process hidup, no DB) + `/api/v1/readyz` (readiness, cek DB ping + storage dir writable + return 503 kalau ada yang fail). systemd `ExecStartPost` pakai readyz. Loadbalancer/uptime monitor pakai readyz. |
| 45 | Remedial snapshot policy | Saat reset attempt: HasilSoalBab/HasilUjian + JawabanBab/Jawaban + SoalAssignment di-soft-delete (`DeletedAt`). Attempt baru bikin **assignment baru fresh** (refetch SoalBab/Soal aktif sekarang). AuditLog catat: actor, target_siswa, target_bab/ujian, reason, jumlah_soal_lama, jumlah_soal_baru, soal_diff (added/removed IDs). Siswa lihat soal baru — penting kalau guru udah edit/tambah soal antar attempt. |
| 46 | File upload hardening | (1) Mime detect via `http.DetectContentType` (sniff isi, jangan trust client `Content-Type`). (2) Allowlist eksplisit per kategori: tugas (pdf, docx, jpg, png, zip), gambar soal (jpg, png, webp). (3) Filename sanitize: simpan sebagai `<uuid>.<ext>`, original name di DB column terpisah. (4) Gambar soal: resize on upload (max 1920px, quality 85) pake `disintegration/imaging` atau `nfnt/resize`. (5) PDF tugas max 20MB, gambar 5MB. (6) Block executable mime explicit (application/x-executable, application/x-msdownload, application/x-sh). (7) Upload dir di luar `frontend/out/` — di-serve via authenticated endpoint, bukan static. |
| 47 | Global rate limit | Selain `/auth/login` (5/15m per IP+email), tambahin: per-IP global 120 req/menit (Fiber `limiter` middleware), `/auth/refresh` 10/menit per refresh token, `/kelas/join` 10/menit per IP (cegah brute force kode invite), upload endpoints 30/menit per user. In-memory store cukup MVP. |
| 48 | Bab progress formula | Per siswa per bab: `progress_persen = round( (w_materi × pct_materi + w_latihan × pct_latihan + w_ulangan × pct_ulangan + w_tugas × pct_tugas) / total_w )` dengan default bobot equal (25/25/25/25), skip komponen yang gak ada (mis. bab tanpa tugas → bobot tugas dropped, sisanya re-normalize). pct_materi = `materi_dibaca / total_materi`. pct_latihan = `1 if ada attempt latihan else 0`. pct_ulangan = `1 if HasilSoalBab(mode=ulangan, status=submitted/expired) ada else 0`. pct_tugas = `submission_graded / total_tugas`. Display: progress bar + tooltip breakdown. |
| 49 | Request ID & observability | Middleware bikin `X-Request-ID` (UUID v4 atau dari header kalau ada) di semua request, propagate ke slog context (`request_id`, `user_id`, `path`, `method`). Response header echo balik. Error response include `request_id` supaya user bisa report ke admin. Dipasang dari Fase 0, bukan Fase 8. |
| 50 | Test coverage target | Per package backend: auth/admin/soalbab/ujian/nilai minimal 70% line coverage. Frontend critical path (login, form bikin user, kerjain ulangan, submit tugas) wajib ada Vitest unit + Playwright E2E (Fase 8). CI gate: `go test -cover ./...` fail kalau di bawah threshold. |
| 51 | Data retention policy | LoginAttempt 30 hari (auto-cleanup). AuditLog **forever** (compliance, kalau perlu archive ke cold storage di v1). Submission file: retain sampai kelas di-archive + 1 tahun, lalu hard-delete file (DB row tetap untuk nilai history). HasilSoalBab/HasilUjian deleted_at: hard delete setelah 1 tahun + audit log. Backup pg_dump: retain 30 hari rolling, monthly archive 1 tahun. Cleanup task daily cron di server. |
| 52 | Multi-admin promotion | Admin baru bisa dibikin via `/admin/users` create form (role=admin). Tapi promote/demote dari guru→admin atau sebaliknya wajib **re-auth**: admin yang melakukan harus re-input password sendiri di modal (POST `/admin/users/:id/role` { role, current_password }). AuditLog catat actor + target + role_lama + role_baru. Tidak ada self-demote (admin gak bisa demote dirinya sendiri kalau dia satu-satunya admin). |
| 53 | Admin lock-out recovery | `cmd/seed-admin` cuma jalan kalau belum ada admin. Kalau admin satu-satunya kena lock/forget password: `cmd/reset-admin` CLI minta email + password baru, override lewat akses fisik server. Production: butuh SSH access. AuditLog: `actor_id=NULL` + `action='admin_reset_via_cli'`. Tidak ada self-service recovery — by design (akses fisik = trust boundary). |
| 54 | CSV import preview persistence | Upload CSV → ImportJob status=`preview` (PreviewRowsJSON jsonb + valid_count + invalid_count). Confirm → status=`processing` → `completed`. Cancel atau timeout 1 jam tanpa confirm → status=`expired`, cleanup file + row. Admin bisa close tab tanpa lose preview state — reload `/admin/pengguna/import` resume preview kalau status=preview. |
| 55 | Activity feed cursor | `GET /guru/feed?cursor=BASE64&limit=20` pakai opaque cursor `(at_unix_micro, id)` di-base64. Default 20 item. Response: `{ events: [...], next_cursor }`. Polling 30s pakai `cursor=null` (latest 20) buat refresh; load-more pakai cursor. Cegah duplicate/skip kalau dua event timestamp sama. |
| 56 | Concurrent edit version | Tambah field `Version int default 1` di Bab, Kelas, SoalBab, Soal, UlanganBabSetting, Ujian. Increment tiap update. Request PATCH wajib include `version` di body, backend cek match → reject 409 + `{ error, current_version }` kalau mismatch. UI tampil "Konten ini diubah orang lain — refresh dulu". Cegah dua tab/device guru sama overwrite tanpa sadar. |
| 57 | Auth boundary explicit | **Endpoint tanpa auth (anon allowed):** `/auth/login`, `/auth/refresh`, `/healthz`, `/readyz`, static files (`/`, `/login`, `/lupa-password`). **Semua lain butuh auth.** Tambahan: enrollment check di endpoint kelas-scope (siswa hanya akses kelas yang dia enrolled, guru hanya akses kelas yang dia owner). Middleware order: ratelimit → request-id → auth → role-guard → enrollment-guard. |
| 58 | Storage path convention | Flat structure dengan kategori prefix: `./storage/uploads/<kategori>/<uuid>.<ext>` dimana kategori = `tugas`, `soal`, `materi`, `submission`, `import`. Tidak hierarki by bab/kelas — orphan cleanup lebih simple via "select uuid not in (select file_path from <ref tables>)". `OriginalFilename` disimpan di DB column terpisah untuk download UX. Saat duplicate kelas/bab → copy file ke uuid baru (jangan share). |
| 59 | Guru audit scope | `GET /guru/kelas/:id/audit?action=<filter>&limit=50` — guru bisa lihat audit log yang berkaitan kelas miliknya: action subset (`hasil_reset`, `bab_archived`, `bab_published`, `siswa_kicked`, `tugas_deleted`). Hanya entry dengan `target_kelas_id=<id>`. Berguna untuk transparansi kalau admin bantu reset attempt. |
| 60 | Frontend env strategy | `NEXT_PUBLIC_API_BASE` di-bake at build time (static export limit). **Production**: rebuild dengan `NEXT_PUBLIC_API_BASE=/api/v1` (same-origin). **Dev**: `.env.development.local` set `NEXT_PUBLIC_API_BASE=http://localhost:8200/api/v1`. Dokumentasikan di `docs/DEPLOY.md`: kalau base URL berubah, FE wajib rebuild. |

**Open (perlu sesi terpisah):**
- Notifikasi flow & desain — bedah di v0.8 setelah Fase 0-3 jalan.

---

## 1. Goal

LMS multi-guru, admin-controlled, **berbasis Bab**:
- **Admin**: manage user (guru, siswa), reset password, bulk import via CSV, audit log
- **Guru**: manage kelas + bab (materi, soal latihan, ulangan bab, tugas), ulangan harian lintas bab, penilaian
- **Siswa**: akses bab di kelas (lihat materi, kerjain latihan, ikut ulangan bab, submit tugas), kerjain ulangan harian, lihat nilai transparan per bab

---

## 2. Target Users

| Role  | Akses |
|-------|-------|
| Admin | Manage semua user (CRUD), reset password, suspend, bulk import siswa via CSV, lihat audit log |
| Guru  | Manage kelas yang dia bikin, bab + materi + soal + tugas, ulangan harian, nilai siswa di kelasnya |
| Siswa | Join kelas via kode (atau di-assign admin), akses bab + materi, kerjain latihan/ulangan, submit tugas, lihat nilai transparan |

**Akses register publik:** TIDAK ADA. Semua akun harus di-create oleh admin.

---

## 3. Tech Stack

### Backend (Go)
- **Framework**: Fiber v2
- **ORM**: GORM + AutoMigrate (awal), pindah `golang-migrate` kalau prod stabil
- **DB**: PostgreSQL 15+
- **Auth**: JWT (`github.com/golang-jwt/jwt/v5`), bcrypt (`golang.org/x/crypto/bcrypt`)
- **Validation**: `go-playground/validator/v10`
- **Config**: `.env` via `joho/godotenv` + `os.Getenv`
- **Logging**: `slog` (stdlib)
- **File upload**: Fiber multipart -> disk
- **Test**: stdlib + `testify`
- **Static serve**: Fiber `app.Static("/", "./frontend/out")` + SPA fallback

### Frontend (Next.js — static export)
- Next.js 14 App Router + TypeScript
- `output: 'export'` -> hasilnya di `frontend/out/` (HTML + JS + assets)
- Tailwind CSS + shadcn/ui (komponen siap pake)
- Zustand (global state ringan) + TanStack Query (server state)
- React Hook Form + Zod
- Auth: JWT disimpan di localStorage (atau cookie non-httpOnly karena static export gak bisa set cookie server-side)
- HTTP: fetch wrapper sederhana
- API base URL: same-origin (`/api/v1`) karena Go yang serve

**Catatan static export limits (gak masalah untuk LMS):**
- Gak ada API routes Next.js (gak perlu, Go handle)
- Gak ada ISR / server actions (gak perlu)
- Gak ada `next/image` optimizer di runtime (pake `unoptimized: true`)

### Storage & Infra
- File: `./storage/uploads/<kategori>/<uuid>.<ext>`
- DB: PostgreSQL lokal di rdpkhorur (DB user/pass di `.env`)
- Tidak ada Nginx — Go Fiber langsung listen `0.0.0.0:8200` (mirip fb-bot di 8100)
- Akses via SSH tunnel: `ssh -L 8200:127.0.0.1:8200 rdpkhorur`
- systemd unit: `lms-api.service` (1 service, simpler dari fb-bot)

---

## 4. Core Features (MVP)

### 4.0 Konsep Hierarki

```
Kelas
 └── Bab (1, 2, 3, ... — dengan urutan, judul, deskripsi)
      ├── Materi  (PDF / link YouTube / teks markdown — banyak per bab)
      ├── Soal Bab
      │     ├── Mode "Latihan" (retry unlimited, jawaban benar muncul setelah submit, TIDAK masuk nilai)
      │     └── Mode "Ulangan Bab" (1x kerja, masuk nilai bab)
      └── Tugas (opsional — bisa nempel ke bab atau berdiri sendiri)

Kelas (lintas bab)
 └── Ulangan Harian — assessment besar lintas-bab, di rapor kelas, TIDAK masuk nilai bab
```

Materi & Tugas punya field `BabID` nullable — kalau diisi, dia bagian dari bab itu; kalau null, dia berdiri bebas di kelas (legacy / pengumuman umum).

### 4.1 Admin
- Manage user (CRUD), reset password, suspend
- Bikin user: input nama+email+role, password bisa **ketik manual atau klik "Generate"** (8 char acak alfanumerik) — password ditampilkan SEKALI di modal sukses, admin kasih tahu user manual. `MustChangePassword=true` otomatis di-set
- Reset password user: sama flow dengan create — `MustChangePassword=true` di-set ulang
- Bulk import siswa via CSV — generate password per siswa, kasih file CSV download "credentials_<job_id>.csv" sekali
- Suspend / unlock akun (kalau locked karena gagal login berkali-kali)
- Lihat semua kelas (read-only)
- Audit log

### 4.2 Guru
- Login (akun dibuat admin) + force change password kalau pertama kali
- Dashboard: ringkasan, **activity feed** (polling 30s — submission masuk, ulangan selesai, siswa join), **pending counters** (badge tugas belum dinilai, ulangan belum di-review)
- Kelas: CRUD + archive + **duplicate (copy ke tahun ajaran baru)**, kode invite, list/kick siswa, set bobot nilai bab (Soal vs Tugas)
- Bab: CRUD + drag-and-drop urutan + **status (draft/published/archived)** + duplicate, per-bab tab (Materi / Soal / Tugas / Pengumuman / Pengaturan Ulangan Bab)
- Materi: upload PDF, link YouTube, teks markdown — per bab atau kelas
- Soal Bab: editor (form + bulk paste), set mode (latihan / ulangan), poin, gambar soal & gambar opsi (opsional)
- Pengaturan Ulangan Bab per bab: durasi, jadwal, shuffle, **JumlahSoalRandom (random N dari pool)**, **IzinkanReviewSetelahSubmit + WaktuBukaReview**
- Preview ulangan: render persis kayak siswa (mode read-only) sebelum publish
- Tugas: CRUD + deadline + max nilai + attachment + izinkan late + penalty persen, review submission, grade + feedback
- Remedial: reset attempt siswa untuk Ulangan Bab atau Ulangan Harian (bikin siswa bisa kerjain lagi, soft-delete + audit log dengan reason)
- Pengumuman: bikin pengumuman per-kelas atau per-bab
- Ulangan Harian (MCQ lintas bab): bank soal pribadi, buat ulangan + duplicate, auto-grade, rekap, `IzinkanReviewSetelahSubmit`
- Lihat rekap nilai per kelas: tabel siswa × bab + ulangan harian
- Export nilai CSV

### 4.3 Siswa
- Login (akun dibuat admin) + force change password kalau pertama kali
- Lupa password: halaman `/lupa-password` -> instruksi "Hubungi admin"
- Dashboard: list kelas + **banner "Lanjutkan ulangan"** kalau ada sesi berlangsung + button "Gabung Kelas"
- Join kelas via kode 6 char (kalau belum di-assign admin)
- Buka kelas -> lihat list bab (cuma yang `published`) + progress per bab + section ulangan harian + pengumuman
- Buka bab -> tab: Materi / Latihan / Tugas / Hasil
  - Materi: list materi (badge "sudah dibaca"), klik buat baca/embed -> auto mark-read
  - Latihan: kerjain soal mode latihan, retry unlimited, lihat jawaban benar
  - Tugas: submit tugas yang nempel di bab itu, boleh resubmit selama belum graded & belum lewat deadline
  - Hasil: breakdown transparan nilai bab — Ulangan Bab xx, Tugas xx, Bobot xx, Total xx. **Review jawaban ulangan** muncul kalau guru izinin (langsung atau setelah `WaktuBukaReview`)
- Submit tugas (file/teks). Kalau lewat deadline & guru izinin late: submission masuk dengan flag `LATE` + nilai max akan di-penalty
- Kerjain Ulangan Bab atau Ulangan Harian
  - Recovery / resume: kalau browser crash atau internet putus, siswa login lagi -> dashboard tampilin "Ulangan sedang berlangsung" -> klik resume -> lanjut dengan jawaban yang udah ke-save (timer server-side terus jalan, gak di-pause)
- Halaman Nilai (`/siswa/nilai`): full transparansi per kelas + lintas kelas

### 4.4 Anti-cheat (locked)
- Timer server-side autoritatif (berlaku untuk Ulangan Bab dan Ulangan Harian)
- Shuffle soal & shuffle opsi (per siswa, deterministik dari `mulai_at` + `siswa_id`)
- Random N dari pool (untuk Ulangan Bab + Ulangan Harian) — masing-masing siswa dapat soal yang beda
- Log event tab-switch & window-blur
- Tidak ada fullscreen lock

### 4.5 Login Security (locked)
- Rate limit: 5 gagal/15 menit per (IP + email) untuk `/auth/login`
- Global rate limit: 120 req/menit per IP (Fiber `limiter` middleware)
- `/auth/refresh` 10/menit per refresh token (cegah replay)
- `/kelas/join` 10/menit per IP (cegah brute force kode invite 6-char)
- Upload endpoints 30/menit per user
- Lockout: 10 gagal kumulatif -> akun `locked` (admin reset)
- Audit log semua login attempt (success + fail) dengan IP + UserAgent
- Bcrypt cost 12 untuk password hash
- JWT: access token 15m (stateless), refresh 7d (stateful, tracked di DB `RefreshToken`)
- Refresh token rotation tiap refresh (old jti revoked, new jti issued); reuse detection → revoke-all-chain
- Suspend / lock / change-password / admin reset → revoke semua refresh token user
- `MustChangePassword=true` -> semua endpoint return 403 kecuali `/auth/me`, `/auth/change-password`, `/auth/logout`

### 4.6 Notifikasi (TUNDA — bedah di v0.8)
Akan dibedah terpisah setelah Fase 0-3 jalan. Sementara: tidak ada placeholder UI bell, tidak ada notif store di FE/BE. Pengumuman pakai polling refresh biasa di dashboard kelas. Activity feed guru pakai polling 30s.

### 4.7 YAGNI
Tipe soal selain MCQ, video conference, forum, mobile app, AI grading, payment, multi-tenant, profile picture, komentar di submission, search global, bulk grade, soal LaTeX/MathJax, activity log siswa untuk guru, co-teacher 1 kelas, fitur "tahun ajaran/semester" terstruktur, konversi nilai ke huruf, print/export rapor PDF, maintenance mode, self-service forgot password (chat admin dulu), email notification.

---

## 5. User Flows

### 5.0 Landing Page & Auth Entry Flow

**No public self-register.** Semua user dibikin oleh admin. Landing page cuma punya 1 jalur masuk.

Landing page (`/`):
- Hero singkat (judul + tagline LMS) + 1 CTA: **"Masuk"** -> `/login`
- Footer kecil: link kontak admin/sekolah kalau user lupa kredensial

Login flow (`/login`):
1. User isi email + password
2. POST `/auth/login` -> backend response `{ access, refresh, user: { role, status } }`
3. Kalau `status = suspended` -> tolak + pesan "Akun dinonaktifkan, hubungi admin"
4. Frontend cek `user.role`:
   - `admin` -> redirect `/admin`
   - `guru`  -> redirect `/guru`
   - `siswa` -> redirect `/siswa`
5. Token disimpan di localStorage, Zustand store update

**Tidak ada `/register` publik.** Endpoint `/auth/register` juga tidak diekspose.

Reset password (MVP):
- User klik "Lupa password?" di `/login` -> tampil pesan "Hubungi admin sekolah untuk reset"
- Admin reset dari panel admin: pilih user -> klik "Reset Password" -> dapet password sementara untuk dikasih ke user

Self-change password (setelah login):
- User bisa ganti password sendiri di `/me/security` (semua role)
- Wajib re-input password lama

### 5.0a Flow Admin — Bikin Akun Guru / Siswa
1. Login admin -> `/admin`
2. Tab "Pengguna" -> "Tambah Pengguna"
3. Pilih role (guru/siswa) -> isi nama, email, password awal
4. (Opsional, kalau siswa) langsung assign ke kelas
5. Save -> akun aktif, kasih kredensial ke user via cara apa pun (chat/email manual)

### 5.0b Flow Admin — Bulk Import Siswa via CSV
1. `/admin/pengguna` -> "Import CSV"
2. Download template CSV (`name,email,password,kode_kelas?`)
3. Upload file CSV
4. Backend parse + validate per baris -> tampilkan preview (N valid, M error)
5. Konfirmasi -> backend insert massal dalam transaction
6. Hasil: ringkasan (X siswa di-create, Y enrolled ke kelas, Z error dengan alasan)

### 5.0c Flow Siswa — Join Kelas
Dua cara, tergantung apa yang admin lakukan saat create akun:
- **Cara A (admin assign langsung):** akun siswa udah pre-enrolled -> begitu login, kelas udah muncul
- **Cara B (siswa pakai kode kelas):** siswa login, klik "Gabung Kelas" di dashboard, masukin kode 6 char -> backend POST `/kelas/join` -> enrolled

### 5.1 Guru — Bikin Soal Bab (Latihan + Ulangan Bab)
1. Login -> Dashboard -> pilih kelas -> tab "Bab"
2. Pilih bab -> tab "Soal" -> "Tambah Soal"
3. Isi: pertanyaan, opsi A-E, jawaban benar, poin, **mode (latihan / ulangan)**
4. Repeat untuk soal lainnya, atau pakai "Bulk Paste"
5. Kalau ada soal mode `ulangan`: buka tab "Pengaturan Ulangan Bab" -> set durasi, jadwal, shuffle, aktifkan

### 5.2 Guru — Bikin Ulangan Harian (lintas bab)
1. Login -> Dashboard -> pilih kelas -> tab "Ulangan Harian"
2. "Buat Baru" -> isi judul, durasi, jadwal mulai/selesai
3. Tambah soal: ketik manual / pilih dari Bank Soal / random N dari topik bank
4. Setting: shuffle soal & opsi, tampilkan nilai langsung
5. Publish

### 5.3 Siswa — Latihan Soal Bab (formative, no nilai)
1. Login -> kelas -> bab -> tab "Latihan"
2. Klik "Mulai Latihan" -> server bikin attempt baru (`HasilSoalBab.mode=latihan`)
3. Jawab soal-soal sesuai shuffle pribadi
4. Submit -> auto-grade -> reveal jawaban benar + pembahasan
5. Boleh retry sebanyak yang siswa mau (attempt baru tiap retry)

### 5.4 Siswa — Kerjain Ulangan Bab (1x, masuk nilai)
1. Login -> kelas -> bab -> tab "Hasil" atau notif -> klik "Ulangan Bab" (kalau aktif)
2. Baca instruksi -> "Mulai" -> server cek belum pernah submit, bikin `HasilSoalBab.mode=ulangan` dengan `mulai_at = now`
3. Halaman fokus full screen, timer countdown server-authoritative
4. Tiap pilih jawaban -> debounced auto-save
5. Submit / timer habis -> auto-grade -> tampilkan skor (kalau guru izinin)
6. Nilai langsung masuk ke perhitungan Nilai Bab (lihat Section 6.2)

### 5.5 Siswa — Kerjain Ulangan Harian (lintas bab)
1. Login -> kelas -> section "Ulangan Harian" -> klik ulangan aktif
2. Baca instruksi -> "Mulai" -> server bikin `HasilUjian` dengan urutan soal/opsi sesuai shuffle pribadi
3. Halaman fokus + timer server-side
4. Auto-save jawaban tiap pilih
5. Submit / timer habis -> auto-grade -> nilai masuk section "Ulangan Harian" di rapor (TIDAK masuk Nilai Bab)

### 5.6 Siswa — Lihat Nilai (transparansi)
1. Login -> `/siswa/nilai` (lintas kelas) atau `/siswa/kelas/:id/nilai` (per kelas)
2. Per kelas, lihat list bab dengan breakdown:
   - Nilai Ulangan Bab (raw + skala 100)
   - Nilai Tugas Bab (avg dari semua tugas di bab itu)
   - Bobot yang dipake
   - Total Nilai Bab
3. Section terpisah: "Ulangan Harian" — list ulangan yang udah dikerjain + nilainya
4. Total Kelas = rata-rata Nilai Bab (skip NULL)

### 5.7 Guru — Review Tugas
1. Dashboard -> Tugas -> N submission baru
2. Buka submission -> download/lihat
3. Input nilai + feedback -> save
4. Status submission jadi "graded" -> nilai masuk hitungan Nilai Tugas Bab (kalau tugas itu nempel ke bab)

### 5.8 Siswa — Resume Ulangan (recovery dari crash / disconnect)
1. Siswa lagi kerjain Ulangan Bab / Harian -> browser crash, internet putus, atau laptop mati
2. Siswa login lagi -> dashboard nampilin banner "Ulangan sedang berlangsung — sisa waktu xx menit"
3. Klik "Lanjutkan" -> redirect ke `/play` ulangan tsb
4. Server return jawaban yang udah ke-save + sisa waktu (`mulai_at + durasi - now`)
5. Frontend render state, timer lanjut dari sisa waktu
6. Kalau timer udah habis pas siswa offline -> auto-submit (status `expired`), siswa langsung dapet skor tanpa bisa lanjut

### 5.9 Guru — Remedial (Reset Attempt)
1. Buka rekap hasil ulangan (`/guru/kelas/.../bab/.../hasil` atau `/ulangan/.../hasil`)
2. Pilih siswa yang mau direset -> klik "Reset Attempt"
3. Konfirmasi (warning: nilai sebelumnya akan ke-soft-delete, masuk audit log)
4. Backend: HasilSoalBab/HasilUjian + Jawaban-nya di-soft-delete (`DeletedAt`), siswa diijinkan start lagi
5. Siswa dapet ulangan tsb muncul lagi sebagai "tersedia" di dashboard

### 5.10 Guru — Bikin Pengumuman
1. Pilih kelas -> tab "Pengumuman" atau di tab Bab tertentu -> "Buat Pengumuman"
2. Isi: judul, isi (markdown), scope (kelas atau bab tertentu)
3. Publish -> langsung muncul di dashboard siswa pas refresh
4. Siswa lihat banner "Pengumuman baru" di dashboard kelas / bab

### 5.12 Admin — Bikin User Baru (password lifecycle)
1. Buka `/admin/pengguna` -> "Tambah Pengguna"
2. Isi nama, email, role
3. Pilih cara set password:
   - **Ketik manual**: input langsung
   - **Generate**: klik tombol "Generate" -> isi otomatis 8 char acak
4. Submit -> backend bcrypt + simpan user dengan `MustChangePassword=true`, `Status=active`
5. **Modal sukses**: tampil sekali "Password user X: `aB3xY9zK`" + tombol copy + warning "tutup modal = password gak bisa dilihat lagi, harus reset"
6. Admin kasih tau user manual (chat, papan tulis, print)
7. User login pertama kali -> dapat redirect paksa ke `/me/security` ganti password

### 5.13 Admin — Bulk Import Siswa (CSV)
1. Buka `/admin/pengguna/import` -> upload CSV (kolom: nama, email)
2. Backend preview: validasi format, cek email duplicate, tampil tabel preview + jumlah valid/invalid
3. Admin klik "Confirm Import" -> backend create user satu-per-satu, generate password 8 char per siswa, simpan dengan `MustChangePassword=true`
4. Setelah selesai: ImportJob disimpan, **download file `credentials_<job_id>.csv`** dengan kolom (nama, email, password_awal). Admin distribute file ini ke wali kelas / siswa.
5. File credentials cuma bisa di-download SEKALI dari modal sukses — gak ada ulang setelah modal ditutup. Kalau ketinggalan, harus reset password individual.

### 5.14 User — Force Change Password (login pertama)
1. User login dengan password awal dari admin -> sukses
2. Backend response include `must_change_password: true`
3. Frontend redirect paksa ke `/me/security`
4. Form: password baru + konfirmasi (min 8 char, ada angka)
5. Submit -> backend update password hash + set `MustChangePassword=false`
6. Redirect ke dashboard sesuai role

### 5.15 Siswa — Lihat Review Jawaban Ulangan
1. Setelah submit Ulangan Bab/Harian, masuk ke halaman hasil
2. Cek setting `IzinkanReviewSetelahSubmit` + `WaktuBukaReview`:
   - Jika `IzinkanReviewSetelahSubmit=true`: review terbuka langsung, tampilkan tiap soal + jawaban siswa + jawaban benar + status (✓/✗)
   - Jika `WaktuBukaReview` set & sudah lewat: review terbuka
   - Else: cuma tampil "Skor: 80, review akan dibuka pada {WaktuBukaReview}" atau "Hubungi guru untuk review"

### 5.16 Guru — Duplicate Kelas (re-use untuk tahun ajaran baru)
1. Buka `/guru/kelas` -> klik tombol "Duplicate" di kelas existing
2. Modal: input nama kelas baru
3. Submit -> backend copy: kelas + bab (status=draft semua) + materi + soal bab + tugas (tanpa submission) + ulangan harian (tanpa hasil)
4. Kode invite kelas baru di-regenerate, enrollment kosong
5. Guru tinggal publish bab yang mau dipakai + invite siswa baru

### 5.17 Forgot Password (siswa lupa)
1. Siswa di halaman login klik "Lupa password?"
2. Halaman `/lupa-password` menampilkan: "Hubungi admin sekolah/guru wali kelas Anda untuk minta reset password. Setelah reset, Anda akan dapat password sementara dan wajib ganti pas login pertama."
3. Siswa chat admin, admin buka `/admin/pengguna` -> klik user -> "Reset Password" (flow sama dengan create) -> kasih tau siswa
4. (Self-service forgot password ditunda — perlu email kalau mau ada)

---

## 6. Data Model (GORM)

```go
User       { ID, Name, Email(unique), PasswordHash, Role(admin|guru|siswa), Status(active|suspended|locked), MustChangePassword(bool, default true), FailedLoginCount(int, default 0), LastFailedLoginAt(*), CreatedByID(*), LastLoginAt(*), CreatedAt, UpdatedAt }
Kelas      { ID, Nama, Deskripsi, KodeInvite(unique,6), GuruID, BobotSoalUlangan(default 50), BobotTugas(default 50), Version(int default 1), CreatedAt, ArchivedAt(*) }
Enrollment { KelasID, SiswaID, Status, JoinedAt, JoinedVia(admin|kode) }  // PK composite
Bab        { ID, KelasID, Nomor, Judul, Deskripsi, Urutan, Status(draft|published|archived, default draft), Version(int default 1), CreatedAt, ArchivedAt(*) }
Materi     { ID, KelasID, BabID(*), Judul, Tipe, Konten, FilePath, Urutan, CreatedAt }
MateriRead { MateriID, SiswaID, ReadAt }                              // PK composite
Tugas      { ID, KelasID, BabID(*), Judul, Deskripsi, Deadline, MaxNilai, AttachmentPath, IzinkanLate(bool), PenaltyPersen(int 0-100), CreatedAt }
Submission { ID, TugasID, SiswaID, Konten, AttachmentPath, SubmittedAt, IsLate(bool), Nilai(*), NilaiSetelahPenalty(*), Feedback, GradedAt(*), Version }

// Soal Bab + gambar
SoalBab    { ID, BabID, Pertanyaan, GambarSoal(*), OpsiA..E(*), GambarOpsiA..E(*), JawabanBenar, Poin, Mode(latihan|ulangan), Urutan, Version(int default 1), CreatedAt }
UlanganBabSetting { BabID(PK), DurasiMenit, MulaiAt(*), SelesaiAt(*), ShuffleSoal, ShuffleOpsi, JumlahSoalRandom(*), TampilkanNilaiLangsung, IzinkanReviewSetelahSubmit(default false), WaktuBukaReview(*), Aktif, Version(int default 1) }
HasilSoalBab { ID, BabID, SiswaID, Mode(latihan|ulangan), AttemptKe, MulaiAt, SubmitAt(*), TotalNilai(*), Status(berlangsung|submitted|expired), DeletedAt(*) }
JawabanBab   { ID, HasilSoalBabID, SoalBabID, JawabanSiswa(*), Benar, Poin }
EventBab     { ID, HasilSoalBabID, Tipe(tab_switch|blur|focus|paste), At }

// Ulangan Harian + Soal bisa pakai gambar juga
Ujian      { ID, KelasID, Judul, DurasiMenit, MulaiAt, SelesaiAt, ShuffleSoal, ShuffleOpsi, JumlahSoalRandom(*), TampilkanNilaiLangsung, IzinkanReviewSetelahSubmit(default false), WaktuBukaReview(*), Version(int default 1), CreatedAt }
Soal       { ID, GuruID(pemilik bank), UjianID(*), Pertanyaan, GambarSoal(*), OpsiA..E(*), GambarOpsiA..E(*), JawabanBenar, Poin, Topik, Version(int default 1), CreatedAt }
UjianSoal  { UjianID, SoalID, Urutan }                                // PK composite
HasilUjian { ID, UjianID, SiswaID, MulaiAt, SubmitAt(*), TotalNilai(*), Status(berlangsung|submitted|expired), DeletedAt(*) }
Jawaban    { ID, HasilUjianID, SoalID, JawabanSiswa(*), Benar, Poin }
EventUjian { ID, HasilUjianID, Tipe, At }

// Ulangan attempt assignment (untuk random pool — soal mana yang dikasih ke siswa mana, deterministik)
HasilSoalBabSoalAssignment { HasilSoalBabID, SoalBabID, Urutan }      // PK composite
HasilUjianSoalAssignment   { HasilUjianID, SoalID, Urutan }           // PK composite

// Pengumuman per kelas / per bab
Pengumuman { ID, KelasID, BabID(*), Judul, Isi(markdown), CreatedByID, CreatedAt }

AuditLog   { ID, ActorID(*), ActorRole(*), Action, TargetType, TargetID(*), TargetKelasID(*), Meta(jsonb), IP(*), UserAgent(*), At }
LoginAttempt { ID, Email, IP, UserAgent, Success(bool), Reason(*), At }
ImportJob  { ID, AdminID, Filename, Status(preview|processing|completed|expired|failed), TotalRows, ValidCount, InvalidCount, SuccessCount, FailCount, PreviewRowsJSON(jsonb), ErrorsJSON(jsonb), CredentialsCSV(*), ExpiresAt, CreatedAt, ConfirmedAt(*), CompletedAt(*) }
RefreshToken { ID, JTI(unique), UserID, IssuedAt, ExpiresAt, RevokedAt(*), RevokedReason(*), IP(*), UserAgent(*), ReplacedByJTI(*) }
```

`(*)` = nullable.

### 6.1 Catatan model

- **User**: `Status` tambah `locked` (akun di-lock karena terlalu banyak gagal login). `MustChangePassword` default `true` saat create — set `false` setelah user ganti password sendiri. `FailedLoginCount` di-increment per gagal login (per email). `LastFailedLoginAt` untuk window rate limit.
- **Bab.Status**: `draft` (default, siswa gak lihat), `published` (siswa lihat), `archived` (siswa gak lihat lagi). Beda dari `ArchivedAt` — `Status=archived` adalah workflow guru, `ArchivedAt` adalah hard archive. Untuk konsistensi, **gabung jadi 1**: enum `Status(draft|published|archived)`, tanpa `ArchivedAt` di Bab. Kelas tetap pakai `ArchivedAt`.
- **MateriRead**: dipakai untuk progress per bab di sisi siswa. Auto-insert pas siswa buka viewer materi.
- **Tugas**: `IzinkanLate` default false. `PenaltyPersen` 0-100, jadi nilai max submission late = `MaxNilai × (100 - PenaltyPersen) / 100`.
- **Submission**: `Version` increment tiap resubmit; baris terbaru saja yang dipake (atau pakai 1 row dengan overwrite). Default: **1 row, overwrite** — hemat storage. `IsLate` di-set saat submit, `NilaiSetelahPenalty` dihitung backend pas grading.
- **SoalBab/Soal**: gambar disimpan sebagai path relatif di `./storage/uploads/soal/<uuid>.jpg`. Gambar opsi opsional (untuk soal "pilih gambar").
- **HasilSoalBab.Status**:
  - `berlangsung`: siswa udah start, belum submit. Inilah state yang dipake recovery resume.
  - `submitted`: siswa udah submit normal.
  - `expired`: timer habis, auto-submit.
- **HasilSoalBab.DeletedAt** + **HasilUjian.DeletedAt**: dipakai untuk remedial / reset attempt — soft delete supaya audit trail tetap ada. Constraint unique untuk mode=ulangan harus di-update jadi partial: `WHERE deleted_at IS NULL`.
- **HasilSoalBabSoalAssignment / HasilUjianSoalAssignment**: snapshot soal mana yang ditugaskan ke attempt itu, beserta urutan shuffle. Dibikin saat `start`. Penting untuk: (1) konsistensi soal saat resume, (2) random pool tetap deterministik per attempt, (3) review jawaban setelah submit pakai data ini.
- **EventBab**: tabel terpisah dari `EventUjian`, sama bentuk, biar bersih.
- **Pengumuman**: `BabID` nullable — kalau diisi, pengumuman cuma muncul di bab tsb.
- **AuditLog**: untuk audit trail aksi admin/guru. `ActorID` nullable supaya bisa log "system reset" atau aksi otomatis.
- **LoginAttempt**: tabel terpisah dari `AuditLog` karena volume tinggi & query pattern beda. Cleanup periodic (retain 30 hari).
- **ImportJob.CredentialsCSV**: path file sementara, di-cleanup setelah modal sukses ditutup atau timeout 1 jam.
- **ImportJob lifecycle (locked #54)**: `preview` (PreviewRowsJSON di-populate, file di disk, ExpiresAt = now+1h) → `processing` (admin confirm, sedang insert users) → `completed` (CredentialsCSV ready). Cleanup job hourly: kalau status=preview & ExpiresAt<now → status=expired + delete file. Failed insert → status=failed + ErrorsJSON.
- **Version field (locked #56)**: di Bab/Kelas/SoalBab/Soal/UlanganBabSetting/Ujian — optimistic concurrency. PATCH wajib include `version`. Backend: `UPDATE ... SET version=version+1 WHERE id=? AND version=?`. Affected rows=0 → return 409 + `current_version`. UI tampil "Konten ini diubah orang lain — refresh dulu".
- **RefreshToken**: tabel khusus refresh token tracking. Access token tetap stateless JWT 15m (gak di-store). Refresh token issued saat login, jti random UUID, simpan hash-nya di body JWT + DB row. Saat refresh: cek `revoked_at IS NULL` & `expires_at > now`, lalu rotate (mark old `revoked_at=now`, `replaced_by_jti=new_jti`, issue new token). Detection token reuse: kalau revoked token dipake lagi → revoke semua chain user (suspicious). `RevokedReason`: `logout`, `rotate`, `password_changed`, `admin_reset`, `user_locked`, `user_suspended`, `reuse_detected`.
- **Recovery resume logic**: server cek `HasilSoalBab` / `HasilUjian` dengan `Status=berlangsung` & `DeletedAt IS NULL` untuk siswa tsb -> kalau ada, banner "lanjutkan ulangan" muncul. Soal yang ditampilkan re-fetch dari `*SoalAssignment` (deterministik).
- **Submit transition (locked #43)**: dari `berlangsung → submitted/expired` wajib dalam transaction dengan `SELECT ... FOR UPDATE` di row HasilSoalBab/HasilUjian + cek `Status='berlangsung'` sebelum update. Auto-grade jalan dalam transaction yang sama. Idempotent: status udah final → return existing TotalNilai, no re-grade. Background timer-expire job pakai pg advisory lock per row.

### 6.2 Formula Nilai Bab (per siswa)

```
NilaiUlanganBab = TotalNilai dari HasilSoalBab(mode=ulangan, deleted_at IS NULL) terakhir untuk (BabID, SiswaID)
                  -> normalize ke skala 0-100 = (TotalNilai / SUM(SoalBab.Poin where Mode=ulangan)) × 100
                  -> kalau gak ada soal ulangan / belum dikerjain: NULL

NilaiTugasBab   = AVG(Submission.NilaiSetelahPenalty) untuk semua Tugas dengan BabID = bab tsb dan SiswaID
                  (di-skala ke 0-100 per tugas: NilaiSetelahPenalty / MaxNilai × 100)
                  -> kalau gak ada tugas / belum dinilai: NULL

NilaiBab = weighted_avg(NilaiUlanganBab, NilaiTugasBab,
                        weights = (Kelas.BobotSoalUlangan, Kelas.BobotTugas),
                        skip NULL components)
```

Catatan: kalau `IsLate=true` dan `PenaltyPersen=20`, `NilaiSetelahPenalty = Nilai × 0.80`. Kalau `IsLate=false`, `NilaiSetelahPenalty = Nilai`.

Contoh:
- Bobot kelas: SoalUlangan=60, Tugas=40
- NilaiUlanganBab=80, NilaiTugasBab=90 -> (80×60 + 90×40)/100 = 84
- NilaiUlanganBab=80, NilaiTugasBab=NULL -> 80 (bobot tugas di-skip)
- NilaiUlanganBab=NULL, NilaiTugasBab=NULL -> NULL ("-")

### 6.3 Indexes penting
- `enrollment(kelas_id, siswa_id)` PK
- `bab(kelas_id, urutan)` index
- `bab(kelas_id, status)` index (filter siswa: where status='published')
- `materi(kelas_id, bab_id, urutan)` index
- `materi_read(materi_id, siswa_id)` PK
- `tugas(kelas_id, bab_id)` index
- `soal_bab(bab_id, mode, urutan)` index
- `hasil_soal_bab(bab_id, siswa_id, mode)` — partial unique untuk `mode='ulangan' AND deleted_at IS NULL` (1 attempt aktif only)
- `hasil_soal_bab_soal_assignment(hasil_soal_bab_id, soal_bab_id)` PK
- `hasil_ujian(ujian_id, siswa_id)` — partial unique untuk `deleted_at IS NULL`
- `hasil_ujian_soal_assignment(hasil_ujian_id, soal_id)` PK
- `kelas(kode_invite)` unique
- `pengumuman(kelas_id, created_at DESC)` index
- `login_attempt(email, at DESC)` index (untuk rate limit query)
- `login_attempt(ip, at DESC)` index (untuk per-IP throttling)
- `users(email)` unique
- `users(status)` index (filter aktif/locked/suspended)
- `audit_log(actor_id, at DESC)` index
- `refresh_token(jti)` unique
- `refresh_token(user_id, revoked_at)` index (cepat revoke-all-by-user)
- `refresh_token(expires_at)` index (cleanup job)
- `import_job(admin_id, status, expires_at)` index (cleanup query + admin filter)
- `audit_log(target_kelas_id, at DESC)` index (untuk guru audit scope #59 — tambah column `target_kelas_id` nullable di AuditLog kalau action terkait kelas)

### 6.4 Formula Progress Bab (per siswa, locked #48)

```
komponen   bobot_default   pct
materi     25              materi_dibaca / total_materi
latihan    25              1 if exists HasilSoalBab(mode=latihan, status=submitted) else 0
ulangan    25              1 if exists HasilSoalBab(mode=ulangan, status IN (submitted,expired), deleted_at IS NULL) else 0
tugas      25              count(submission graded) / count(tugas di bab itu)

Rule:
- Komponen yang gak punya konten (mis. bab tanpa tugas) bobotnya di-drop, total bobot re-normalize.
- Kalau total konten 0 (bab kosong total) → progress 0% atau "—" di UI.
- Hasil: integer 0-100. Round half-up.

Contoh:
- Bab punya 3 materi (siswa baca 2), 1 ulangan (selesai), 0 tugas, 0 latihan
  → komponen aktif: materi+ulangan, bobot 50/50
  → progress = 0.5 × (2/3) + 0.5 × 1 = 0.833 ≈ 83%
```

API: `GET /siswa/kelas/:id/bab` returns `progress: { persen, breakdown: { materi: {pct, w}, latihan: {pct, w}, ulangan: {pct, w}, tugas: {pct, w} } }`.

---

## 7. API Endpoints (`/api/v1`)

### Health & Readiness
- `GET /healthz` — liveness, return 200 selalu kalau process hidup. No DB, no deps. Dipake systemd / load balancer dasar.
- `GET /readyz` — readiness, cek DB ping + storage dir writable. Return 503 kalau ada yang fail. Dipake uptime monitor.

### Auth
- `POST /auth/login` { email, password } -> { access, refresh, user: { id, name, email, role, status, must_change_password } }
- `POST /auth/refresh` { refresh } — rotate token, mark old revoked, issue new pair. Reuse detection: kalau token udah revoked dipake → revoke semua refresh chain user.
- `POST /auth/logout` — revoke current refresh token (`revoked_reason='logout'`)
- `POST /auth/logout-all` — revoke semua refresh token user (logout dari semua device)
- `GET  /auth/me`
- `POST /auth/change-password` { old_password, new_password } — set `must_change_password=false`, revoke semua refresh token user kecuali current (opsional, default revoke all biar aman)
- `GET  /auth/sessions` — list active refresh tokens user (jti masked, ip, user_agent, issued_at, last_used_at) untuk halaman "Perangkat aktif"

> **No `/auth/register`** — semua user dibuat oleh admin (lihat Section 5.0a/5.12).
> **No public `/auth/forgot-password`** — siswa hubungi admin untuk reset (lihat Section 5.17).
> **Rate limit middleware**: `/auth/login` di-throttle 5 gagal/15 menit per (IP, email). 10 gagal kumulatif → akun `locked`.
> **Force change password gate**: middleware cek `must_change_password=true` → block semua endpoint kecuali `/auth/me`, `/auth/change-password`, `/auth/logout`.

### Admin (`/admin/*`, role=admin only)
- `GET    /admin/users` (paginated, filter role/status, search)
- `POST   /admin/users` { name, email, role, password? } — kalau password kosong, backend generate 8 char acak. Response: `{ user, generated_password? }` (cuma muncul kalau backend yang generate atau admin minta show). Kalau `role=admin` saat create → wajib `current_password` di body (re-auth).
- `GET    /admin/users/:id`
- `PATCH  /admin/users/:id` { name?, email?, status? } — gak include `role`, role pindah ke endpoint khusus.
- `POST   /admin/users/:id/role` { role, current_password } — promote/demote, wajib re-auth admin yang melakukan. Tolak kalau target=actor & role=admin & ini admin terakhir (cegah lock-out).
- `POST   /admin/users/:id/reset-password` { password? } — sama logic dengan create
- `POST   /admin/users/:id/unlock` (kalau status=locked karena failed login)
- `DELETE /admin/users/:id` (hard delete, hati-hati — cuma kalau gak ada referensi data)
- `GET    /admin/users/:id/sessions` — list refresh token aktif user (untuk panel admin)
- `POST   /admin/users/:id/revoke-sessions` — revoke semua refresh token user (force logout)
- `POST   /admin/import-csv/upload` (multipart) → ImportJob status=`preview`, response `{ job_id, valid_count, invalid_count, preview_rows }`
- `GET    /admin/import-csv/:job_id` — resume preview (kalau admin reload page sebelum confirm) — return preview_rows + counts
- `POST   /admin/import-csv/:job_id/confirm` — status preview → processing → completed, response `{ job_id, success_count, fail_count, errors }`
- `POST   /admin/import-csv/:job_id/cancel` — status preview → expired + cleanup file
- `GET    /admin/import-csv/template.csv`
- `GET    /admin/import-jobs/:id/credentials.csv` — sekali download (file di-cleanup setelah)
- `POST   /admin/users/:id/enroll` { kelas_id }
- `GET    /admin/audit-log` (filter actor/action/target/date)
- `GET    /admin/login-attempts` (filter email/ip/success/date)
- `GET    /admin/stats` { user_counts, kelas_counts, locked_accounts }
- `GET    /admin/kelas` (read-only list semua kelas)

### Kelas (guru)
- `POST   /kelas`
- `GET    /kelas`
- `GET    /kelas/:id`
- `PATCH  /kelas/:id` { nama?, deskripsi?, bobot_soal_ulangan?, bobot_tugas? }
- `DELETE /kelas/:id` (soft archive)
- `GET    /kelas/:id/siswa`
- `DELETE /kelas/:id/siswa/:siswaId`

### Bab (guru — owner kelas)
- `POST   /kelas/:id/bab` { nomor, judul, deskripsi }
- `GET    /kelas/:id/bab` -> list bab + counter (jumlah materi/soal/tugas)
- `GET    /bab/:id`
- `PATCH  /bab/:id` { nomor?, judul?, deskripsi?, urutan?, status? } — transisi `draft|published|archived`
- `DELETE /bab/:id` (cascade: materi/tugas yang BabID-nya = bab ini di-set null, atau ditolak kalau ada hasil — saran: gunakan `Status=archived` instead)
- `POST   /kelas/:id/bab/reorder` { ordered_ids[] } -> bulk update urutan
- `POST   /bab/:id/duplicate` -> bikin bab baru status=draft + copy materi/soal/tugas

### Kelas (guru) — Duplicate
- `POST   /kelas/:id/duplicate` { nama_baru } -> bikin kelas baru + copy bab/materi/soal/tugas/ulangan (no enrollment, no submission, no hasil)

### Kelas (siswa)
- `GET  /siswa/kelas` -> list kelas yang siswa ikuti
- `POST /kelas/join` { kode } -> join kelas via kode invite

### Bab (siswa)
- `GET /siswa/kelas/:id/bab` -> list bab WHERE status='published' + progress per bab (materi dibaca, latihan, ulangan bab status, nilai bab)
- `GET /siswa/bab/:id` -> detail bab + tab data (materi, latihan summary, tugas list, hasil) — return 404 kalau bukan published

### Materi
- `POST   /kelas/:id/materi` (multipart kalau pdf, body bisa include `bab_id?`)
- `GET    /kelas/:id/materi` (filter: `?bab_id=X` atau `?bab_id=null` untuk yang bebas)
- `GET    /materi/:id`
- `PATCH  /materi/:id` { ..., bab_id? }
- `DELETE /materi/:id`
- `POST   /materi/:id/read` (siswa, mark as read — idempotent)

### Pengumuman
- `POST   /kelas/:id/pengumuman` (guru) { judul, isi, bab_id? }
- `GET    /kelas/:id/pengumuman` (guru + siswa enrolled, paginated, default DESC)
- `GET    /pengumuman/:id`
- `PATCH  /pengumuman/:id` (guru pemilik)
- `DELETE /pengumuman/:id`

### Soal Bab (guru)
- `POST   /bab/:id/soal` (multipart: { pertanyaan, opsi[], jawaban_benar, poin, mode, gambar_soal?, gambar_opsi[]? })
- `GET    /bab/:id/soal` (filter: `?mode=latihan|ulangan`)
- `PATCH  /soal-bab/:id`
- `DELETE /soal-bab/:id`
- `POST   /bab/:id/soal/bulk` (paste banyak soal sekaligus, opsional)
- `GET    /bab/:id/ulangan-setting`
- `PUT    /bab/:id/ulangan-setting` { durasi_menit, mulai_at?, selesai_at?, shuffle_soal, shuffle_opsi, jumlah_soal_random?, tampilkan_nilai_langsung, izinkan_review_setelah_submit, waktu_buka_review?, aktif }
- `GET    /bab/:id/ulangan/preview` (guru — render persis kayak siswa, mode read-only)

### Soal Bab (siswa)
- `GET    /siswa/bab/:id/soal/latihan` -> list soal mode=latihan + status attempt terakhir
- `POST   /siswa/bab/:id/soal/latihan/start` -> { hasil_id, soal[] sesuai shuffle }
- `POST   /hasil-soal-bab/:id/answer` { soal_id, jawaban }
- `POST   /hasil-soal-bab/:id/submit` -> { total_nilai, breakdown jawaban benar/salah }
- `GET    /siswa/bab/:id/ulangan` -> info ulangan bab (durasi, status: belum/dikerjain/selesai, sisa waktu kalau resume)
- `POST   /siswa/bab/:id/ulangan/start` -> { hasil_id, soal[], sisa_detik }   // ditolak kalau sudah pernah submit
- `GET    /hasil-soal-bab/:id/resume` -> { soal[], jawaban_tersimpan[], sisa_detik }   // dipake siswa saat reload page
- `POST   /hasil-soal-bab/:id/event` { tipe } (anti-cheat log untuk ulangan bab)

### Hasil Bab (guru)
- `GET /bab/:id/hasil` -> rekap kelas (siswa × ulangan bab nilai + tab-switch count)
- `POST /bab/:id/hasil/:siswaId/reset` { reason } (guru — remedial; soft-delete HasilSoalBab + JawabanBab + audit log dengan reason)
- `GET /siswa/hasil-soal-bab/:id/review` (siswa, kalau IzinkanReviewSetelahSubmit=true atau WaktuBukaReview lewat — return list soal + jawaban siswa + jawaban benar + status)

### Tugas
- `POST   /kelas/:id/tugas` { ..., bab_id?, izinkan_late, penalty_persen }
- `GET    /kelas/:id/tugas` (filter: `?bab_id=X`)
- `GET    /tugas/:id`
- `PATCH  /tugas/:id`
- `DELETE /tugas/:id`
- `POST   /tugas/:id/submit` (siswa, multipart) — auto-overwrite kalau udah pernah submit & belum graded; reject kalau lewat deadline & gak izinin late
- `GET    /siswa/tugas/:id/submission` -> submission siswa sendiri (untuk pre-fill form resubmit)
- `GET    /tugas/:id/submissions` (guru)
- `POST   /submission/:id/grade` (guru) — backend hitung NilaiSetelahPenalty otomatis
- `GET    /siswa/submissions` (siswa)

### Ulangan Harian (lintas bab)
- `POST   /kelas/:id/ujian`
- `GET    /kelas/:id/ujian`
- `GET    /ujian/:id`
- `PATCH  /ujian/:id` (termasuk `izinkan_review_setelah_submit`, `waktu_buka_review`)
- `DELETE /ujian/:id`
- `POST   /ujian/:id/duplicate` -> bikin salinan dengan nama baru, status reset
- `GET    /ujian/:id/preview` (guru — read-only)
- `POST   /ujian/:id/start` (siswa) -> { hasil_id, soal[], sisa_detik }
- `GET    /ujian/:id/play`
- `GET    /hasil-ujian/:id/resume` -> { soal[], jawaban_tersimpan[], sisa_detik }
- `POST   /hasil-ujian/:id/answer`
- `POST   /hasil-ujian/:id/submit`
- `POST   /hasil-ujian/:id/event`
- `GET    /siswa/hasil-ujian/:id/review` (siswa, kalau review terbuka)
- `GET    /ujian/:id/hasil` (guru)
- `POST   /ujian/:id/hasil/:siswaId/reset` { reason } (guru — remedial)

### Sesi Aktif (untuk recovery banner di dashboard)
- `GET /siswa/active-assessments` -> list HasilSoalBab/HasilUjian dengan `Status=berlangsung` -> banner "Lanjutkan ulangan" di dashboard

### Guru — Dashboard Activity & Counters
- `GET /guru/feed?cursor=BASE64&limit=20` — opaque cursor pagination `(at_unix_micro, id)`. Response: `{ events: [...], next_cursor }`. Polling 30s pake `cursor=null` (latest 20).
- `GET /guru/pending-counts` -> `{ ungraded_submissions, pending_review_ulangan_bab, pending_review_ulangan_harian }`
- `GET /guru/kelas/:id/audit?action=<filter>&limit=50` — guru audit scope (subset action: `hasil_reset`, `bab_archived`, `bab_published`, `siswa_kicked`, `tugas_deleted`). Hanya entry dengan `target_kelas_id=<id>`.

### Bank Soal (guru) — untuk Ulangan Harian
- `POST   /bank-soal`
- `GET    /bank-soal` (filter: topik)
- `PATCH  /bank-soal/:id`
- `DELETE /bank-soal/:id`

### Nilai (transparansi siswa)
- `GET /siswa/kelas/:id/nilai` -> per kelas:
  - `bab[]`: { id, nomor, judul, nilai_ulangan_bab, nilai_tugas_bab, nilai_bab, breakdown }
  - `ulangan_harian[]`: { id, judul, nilai }
  - `total_kelas`: rata-rata semua bab (skip NULL)
- `GET /guru/kelas/:id/rekap-nilai` -> tabel siswa × bab + ulangan harian (read-only matrix)

### Export
- `GET /kelas/:id/nilai/export` (CSV: kolom = siswa, bab1, bab2, ..., ulangan_harian, total)

---

## 8. Routes / Screens (Next.js)

### Public
- `/` Landing (1 CTA: Masuk)
- `/login`
- `/lupa-password` (instruksi "hubungi admin", no form)
- ~~`/register`~~ tidak ada — semua akun dibuat oleh admin

### Self (semua role yang sudah login)
- `/me` Profil (nama, email, ganti password)
- `/me/security` Ganti password (force redirect kalau `MustChangePassword=true`)

### Admin (`/admin/*`, role=admin only)
- `/admin` Dashboard (stats: total guru/siswa, kelas aktif, ujian berlangsung, locked accounts shortcut, audit log ringkas)
- `/admin/pengguna` List user (filter role, search, badge status)
- `/admin/pengguna/baru` Create user (input nama+email+role + password manual atau Generate -> modal sukses tampilkan password sekali)
- `/admin/pengguna/[id]` Detail user (edit, reset password, suspend, unlock, enrollment list, riwayat login)
- `/admin/pengguna/import` Bulk import CSV (upload -> preview -> confirm -> download credentials.csv sekali)
- `/admin/kelas` List semua kelas (read-only, lihat siapa guru-nya, jumlah siswa)
- `/admin/audit-log` Riwayat aksi admin/guru
- `/admin/login-attempts` Riwayat login attempts (success + fail)

### Guru (`/guru/*`)
- `/guru` Dashboard — activity feed (polling 30s) + pending counters di sidebar (badge "12 belum dinilai")
- `/guru/kelas` List + tombol Duplicate per kelas
- `/guru/kelas/[id]` Detail (tabs: Bab / Siswa / Tugas / Ulangan Harian / Pengumuman / Rekap Nilai / Pengaturan)
- `/guru/kelas/[id]/bab/baru` Form bikin bab
- `/guru/kelas/[id]/bab/[bid]` Detail bab (tabs: Materi / Soal / Tugas / Pengumuman / Pengaturan Ulangan Bab) — header tampil status badge `draft|published|archived` + tombol Publish/Unpublish + Duplicate
- `/guru/kelas/[id]/bab/[bid]/materi/baru`
- `/guru/kelas/[id]/bab/[bid]/soal/editor` Editor soal (latihan + ulangan bab) + upload gambar
- `/guru/kelas/[id]/bab/[bid]/ulangan/preview` Preview ulangan bab (read-only)
- `/guru/kelas/[id]/bab/[bid]/hasil` Rekap ulangan bab + tombol Reset Attempt (modal: input reason)
- `/guru/kelas/[id]/tugas/baru` (pilih bab di form, atau "tanpa bab", set izinkan late + penalty)
- `/guru/kelas/[id]/tugas/[tid]/submissions` (badge LATE pada submission yang IsLate)
- `/guru/kelas/[id]/ulangan/baru` (Ulangan Harian lintas bab) + setting review
- `/guru/kelas/[id]/ulangan/[uid]/edit`
- `/guru/kelas/[id]/ulangan/[uid]/preview` Preview ulangan harian (read-only)
- `/guru/kelas/[id]/ulangan/[uid]/hasil` Rekap + tombol Reset Attempt (modal reason) + Duplicate
- `/guru/kelas/[id]/pengumuman` List + bikin baru
- `/guru/kelas/[id]/rekap-nilai` Matrix siswa × bab + ulangan harian
- `/guru/kelas/[id]/pengaturan` Bobot nilai bab (Soal vs Tugas) + archive kelas + Duplicate
- `/guru/bank-soal` (CRUD bank soal pribadi + upload gambar)

### Siswa (`/siswa/*`)
- `/siswa` Dashboard (kelas + tombol "Gabung Kelas" + banner "Lanjutkan ulangan" kalau ada sesi berlangsung)
- `/siswa/gabung` Form input kode kelas
- `/siswa/kelas/[id]` Detail kelas — list bab (cuma published) dengan progress + section "Ulangan Harian" + section "Pengumuman"
- `/siswa/kelas/[id]/bab/[bid]` Detail bab (tabs: Materi / Latihan / Tugas / Hasil)
- `/siswa/kelas/[id]/bab/[bid]/materi/[mid]` Viewer materi (auto-call mark-read)
- `/siswa/kelas/[id]/bab/[bid]/latihan` Halaman kerjain soal latihan (retry)
- `/siswa/kelas/[id]/bab/[bid]/ulangan` Lobby ulangan bab (tampil "Lanjutkan" kalau ada sesi berlangsung)
- `/siswa/kelas/[id]/bab/[bid]/ulangan/play` Halaman fokus kerjain ulangan bab — auto-resume kalau ada session
- `/siswa/kelas/[id]/bab/[bid]/ulangan/review` Review jawaban setelah submit (kalau guru izinin)
- `/siswa/kelas/[id]/tugas/[tid]` Submit tugas — pre-fill kalau udah pernah submit + warning "Late penalty xx%" kalau lewat deadline & izinin late
- `/siswa/kelas/[id]/ulangan-harian/[uid]` Lobby ulangan harian
- `/siswa/kelas/[id]/ulangan-harian/[uid]/play` Kerjain ulangan harian — auto-resume
- `/siswa/kelas/[id]/ulangan-harian/[uid]/review` Review jawaban setelah submit
- `/siswa/kelas/[id]/nilai` Transparansi nilai per kelas — list bab + breakdown + total + ulangan harian
- `/siswa/nilai` Rekap nilai lintas kelas (semua kelas yg diikuti)

Karena static export, semua dynamic routes pakai `generateStaticParams` kalau perlu pre-render, atau di-handle full client-side dengan route group + `useParams` + fetch.

---

## 9. Project Structure

```
lms/
├── backend/                  # Go API
│   ├── cmd/
│   │   ├── server/main.go        # API server (Fiber)
│   │   ├── seed-admin/main.go    # CLI bootstrap admin pertama
│   │   └── reset-admin/main.go   # CLI reset password admin (kalau lupa)
│   ├── internal/
│   │   ├── auth/             # login, JWT, change-password, middleware
│   │   ├── admin/            # user CRUD, CSV import, audit log
│   │   ├── user/             # user model + repo
│   │   ├── kelas/
│   │   ├── enrollment/
│   │   ├── bab/              # Bab CRUD + reorder
│   │   ├── materi/
│   │   ├── tugas/
│   │   ├── soalbab/          # SoalBab + UlanganBabSetting + HasilSoalBab
│   │   ├── ujian/            # Ulangan Harian (lintas bab) + bank soal
│   │   ├── nilai/            # formula nilai bab + rekap + export CSV
│   │   ├── audit/            # audit log writer
│   │   ├── middleware/       # auth, role guard, logging, recover
│   │   ├── storage/          # file upload helper
│   │   └── db/               # GORM setup, migrations
│   ├── pkg/                  # shared utils (jwt, hash, validator, csv)
│   ├── go.mod
│   └── go.sum
├── frontend/                 # Next.js (static export)
│   ├── app/
│   │   ├── (auth)/login/
│   │   ├── admin/
│   │   ├── guru/
│   │   ├── siswa/
│   │   ├── me/
│   │   ├── layout.tsx
│   │   └── page.tsx          # landing
│   ├── components/
│   │   ├── ui/               # shadcn
│   │   ├── bab/
│   │   ├── soal/
│   │   └── ...
│   ├── lib/
│   │   ├── api.ts            # fetch wrapper + token refresh
│   │   ├── auth.ts           # token store (Zustand)
│   │   └── utils.ts
│   ├── next.config.js        # output: 'export'
│   ├── package.json
│   └── ...
├── storage/uploads/          # gitignored, runtime files
├── docs/
│   ├── DEPLOY.md             # runbook (mirip fb-bot)
│   └── ARCHITECTURE.md
├── deploy/
│   ├── deploy.sh
│   └── systemd/lms-api.service
├── .kiro/steering/           # plan + state
├── .env.example
├── LOCAL_AI_CONTEXT.md       # quick context buat AI sessions
├── README.md
└── .gitignore
```

---

## 10. Phasing / Roadmap

### Fase 0 — Setup (1-2 hari)
- Init repo Git, struktur folder
- Backend: `go mod init`, Fiber, GORM connect Postgres, **golang-migrate setup** (migrations dir + initial migration), healthcheck `/api/v1/healthz` (liveness, no DB) + `/api/v1/readyz` (readiness, cek DB + storage)
- **Request ID middleware** (UUID v4 atau ambil dari header), propagate ke slog context (`request_id`, `user_id`, `path`, `method`)
- **Global rate limit middleware** (Fiber `limiter` 120 req/menit per IP)
- Lock timezone server: `time.LoadLocation("Asia/Jakarta")` + `time.Local` di main.go
- Frontend: `create-next-app`, Tailwind+shadcn (new-york), halaman login stub, `output: 'export'`
- Adopt design baseline (warna, font — pakai `ui-ux-pro-max` skill)
- Build dan test Go serve `frontend/out/` di port 8200
- systemd unit & deploy.sh draft di `deploy/` (`ExecStartPost` curl readyz)
- Push ke GitHub, clone ke rdpkhorur, smoke test via SSH tunnel
- Bikin `LOCAL_AI_CONTEXT.md`, `docs/DEPLOY.md`, `README.md`
- Bikin `cmd/seed-admin` CLI (lihat Section 17)
- Bikin `cmd/reset-admin` CLI (emergency reset password admin)
- CI gate setup: `go test -cover ./...` minimal 70% target (initially loose, ketat tiap fase nambah)

### Fase 1 — Auth & Admin Panel (4-5 hari)
- User model lengkap (role admin|guru|siswa, status active|suspended|locked, MustChangePassword, FailedLoginCount)
- **RefreshToken table** + repository (issue, rotate, revoke single, revoke-all-by-user, reuse detection)
- Login + JWT (access 15m stateless + refresh 7d stateful) + bcrypt cost 12 + change-password
- **Refresh rotation flow**: tiap refresh → mark old jti `revoked_at`, issue new jti, update `replaced_by_jti`
- **Reuse detection**: kalau token revoked dipake → revoke all chain user + audit log `reuse_detected`
- **Auto-revoke triggers**: suspend / lock / change-password / admin reset → revoke all refresh tokens user
- **Rate limit middleware** untuk `/auth/login` (5 gagal/15 menit per IP+email, in-memory)
- **Rate limit `/auth/refresh`** (10/menit per refresh token)
- **Lockout**: 10 gagal kumulatif -> Status=locked
- **ForceChangePassword middleware** — block semua endpoint kecuali `/auth/me`, `/auth/change-password`, `/auth/logout` kalau MustChangePassword=true
- LoginAttempt logging (success + fail)
- AuditLog writer infrastructure (dengan field `target_kelas_id` nullable)
- **Auth boundary middleware order**: ratelimit → request-id → auth → role-guard → enrollment-guard. Whitelist anon: `/auth/login`, `/auth/refresh`, `/healthz`, `/readyz`, static.
- Middleware: auth + role guard (admin/guru/siswa) + enrollment-guard untuk endpoint kelas-scope
- Admin endpoints: CRUD user (password manual atau generate), reset password (manual atau generate), unlock, suspend, enroll
- **Admin promote/demote**: `POST /admin/users/:id/role` — wajib re-auth (current_password). Tolak kalau bikin admin terakhir kena demote.
- Admin endpoints: audit log + login attempts list + user sessions + revoke-sessions
- Self endpoint: `GET /auth/sessions` + `POST /auth/logout-all`
- Frontend: login page, /lupa-password page (instruksi), /me + /me/security (force redirect kalau MustChangePassword) + /me/perangkat (list active sessions + tombol logout-all)
- Frontend admin panel: dashboard, pengguna list (filter, search) + create form (toggle manual/generate password, kalau pilih role=admin → modal re-auth) + modal sukses dengan tombol copy + reset/suspend/unlock + audit-log + login-attempts + detail user (riwayat sesi)
- Seed admin pertama via CLI (`cmd/seed-admin`) + `cmd/reset-admin` emergency
- E2E manual: bootstrap admin -> create akun guru & siswa -> login keduanya -> force change password -> dashboard -> verify suspend langsung kick session aktif -> verify promote butuh re-auth

### Fase 2 — Kelas, Enrollment & Bulk Import (3-4 hari)
- Backend: Kelas CRUD (guru) + bobot nilai (BobotSoalUlangan, BobotTugas) + generate kode invite unik + archive + **duplicate** + **Version field** (optimistic concurrency)
- Backend: Siswa join via kode (rate limit 10/menit per IP), tracking JoinedVia
- Backend: Admin assign siswa ke kelas
- Backend: **ImportJob lifecycle** — upload (status=preview, PreviewRowsJSON, ExpiresAt=now+1h), GET resume preview, confirm (preview→processing→completed), cancel (preview→expired), hourly cleanup expired jobs
- Backend: Bulk CSV import siswa (parser, validator) + **generate password per siswa + credentials.csv download sekali + auto-cleanup 1 jam**
- Backend: **Storage path convention** — `./storage/uploads/<kategori>/<uuid>.<ext>`, OriginalFilename di DB column terpisah
- Frontend admin: import CSV (drag-and-drop, preview tabel persistent — admin bisa close tab + balik tanpa lose state, confirm, modal sukses dengan link download credentials.csv), list kelas (read-only)
- Frontend guru: dashboard list+create kelas + tombol Duplicate, kelas detail (tab Siswa, tab Pengaturan/bobot, tab Pengumuman placeholder), edit form pakai version (409 handler "konten ke-update orang lain")
- Frontend siswa: dashboard, gabung kelas via kode

### Fase 3 — Bab & Materi + Pengumuman + Bab Status (3-4 hari)
- Backend: Bab CRUD (guru) + reorder bulk endpoint + **status (draft/published/archived)** + **Version field** (optimistic concurrency) + duplicate (copy materi/soal/tugas)
- Backend: Materi CRUD dengan field `bab_id` nullable (upload PDF, link YouTube, teks markdown) + **storage path `./storage/uploads/materi/<uuid>.<ext>` + OriginalFilename di DB**
- Backend: MateriRead endpoint (siswa mark-as-read)
- Backend: endpoint siswa list bab (cuma published) + detail bab dengan progress (formula 6.4)
- Backend: Pengumuman CRUD (per-kelas atau per-bab)
- Frontend guru:
  - Tab "Bab" di kelas detail: list bab dengan status badge, drag-and-drop urutan, create/edit/delete/archive/publish/unpublish/duplicate, edit form pakai version (409 → "konten ke-update orang lain, refresh dulu")
  - `/guru/kelas/[id]/bab/[bid]` shell dengan tabs (Materi / Soal placeholder / Tugas placeholder / Pengumuman / Pengaturan) + status badge di header
  - Tab Materi di bab: upload PDF, tambah link YouTube, tulis markdown
  - Tab Pengumuman per kelas + per bab
- Frontend siswa:
  - `/siswa/kelas/[id]` list bab published (urut, judul, deskripsi, **progress bar dengan tooltip breakdown** sesuai formula 6.4) + section pengumuman
  - `/siswa/kelas/[id]/bab/[bid]` detail bab dengan tab Materi (viewer + auto mark-read)
  - Materi viewer: PDF iframe, YouTube embed, react-markdown

> **Checkpoint:** Sebelum Fase 4, bedah notifikasi (v0.8).

### Fase 4 — Tugas (per bab) + Late + Resubmit (3-4 hari)
- Backend: Tugas CRUD dengan field `bab_id` nullable + `IzinkanLate` + `PenaltyPersen`
- Backend: Submission flow + grading + IsLate flag + NilaiSetelahPenalty calc
- Backend: Resubmit logic (overwrite kalau belum graded & belum lewat deadline)
- Backend: Reject submission kalau lewat deadline & gak izinin late
- Frontend guru: form bikin tugas (pilih bab, set late + penalty), tab Tugas di bab, review submissions (badge LATE), grading
- Frontend siswa: tab Tugas di bab + halaman submit (pre-fill kalau udah pernah submit), banner "Late submission akan kena penalty xx%"

### Fase 5 — Soal Bab (Latihan + Ulangan Bab) + Resume + Remedial + Random Pool + Review (5-6 hari)
- Backend: SoalBab CRUD per bab + bulk paste + **upload gambar soal & gambar opsi** (mime sniff, allowlist jpg/png/webp, resize max 1920px, simpan sebagai uuid, original name di DB)
- Backend: UlanganBabSetting (PUT per bab) — termasuk `JumlahSoalRandom`, `IzinkanReviewSetelahSubmit`, `WaktuBukaReview`
- Backend: HasilSoalBab + JawabanBab + EventBab + **HasilSoalBabSoalAssignment**
  - Latihan: start (bikin attempt baru + assignment soal sesuai shuffle), answer save, submit -> auto-grade, reveal jawaban benar
  - Ulangan Bab: start (cek belum pernah submit + status berlangsung, **random N dari pool kalau JumlahSoalRandom set**), server-side timer, answer auto-save
  - **Submit transition**: pakai `SELECT ... FOR UPDATE` + cek `Status='berlangsung'` di transaction, auto-grade dalam transaction yang sama, idempotent (status final → return existing)
  - **Resume**: GET `/hasil-soal-bab/:id/resume` untuk lanjut session berlangsung (re-fetch dari assignment)
  - **Remedial**: POST `/bab/:id/hasil/:siswaId/reset` { reason } — soft-delete attempt + assignment lama, **assignment baru fresh-snapshot** dari SoalBab aktif sekarang, audit log dengan `soal_diff` (added/removed IDs)
  - **Review**: GET `/siswa/hasil-soal-bab/:id/review` (cek IzinkanReviewSetelahSubmit + WaktuBukaReview)
  - Anti-cheat event log
- Backend: timer-expire background job (per row pg advisory lock, transition ke `expired` + auto-grade)
- Backend: GET /bab/:id/hasil (rekap guru)
- Backend: GET /bab/:id/ulangan/preview (guru — read-only render)
- Backend: GET /siswa/active-assessments (banner recovery di dashboard)
- Frontend guru: editor soal dengan image upload (preview thumbnail + warning kalau >5MB pre-resize), pengaturan ulangan bab (durasi, shuffle, jumlah random, review), halaman preview, halaman rekap hasil + tombol Reset Attempt (modal reason)
- Frontend siswa: tab Latihan (kerjain + retry + reveal), tab Ulangan Bab (lobby + play full screen + timer + resume), tab Hasil dengan link Review (kalau dibuka), banner di dashboard
- Test (TDD): auto-grade, **concurrency 1-attempt-only (parallel start request)**, **submit race (parallel submit + timer expire)**, resume after disconnect, remedial flow with soal_diff, random pool deterministik
- Coverage gate: package `soalbab` minimal 70%

### Fase 6 — Ulangan Harian (lintas bab) + Resume + Remedial + Duplicate + Review (4-5 hari)
- Backend: Bank Soal + Ujian + Soal + UjianSoal + HasilUjian + Jawaban + EventUjian + **HasilUjianSoalAssignment**
- Backend: Bank Soal CRUD (guru) + upload gambar, buat Ujian (manual / random N dari bank), termasuk setting `IzinkanReviewSetelahSubmit` + `WaktuBukaReview`
- Backend: Start session (with assignment snapshot), play, answer auto-save, submit, auto-grade, anti-cheat log
- Backend: Resume + Preview + Duplicate + Remedial + Review (mirror Fase 5)
- Frontend guru: bank soal page dengan image, buat ulangan harian, preview, hasil rekap + reset + duplicate
- Frontend siswa: lobby ulangan harian + play full screen + auto-resume + Review (kalau dibuka)
- Test: scenario timer expired, concurrent submit, reset & re-attempt, random pool deterministik

### Fase 7 — Rekap Nilai & Transparansi + Activity Feed + Pending Counters (4 hari)
- Backend: GET /siswa/kelas/:id/nilai (formula nilai bab — section 6.2, dengan NilaiSetelahPenalty)
- Backend: GET /siswa/nilai (lintas kelas)
- Backend: GET /guru/kelas/:id/rekap-nilai (matrix siswa × bab + ulangan harian)
- Backend: **GET /guru/feed** — opaque cursor `(at_unix_micro, id)` base64 pagination, polling 30s pakai cursor=null
- Backend: GET /guru/pending-counts (badge sidebar)
- Backend: **GET /guru/kelas/:id/audit** — guru audit scope (subset action, target_kelas_id filter)
- Backend: export CSV nilai
- Frontend siswa: `/siswa/kelas/[id]/nilai` (transparansi per bab + breakdown), `/siswa/nilai` (lintas kelas)
- Frontend guru: `/guru/kelas/[id]/rekap-nilai` (matrix), tombol Export CSV
- Frontend guru: dashboard activity feed (polling 30s + load-more pakai cursor) + pending counters di sidebar (badge)
- Frontend guru: `/guru/kelas/[id]/audit` halaman riwayat aksi di kelas (filter action, paginated)

### Fase 8 — Polish & Production-ready (3-4 hari)
- Logging hardening, error handling, structured error response (`{ error, code, request_id }`)
- Backup `pg_dump` cron daily ke folder lain (rotation 30 hari rolling, monthly archive 1 tahun)
- **Backup restore drill**: dokumentasikan + test restore di staging (minimal 1x sebelum go-live)
- Hardening (CORS same-origin, file size limit 20MB tugas, gambar size limit 5MB per file pre-resize, mime sniff via `http.DetectContentType` + allowlist eksplisit, executable mime blocklist)
- Cleanup tasks (daily cron):
  - Orphan gambar soal (gak ke-reference SoalBab/Soal manapun)
  - ImportJob credentials.csv expired (>1 jam)
  - LoginAttempt >30 hari
  - RefreshToken expired & revoked >7 hari
  - HasilSoalBab/HasilUjian deleted_at >1 tahun → hard delete + audit log
  - Submission file: kelas archived + 1 tahun → hard delete file (DB row tetap)
- Web performance pass (bundle size, Core Web Vitals)
- Timezone validation: server `Asia/Jakarta`, frontend tampil WIB explicit, semua timestamp di-format konsisten
- **Coverage gate ketat**: backend `auth/admin/soalbab/ujian/nilai` ≥ 70%, fail CI kalau di bawah
- Playwright E2E core flows:
  - admin login -> bikin user guru -> guru login (force change password) -> bikin kelas -> publish bab -> tambah materi -> bikin soal latihan
  - admin import siswa CSV -> siswa login -> force change password -> join kelas -> kerjain latihan -> kerjain ulangan bab -> resume scenario -> review jawaban
  - **submit race scenario**: 2 tab buka ulangan bareng, submit barengan, verify cuma 1 yang terhitung
  - **suspend kick session**: admin suspend user yang lagi login, refresh next request → 401 + redirect ke login
- README polish + screenshot demo

**Total estimasi:** ~6-7 minggu kerja santai
- Fase 0 setup
- Fase 1 auth + admin (lebih lama karena security stack penuh)
- Fase 2 kelas + bulk import
- Fase 3-7 fitur akademik berbasis Bab dengan recovery + remedial + transparansi nilai + activity feed
- Fase 8 polish

> Notifikasi: skipped sampai bedah v0.8 — placeholder UI bell tidak dibikin sampai keputusan ada.

---

## 11. Risks / Concerns

- Concurrency ulangan: partial unique index `(bab_id, siswa_id, mode='ulangan') WHERE deleted_at IS NULL` & `(ujian_id, siswa_id) WHERE deleted_at IS NULL` wajib
- Timer drift: server autoritatif, frontend cuma display
- Upload file: limit 20MB tugas, 5MB per gambar soal, validate mime
- Backup data nilai: cron `pg_dump` daily ke folder lain
- Static export limit: gak bisa SSR (gak masalah, semua data via API)
- Kode invite collision: 6 char alfanumerik (~2.1B), retry kalau collision
- Cascade delete Bab: kalau ada hasil siswa, deletion harus ditolak (pakai status=archived dulu) atau warning loud
- Renormalisasi bobot nilai bab kalau ada komponen NULL — perlu dites edge cases
- Resume race: dua tab buka ulangan bersamaan -> server tetap satu session, frontend cek `Status=berlangsung` & lock UI di tab kedua
- Remedial audit trail: tiap reset attempt wajib masuk `audit_log` dengan actor + target + reason (diketik guru)
- Late penalty edge case: lock penalty saat submit (snapshot `IsLate`), jangan re-calc saat grading kalau guru ubah `PenaltyPersen`
- Timezone: PostgreSQL pakai `TIMESTAMPTZ`, server lock TZ ke `Asia/Jakarta`, tampilkan di frontend dengan suffix WIB explicit
- Image storage growth: gambar soal numpuk; cleanup task (Fase 8) untuk hapus orphan files yang gak ke-reference
- **Password lifecycle**: password awal cuma muncul SEKALI di modal — kalau admin lupa salin, satu-satunya jalan reset ulang. Kasih copy button + confirmation sebelum tutup modal.
- **CSV credentials file leak**: file ada di disk sementara, harus di-cleanup setelah download atau timeout 1 jam. Path harus di luar `frontend/out/` supaya gak ke-serve sebagai static.
- **Rate limit memory**: in-memory store buat rate limit hilang kalau service restart — attacker bisa exploit. OK untuk MVP karena restart jarang. Nanti pindah ke Redis kalau ada notifikasi pakai Redis (v0.8+).
- **Force password change bypass**: pastikan middleware cek di SEMUA endpoint kecuali whitelist. Tes manual: login user yang must_change_password=true, coba akses /api/v1/kelas -> harus 403.
- **Random pool determinisme**: shuffle pakai seed `(mulai_at unix + siswa_id)`, simpan urutan di `*SoalAssignment` saat start. Kalau gak ada assignment, resume bakal random ulang -> jawaban tersimpan gak match. Test scenario ini di TDD.
- **Bab Status & data integrity**: kalau guru unpublish bab yang udah ada hasil siswa, hasil tetap valid (snapshot di assignment), tapi siswa gak bisa lihat detail bab lagi. Tampilkan di /siswa/nilai dengan label "(bab tidak tersedia)".
- **Migration rollback**: simpan migration bersama `up.sql` + `down.sql`. Production rollback dengan `migrate down 1` — tes di staging dulu.
- **Duplicate kelas/bab — referensi**: hati-hati copy gambar — pakai path baru atau reference shared file? Default: copy file (boros tapi aman dari delete).
- **Refresh token reuse race**: kalau attacker pakai refresh token curian sebelum legit user refresh, attacker dapet pair baru, legit user kena revoke. Mitigasi: detect reuse → revoke chain + email/audit alert. Trade-off: legit user kadang ke-logout kalau ada race kondisi browser-buffer; acceptable security trade.
- **SELECT FOR UPDATE deadlock**: kalau dua tab submit + timer expire job barengan ke 1 row HasilSoalBab. Mitigasi: timeout lock 5 detik, retry 1x, kalau masih deadlock → return 409 ke client. Test scenario di Fase 5/6.
- **Mime sniff false positive**: `http.DetectContentType` baca 512 byte pertama. File markdown atau text encoding aneh kadang di-detect sebagai `application/octet-stream`. Allowlist harus include `text/plain` untuk markdown materi. Test dengan sample file real.
- **Image resize OOM**: gambar 50MB jpg yang ke-bypass size check bisa decode jadi 4GB di memory. Set `image.DecodeConfig` dulu, reject kalau dimension > 10000px sebelum full decode.
- **Progress formula edge case**: bab kosong total (0 materi, 0 latihan, 0 ulangan, 0 tugas) → divide-by-zero. Return 0 atau "—" eksplisit. Test scenario.
- **Readyz flapping**: kalau DB sempet down 1 detik, readyz return 503, monitor alert. Tambah grace window: 3x consecutive fail dalam 30 detik baru consider down. Or pake circuit breaker simpel.
- **AuditLog growth**: forever retention bisa numpuk. Saran: partition by month di Postgres setelah 1 tahun, atau archive ke S3-compatible storage di v1.
- **Admin lock-out**: kalau admin satu-satunya kena lock + lupa password + gak ada SSH access ke server → stuck. Mitigasi: setup SSH backup access (≥2 admin server-level), runbook recovery di `docs/DEPLOY.md`, jangan kasih account admin satu-doang ke 1 orang produksi (minimal 2 admin user di sistem).
- **ImportJob abandoned**: admin upload preview tapi tutup tab tanpa confirm. File numpuk di disk + DB row. Cleanup hourly cron wajib running. Test scenario: upload + close, tunggu 1 jam, verify cleanup.
- **Version conflict UX**: terlalu agresif (semua PATCH 409) bisa annoying kalau user kerja sendiri. Solusi: client auto-fetch version sebelum submit, kasih banner "Konten ke-update orang lain" cuma kalau real conflict. Default test: 2 tab edit bab → tab kedua kena 409 → load fresh data + retry.
- **Frontend env mistake**: lupa rebuild FE setelah ubah `NEXT_PUBLIC_API_BASE`. Siswa dapet 404 di production karena API URL salah. Mitigasi: tampilkan banner "API base: /api/v1" di footer dev mode, sanity check di startup script.
- **CSV import preview leak**: PreviewRowsJSON bisa berisi PII (nama, email siswa). Kalau admin lain bisa lihat ImportJob bukan miliknya → leak. Strict scope: query selalu `WHERE admin_id = current_user.id`.
- **AuditLog target_kelas_id backfill**: existing audit_log row sebelum migration #59 gak punya target_kelas_id. Fase 7 implement: migration set NULL untuk existing, baru row baru wajib isi kalau action terkait kelas.

---

## 12. Open Decisions Tersisa (v0.7.2)

1. **Notifikasi**: bentuk apa, kapan trigger, polling/SSE/websocket — bedah di v0.8 setelah Fase 0-3 jalan.
2. **Pengumuman dismiss state per siswa**: sekedar "udah dilihat" atau ada read receipt? — diputuskan saat Fase 3 implementasi.
3. **Pending counters polling vs realtime**: MVP polling 30s, kalau kerasa lemot pertimbangin SSE di v0.8.
4. **Bab unpublish dengan hasil existing**: tampil di /siswa/nilai sebagai "(bab tidak tersedia)" atau hide total. Default: tampil dengan label.
5. **JWT storage strategy**: localStorage (current, gampang implement, gak ada CSRF risk) vs httpOnly cookie (lebih aman dari XSS, butuh CSRF token). MVP: localStorage. Re-evaluate di v0.8 kalau audit security minta.
6. **Self change-password — revoke other sessions only?**: current default revoke ALL termasuk current device (user re-login). Alternatif: revoke all KECUALI current jti (UX lebih halus). Pilih saat Fase 1 implement.
7. **AuditLog partitioning**: kapan trigger? Kalau >1 juta row atau >1 tahun. Decide di v1.
8. **Share bank soal antar guru**: defer ke v1. Sekarang bank soal pribadi per guru.
9. **Email notification**: tetap YAGNI sampai v1, atau worth tambah minimal "password reset link" di v0.9? Tunda keputusan.
10. **AuditLog backfill `target_kelas_id`**: existing rows pre-Fase 7 set NULL. Skip backfill (cuma kelas-scope action baru yang isi). Confirm OK saat Fase 7.

---

## 13. Deploy Strategy (mengikuti pola fb-bot)

Reference: `D:\program\facebook-bot\docs\DEPLOY.md`. Adopsi pola yang sama, disesuaikan:

### 13.1 Production target
- Single Ubuntu VM `rdpkhorur` (Jakarta)
- PostgreSQL lokal di host yang sama
- Go binary listen `0.0.0.0:8200`, juga serve `frontend/out/` sebagai static
- **Tidak pakai Nginx** — sama seperti fb-bot
- SSH tunnel untuk akses browser: `ssh -L 8200:127.0.0.1:8200 rdpkhorur`

### 13.2 Project layout di server
- `/home/ubuntu/lms` (mirip `/home/ubuntu/fb-bot`)
- Binary build di server: `/home/ubuntu/lms/backend/bin/lms-api`
- Frontend static: `/home/ubuntu/lms/frontend/out/`
- Storage: `/home/ubuntu/lms/storage/uploads/`
- `.env` di root project

### 13.3 systemd service: `lms-api.service`
```
[Unit]
Description=LMS API (Go Fiber, serves backend + static frontend)
After=network.target postgresql.service

[Service]
Type=simple
User=ubuntu
Group=ubuntu
WorkingDirectory=/home/ubuntu/lms
EnvironmentFile=/home/ubuntu/lms/.env
ExecStart=/home/ubuntu/lms/backend/bin/lms-api
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

### 13.4 TL;DR ship a change (mirip fb-bot)
```bash
# local (C:\Users\pikip\Documents\program\lms)
git add -A
git commit -m "feat(...): ..."
git push origin main

# server (rdpkhorur)
ssh rdpkhorur "cd /home/ubuntu/lms && git fetch origin \
    && git reset --hard origin/main \
    && cd frontend && npm install --silent && npm run build \
    && cd ../backend && go build -o bin/lms-api ./cmd/server \
    && sudo systemctl restart lms-api \
    && systemctl is-active lms-api"
```

Verify:
```bash
ssh -L 8200:127.0.0.1:8200 rdpkhorur
# browser: http://localhost:8200
```

### 13.5 First-time server setup
1. **Base packages**: `sudo apt install -y golang nodejs npm postgresql git build-essential`
2. **Go versi terbaru** (kalau apt punya yang lama): pasang dari tarball atau `snap install go --classic`
3. **PostgreSQL**: `sudo systemctl enable --now postgresql`, bikin user+DB:
   ```sql
   CREATE USER lms WITH PASSWORD 'xxx';
   CREATE DATABASE lms OWNER lms;
   ```
4. **Clone**: `git clone git@github.com:<user>/lms.git /home/ubuntu/lms`
5. **`.env`**: `cp .env.example .env`, isi: `DATABASE_URL`, `JWT_SECRET_KEY`, `PORT=8200`, `STORAGE_DIR=./storage/uploads`, `ENV=production`
6. **Build**:
   ```bash
   cd backend && go mod download && go build -o bin/lms-api ./cmd/server
   cd ../frontend && npm install && npm run build
   ```
7. **systemd**:
   ```bash
   sudo cp deploy/systemd/lms-api.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now lms-api
   ```
8. **Akses**: `ssh -L 8200:127.0.0.1:8200 rdpkhorur` -> `http://localhost:8200`

### 13.6 Rollback
```bash
cd /home/ubuntu/lms
git log --oneline -n 5
git reset --hard <sha>
cd frontend && npm run build && cd ../backend && go build -o bin/lms-api ./cmd/server
sudo systemctl restart lms-api
```

### 13.7 Backup
Cron daily:
```bash
0 2 * * * pg_dump -U lms lms > /home/ubuntu/lms-backups/lms_$(date +\%F).sql
```

### 13.8 Logs
```bash
journalctl -u lms-api -f --no-pager -n 200
```

---

## 14. Frontend Development Arsenal — Skills & Agents

Daftar skill & agent yang DIPAKE buat ngembangin frontend LMS. Tiap skill/agent gue load otomatis di fase yang relevan, lu gak perlu inget.

### 14.1 Core Frontend Skills (wajib load di tiap fase yang menyentuh FE)

| Skill | Kapan dipakai | Fase |
|-------|---------------|------|
| `senior-frontend` | Scaffold komponen, performance, UI best practices (React + Next + TS + Tailwind) | 0+ |
| `nextjs-app-router-patterns` | Routing, layout, data fetching (App Router) | 0+ |
| `frontend-developer` | Lifecycle frontend lengkap, integrasi BE, a11y WCAG 2.2, Core Web Vitals | 0+ |
| `typescript-pro` | Tipe-tipe rumit, generic, strict mode | 0+ |
| `shadcn` | Setup & manage komponen shadcn/ui (Button, Dialog, Form, dsb) | 0+ |
| `tanstack-query-expert` | Query caching, mutation, optimistic update untuk semua list/detail | 1+ |
| `zustand-store-ts` | Store auth + UI state pakai pattern + middleware | 1+ |
| `zod-validation-expert` | Schema form (login, ganti password, buat kelas/bab/soal/tugas/ulangan, CSV import) | 1+ |
| `react-query-error-boundaries` | Error boundary + global toast cache error | 2+ |
| `react-component-performance` | Optimize page yang berat (rekap nilai, list submission) | 5+ |
| `web-performance-optimization` | Bundle size, Core Web Vitals sebelum production | 8 |
| `react-animejs-v4` | (Opsional) animasi halus di transisi soal/timer ulangan | 5-6 |

### 14.2 Design / UX Skills

| Skill | Fungsi | Kapan |
|-------|--------|-------|
| `ui-ux-pro-max` | Design intelligence: 50 styles, 21 palettes, 50 font pairings, generate komponen polished | 0 (set baseline visual) |
| `frontend-design` | Bikin halaman polished (landing, dashboard) yang gak generic AI | 0, 2, 6 |
| `ui-design-system` | Generate design tokens, dokumentasi komponen, handoff dev | 0 |
| `ui-ux-designer` | Audit/kritik UI: WCAG, eye-tracking, NN/g — review fix prioritized | tiap akhir fase visible |
| `popular-web-designs` | Referensi 54 sistem design real (Stripe/Linear/Vercel) buat inspirasi | 0 |
| `design-md` | (Opsional) `DESIGN.md` token spec | 0 |
| `mobile-design` | (Opsional) prinsip touch & mobile kalau LMS dipake di tab/HP | 6+ |

**Baseline visual (saran):**
- Style: minimalism + bento grid untuk dashboard
- Palette: tone netral + 1 accent (biru/hijau-akademik)
- Font pairing: ditentukan via `ui-ux-pro-max` di Fase 0
- Font primer: Inter (UI), JetBrains Mono (code/skor)
- Komponen: shadcn `new-york` style (default)

### 14.3 Quality & Process Skills

| Skill | Fungsi | Kapan |
|-------|--------|-------|
| `writing-plans` | Bite-sized task plan (gue pake buat append section task plan) | tiap awal fase |
| `subagent-driven-development` | Eksekusi plan task-by-task via delegate_task subagent (review 2-stage) | tiap fase eksekusi |
| `test-driven-development` | RED-GREEN-REFACTOR untuk component & API contract | tiap fitur baru |
| `code-reviewer` | Review TypeScript/React sebelum commit | tiap PR / batch commit |
| `requesting-code-review` | Pre-commit security & quality gate | sebelum push |

### 14.4 Coding Agents (delegate via `delegate_task`)

Buat tugas yang banyak/repeat atau biar context utama gue bersih, gue delegasi ke subagent. Tiga opsi yang available:

| Agent | Profil | Kapan dipake |
|-------|--------|--------------|
| `claude-code` (Claude Code CLI) | Reasoning kuat, refactor besar, design-aware | Bikin halaman kompleks (engine ulangan, rekap nilai, formula bab), desain UI baru |
| `codex` (OpenAI Codex CLI) | Cepat, eksekusi PR-style task | Scaffold rutin (CRUD page, form), generate boilerplate |
| `opencode` (OpenCode CLI) | Open-source, review-friendly | Code review pass kedua, second opinion |

**Pola pakai:**
- Fase 0-2: tugas masih kecil -> gue handle langsung tanpa delegasi
- Fase 3-6: per-task gue bisa delegasi ke `codex` (CRUD scaffolding) atau `claude-code` (engine ujian)
- Tiap delegasi: pasangin skill `senior-frontend` + `nextjs-app-router-patterns` + skill spesifik fitur (mis `tanstack-query-expert`)
- Dua-stage review pakai `subagent-driven-development`: stage 1 spec compliance, stage 2 code quality

### 14.5 Stack Frontend (recap final)

Yang dipake (di-confirm via skill di atas):
- **Next.js 14 App Router** + **TypeScript** + `output: 'export'`
- **Tailwind CSS** + **shadcn/ui** (style: new-york) + **lucide-react** + **Radix UI** (via shadcn)
- **TanStack Query** (server state) + **Zustand** (global UI/auth state)
- **React Hook Form** + **Zod** + `@hookform/resolvers/zod`
- **react-markdown** + **remark-gfm** (render materi markdown)
- **date-fns** (format tanggal/timer)
- **clsx** + **tailwind-merge** + **class-variance-authority** (bawaan shadcn)
- **TanStack Table** (kalau perlu sort/filter di rekap nilai, fase 6+)
- **react-animejs-v4** (opsional, animasi transisi ujian)

Test:
- **Vitest** + **@testing-library/react** (unit/component)
- **Playwright** (E2E untuk flow login/join kelas/kerjain ulangan, fase 8)

Build & dev:
- `next build && next export` -> `frontend/out/` -> di-serve oleh Go Fiber
- Dev mode: `next dev` di port 3000, backend di 8200, dengan `NEXT_PUBLIC_API_BASE=http://localhost:8200/api/v1`

---

## 15. Implementation Notes

_belum ada — masih konsep, belum mulai coding_

## 16. Current Next Step

**Fase 0 SELESAI ✅** (commit `24eab15`, deployed ke rdpkhorur, systemd `lms-api` active, healthz/readyz green, migrate `000001_init` applied).

Sedang masuk **Fase 1 — Auth & Admin Panel**. Detail bite-sized tasks ada di **Section 18 (Task-by-Task Implementation Plan)**.

Open dependencies sebelum Fase 1 mulai:
1. (Opsional) Setup GitHub remote — saat ini pakai bare repo `/home/ubuntu/git-repos/lms.git`. Bisa di-swap ke GitHub kapanpun tanpa block kerja.
2. (Wajib sebelum first user) Bedah notifikasi (v0.8) tetap di-tunda sampai mendekati Fase 4.

Mau eksekusi Fase 1 task-by-task lewat `subagent-driven-development`, atau gue handle inline? (Default: inline — task masih kecil, less context overhead.)

### Changelog v0.7.1 → v0.7.2
- **Locked**: 9 keputusan baru (#52-60) — multi-admin promote w/ re-auth, admin lock-out recovery, CSV preview persistence, feed cursor, concurrent edit version, auth boundary explicit, storage path convention, guru audit scope, frontend env strategy.
- **Section 6**: tambah `Version` field di Kelas/Bab/SoalBab/UlanganBabSetting/Soal/Ujian; ImportJob expand (Status, PreviewRowsJSON, ExpiresAt, ConfirmedAt, CompletedAt); AuditLog tambah `TargetKelasID`; new indexes for ImportJob + AuditLog scope.
- **Section 7**: split admin user PATCH dari role endpoint, tambah `/admin/users/:id/role` (re-auth), `/admin/users/:id/sessions`, `/admin/users/:id/revoke-sessions`, ImportJob endpoints (resume, cancel), feed cursor, guru audit scope endpoint.
- **Section 10**: Fase 1 tambah promote re-auth + auth boundary middleware order; Fase 2 tambah ImportJob lifecycle + storage convention + version field di Kelas; Fase 3 tambah version di Bab + materi storage path + progress formula 6.4; Fase 7 tambah feed cursor + guru audit page.
- **Section 11**: 6 risiko baru (admin lock-out, ImportJob abandoned, version conflict UX, frontend env mistake, CSV preview leak, audit log backfill).
- **Section 12**: 3 open decisions baru (#8 share bank soal, #9 email notif, #10 audit backfill).

### Changelog v0.7 → v0.7.1
- **Locked**: 10 keputusan baru (#42-51) — session revocation, submit concurrency, healthz/readyz, remedial snapshot policy, file upload hardening, global rate limit, bab progress formula, request ID middleware, test coverage target, data retention.
- **Section 4.5**: rate limit detail diperluas (refresh/join/upload), JWT note jadi "stateless access + stateful refresh".
- **Section 6**: tambah `RefreshToken` table + indexes + section 6.4 formula progress bab.
- **Section 7**: tambah `/healthz`, `/readyz`, `/auth/logout-all`, `/auth/sessions`, admin user sessions endpoint.
- **Section 10**: Fase 0 nambah request-id + rate limit + readyz; Fase 1 expand auth (refresh rotation, reuse detection, /me/perangkat); Fase 5 expand submit transition + remedial snapshot; Fase 8 expand cleanup tasks + restore drill + race scenario E2E.
- **Section 11**: 7 risiko baru (refresh reuse race, FOR UPDATE deadlock, mime sniff false positive, image OOM, progress edge case, readyz flapping, audit log growth).
- **Section 12**: 3 open decisions baru (#5 JWT storage, #6 change-pw revoke scope, #7 audit partitioning).

---

## 17. First Admin Bootstrap

Karena tidak ada self-register publik, admin pertama harus dibikin via CLI. Pola:

### 17.1 CLI tool: `cmd/seed-admin`

File: `backend/cmd/seed-admin/main.go`

Cara kerja:
1. Connect ke DB pakai `DATABASE_URL` dari `.env`
2. Cek apakah sudah ada user dengan role=admin
   - Kalau ADA -> exit dengan pesan: "Admin sudah ada, gunakan panel /admin untuk manage user"
   - Kalau BELUM -> lanjut step 3
3. Baca input dari env vars atau interactive prompt:
   - `ADMIN_EMAIL`
   - `ADMIN_PASSWORD`
   - `ADMIN_NAME` (opsional, default "Administrator")
4. Hash password dgn bcrypt
5. Insert ke tabel `users` dengan role=admin, status=active
6. Print sukses + email yang dibuat

### 17.2 Cara pakai

**Mode env vars (recommended untuk server):**
```bash
cd /home/ubuntu/lms/backend
ADMIN_EMAIL=admin@sekolah.id ADMIN_PASSWORD='ganti-secepatnya-123' \
  ./bin/seed-admin
```

**Mode interactive (untuk local dev):**
```bash
go run ./cmd/seed-admin
# prompt:
# Email: admin@sekolah.id
# Password: ********
# Name: Apis
```

### 17.3 Kapan dijalankan

- **Sekali** pas first-time deploy ke rdpkhorur (Fase 0 selesai, Fase 1 selesai)
- Setelah itu, admin pertama bisa bikin admin lain via panel `/admin/pengguna` (role bisa di-set ke admin saat create)

### 17.4 Safety

- CLI menolak run kalau sudah ada admin (cegah lupa & bikin admin ganda)
- Kalau lupa password admin pertama:
  - Opsi A: jalankan `cmd/reset-admin` (CLI lain) yang minta email -> set password baru
  - Opsi B: manual update DB: `UPDATE users SET password_hash='<bcrypt-hash>' WHERE email='...'`
- Password yang dipake jangan dipakai forever — login admin -> /me/security -> ganti password

### 17.5 Implementation di Fase 0

Task:
1. Bikin `backend/cmd/seed-admin/main.go` dengan flow di atas
2. Verifikasi: build (`go build -o bin/seed-admin ./cmd/seed-admin`), jalanin, cek DB
3. Dokumentasi di `docs/DEPLOY.md`: bagian "First admin bootstrap"

Setelah tools jadi, runbook deploy jadi:
```
1. git clone + .env + go build + npm build
2. systemctl start lms-api
3. ./backend/bin/seed-admin (sekali aja)
4. login ke /admin pake email yang barusan dibuat
5. ganti password di /me/security
6. mulai bikin akun guru dari /admin/pengguna
```

---

## 18. Task-by-Task Implementation Plan (Fase 0-2)

> Living checklist. Status legend: `[ ]` pending, `[~]` in progress, `[x]` done, `[!]` blocked.
> Setiap task = bite-sized 2-5 menit kerja, lengkap dengan path file, perintah verify, dan commit message.
> Update tiap selesai 1 task. "Current Next Step" di bagian akhir section ini = pointer eksekusi berikutnya.

### Konvensi commit
- Format: `<type>(<scope>): <imperative>`
- Type: `feat`, `fix`, `chore`, `docs`, `refactor`, `test`
- Scope: `auth`, `admin`, `bab`, `kelas`, `db`, `fe`, `deploy`, `migrations`, dst.
- Verify command default backend: `cd backend && go build ./... && go test ./...`
- Verify command default frontend: `cd frontend && npm run build`
- Push flow: `git push server main` → ssh `cd /home/ubuntu/lms && git pull origin main && cd backend && go build -o bin/lms-api ./cmd/server && sudo systemctl restart lms-api`

---

### Fase 0 — Setup ✅ DONE (commits `071d25e`, `f50c8b5`, `24eab15`)

| # | Task | Status |
|---|------|--------|
| 0.1 | Init repo + .gitignore + .env.example + LOCAL_AI_CONTEXT.md | [x] |
| 0.2 | Backend Go module + Fiber + GORM + healthz/readyz + request-id + ratelimit middleware | [x] |
| 0.3 | Backend CLI scaffolding: `cmd/seed-admin` + `cmd/reset-admin` (stub, full di Fase 1) | [x] |
| 0.4 | Migrations dir + `000001_init.up/down.sql` (extensions + schema_meta) | [x] |
| 0.5 | Frontend Next 14 scaffolding (landing, login stub, /me, /me/security, /lupa-password) | [x] |
| 0.6 | systemd unit + deploy.sh + DEPLOY.md + ARCHITECTURE.md + README.md | [x] |
| 0.7 | Push ke rdpkhorur via bare repo `/home/ubuntu/git-repos/lms.git` | [x] |
| 0.8 | Build + smoke test di server (healthz/readyz/static, X-Request-ID, rate limit headers) | [x] |
| 0.9 | `migrate up` apply `000001_init` → schema_meta populated | [x] |
| 0.10 | Document Postgres port 5435 di `.env.example` + `LOCAL_AI_CONTEXT.md` | [x] |
| 0.11 | systemd unit install + enable + start (drop ProtectHome, .env via setup-env.sh) | [x] |

---

### Fase 1 — Auth & Admin Panel (4-5 hari)

#### 1.A Schema Auth (migration 000002)

**Task 1.A.1 — Bikin migration `000002_auth_schema.up.sql`** ✅ done (`e8df533`)
- Files: `backend/migrations/000002_auth_schema.up.sql`, `backend/migrations/000002_auth_schema.down.sql`
- Tables: `users`, `refresh_tokens`, `login_attempts`, `audit_logs`
- Reference: Section 6 (User, RefreshToken, LoginAttempt, AuditLog) + Section 6.3 indexes
- Verify: `migrate -database "$DATABASE_URL" -path migrations up` di server, `psql ... -c '\dt'` cek 4 table baru
- Commit: `feat(migrations): 000002 auth schema (users, refresh_tokens, login_attempts, audit_logs)`
- Done: schema_meta `schema_version=000002_auth_schema`, gen_random_uuid() (no uuid-ossp)

**Task 1.A.2 — GORM models di `backend/internal/auth/model.go`** ✅ done (`d80ed3b` + `478b4a5` lockfiles)
- Files: `backend/internal/auth/model.go`
- Models: `User`, `RefreshToken`, `LoginAttempt`, `AuditLog` (full field per Section 6)
- Tag GORM: `column:`, `not null`, `default:`, `index:`, `uniqueIndex:`
- Verify: `cd backend && go build ./...`
- Commit: `feat(auth): GORM models User RefreshToken LoginAttempt AuditLog`
- Done: gorm.io/datatypes v1.2.7 added; build + vet PASS at server; go.sum + package-lock.json now committed for reproducible builds

**Task 1.A.3 — Repository layer** ✅ DONE (commit `18f7a4e`, 2026-05-19)
- Files: `backend/internal/auth/repo.go` (199 baris)
- Done: Repo struct + NewRepo + 17 methods. User: FindByEmail, FindByID, Create, UpdatePassword, IncFailed, ResetFailed, LockUser (transactional). RefreshToken: Issue, FindByJTI, Rotate (transactional + reuse trigger), Revoke, RevokeAllByUser, RevokeChain, ListUserSessions. LoginAttempt: Log, CountRecentFailedAttempts. AuditLog: Log. `gorm.Expr("now()")` server-side timestamps; build + vet PASS at server.

#### 1.B Login + JWT + Rate Limit

**Task 1.B.1 — bcrypt password helper** ✅ DONE (commit `fa5ba82`, 2026-05-19)
- Files: `backend/internal/auth/password.go` (30 LOC) + `password_test.go` (56 LOC)
- Done: `HashPassword(plain, cost)` (cost 0 → DefaultCost, validates MinCost..MaxCost) + `VerifyPassword(hashed, plain)`. Tests: roundtrip, wrong password, default cost when 0, rejects invalid cost. golang.org/x/crypto promoted to direct.

**Task 1.B.2 — JWT issue + verify** ✅ DONE (commit `fa5ba82`, 2026-05-19)
- Files: `backend/internal/auth/jwt.go` (117 LOC) + `jwt_test.go` (124 LOC)
- Done: AccessClaims (UserID, Role, RegisteredClaims) + RefreshClaims (UserID + JTI in RegisteredClaims.ID). HS256 sign/verify, Issuer="lms-api", config-driven TTL. Sentinel `ErrInvalidSigningMethod`. Tests: roundtrip access+refresh, wrong secret, expired token, invalid signing method (alg=none). Dep added: github.com/golang-jwt/jwt/v5 v5.2.1. Server build/vet/test PASS.

**Task 1.B.3 — Login service** ✅ DONE (commit `4339f2b`, 2026-05-19)
- Files: `backend/internal/auth/service.go` (242 LOC) + `service_test.go` (400 LOC)
- Done: `Service.Login(ctx, email, password, ip, ua) (*LoginResult, error)` dengan flow: normalize email → rate-limit (5/15min via CountRecentFailedAttempts) → FindUserByEmail (gorm.ErrRecordNotFound → ErrInvalidCredentials, no leak) → status gate (Suspended/Locked) → VerifyPassword → on fail: IncFailedLogin + auto LockUser kalo cumulative >=10 → on success: ResetFailedLogin + IssueAccess+IssueRefresh + persist RefreshToken + audit log.
- Sentinel errors: `ErrInvalidCredentials`, `ErrUserSuspended`, `ErrUserLocked`, `ErrRateLimited`.
- Internal `authRepo` interface (subset of *Repo) untuk tests via mockRepo (no DB driver added). 9 test cases pass: success, wrong password, user not found (no leak), suspended, locked, rate-limited (before lookup), auto-lock at 10th fail, email normalization, empty email no-logging.
- Server `go build` + `go vet` + `go test` PASS (0.270s).

**Task 1.B.4 — Login HTTP handler + route + auth-login rate limiter middleware** ✅ DONE (commit `f254b35`, 2026-05-19)
- Files: `backend/internal/auth/handler.go` (132 LOC) + `handler_test.go` (178 LOC) + `cmd/server/main.go` mount
- Done: Handler struct + `Login(c)` + `LoginRateLimit(perWindow)` middleware. Body `{email, password}` → 200 `{user, tokens:{access_token, access_expires_at, refresh_token, refresh_expires_at}}`. Sentinel mapping: ErrInvalidCredentials→401, ErrUserSuspended/ErrUserLocked→403, ErrRateLimited→429. Rate limit middleware key = `ip|email` (peek body via json.Unmarshal, no BodyParser interference), Max=cfg.RateLimit.LoginPer15Min, window=15min.
- Test: 7 cases (success, invalid_credentials, suspended, locked, rate_limited, bad json 400, missing fields 400). Server build/vet/test PASS.
- E2E verified di server (8200): bad json→400, empty body→400, unknown user→401, 5 rapid same-email attempts → attempt 5 jadi 429 (Fiber limiter `count >= Max` semantik; jadi block AT 5th, bukan AFTER 5th — acceptable per locked decision "5 gagal/15min").
- Dual rate limit: middleware coarse (counts ALL requests, IP+email key) + service-layer precise (counts only FAILED LoginAttempt rows). Defense-in-depth.

**Task 1.B.5 — Refresh rotation + reuse detection** ✅ DONE (commit `0656e4d`, 2026-05-19)
- Files: extend `service.go` (+125 LOC) + `service_test.go` (+332 LOC)
- Done: `Service.Refresh(ctx, refreshToken, ip, ua) (*LoginResult, error)` flow: VerifyRefresh JWT → uuid.Parse JTI → repo.FindRefreshByJTI (gorm.ErrRecordNotFound → ErrInvalidCredentials, NO chain revoke) → user mismatch check (defense) → reuse detection (RevokedAt != nil → repo.RevokeRefreshChain reason=reuse_detected → ErrRefreshReuse) → expiry check → user status gate (Suspended/Locked) → IssueAccess+IssueRefresh → repo.RotateRefresh (atomic revoke-old + insert-new) → audit `refresh_success`.
- New sentinel: `ErrRefreshReuse` — for compromised token replay.
- Extended authRepo interface: FindRefreshByJTI, RotateRefresh, RevokeRefreshChain.
- 9 test cases pass: success rotation (verify old.RevokedAt set + ReplacedByJTI = new), invalid JWT, wrong secret, unknown JTI (no chain revoke — could be replay before deploy), reuse detection chain revoke (verify other user tokens revoked), expired persisted token, user suspended, user locked, user mismatch.
- Server `go test -v -run Refresh` shows all 9 PASS in 0.018s. Full suite PASS (0.139s).

**Task 1.B.6 — POST /auth/refresh + POST /auth/logout + POST /auth/logout-all + GET /auth/sessions** ✅ DONE (commit `9855c56`, 2026-05-19, bundled dgn 1.C.1)
- Files: extend handler.go (+140 LOC), service.go (+63 LOC), service_test.go (+210 LOC), handler_test.go (+255 LOC), cmd/server/main.go (+9 LOC mount)
- Done: handlers Refresh/Logout/LogoutAll/Sessions + service methods Logout/LogoutAll/ListSessions/VerifyAccessToken. authService interface replaces loginService. RefreshRateLimit middleware dgn key=ip+token-prefix-16char (no full JWT in memory). Refresh sentinel mapping: ErrInvalidCredentials/ErrRefreshReuse→401, ErrUserSuspended/ErrUserLocked→403. Logout idempotent (bad token→204, not 401). LogoutAll/Sessions need bearer (via middleware.BearerAuth + UserIDFromCtx).
- Server build/vet/test PASS, E2E confirmed: refresh empty→400/bad→401, logout empty→400/bad→204, logout-all/sessions no-bearer→401, bad-bearer→401.

#### 1.C Auth Middleware

**Task 1.C.1 — Auth middleware (parse access JWT → set ctx user)** ✅ DONE (commit `9855c56`, 2026-05-19, bundled dgn 1.B.6)
- Files: `backend/internal/middleware/auth.go` (69 LOC)
- Done: `BearerAuth(verifier UserVerifier) fiber.Handler` reads `Authorization: Bearer <token>`, verifies via injected verifier, sets `c.Locals(LocalsUserID uuid.UUID, LocalsUserRole string, LocalsUserEmail string)`. Helper `UserIDFromCtx(c)` for handler retrieval. `ErrNoUserContext` sentinel. UserVerifier interface (auth package's *Service satisfies it via VerifyAccessToken method) keeps middleware decoupled — no import cycle.
- Whitelist via route placement (anonymous routes mounted on `authGroup` directly; protected on `authGroup.Group("", BearerAuth)`).
- Server build PASS, E2E: no bearer → 401 unauthorized, bad bearer → 401, valid bearer akan kebuka di Task 1.E.1 setelah seed-admin jalan.

**Task 1.C.2 — Role guard middleware (admin/guru/siswa) + ForceChangePassword middleware** ✅ DONE (commit `768333f`, 2026-05-19, bundled dgn 1.E.1)
- Files: `backend/internal/middleware/role.go` + `role_test.go` (84+36 LOC) + `force_change.go` + `force_change_test.go` (80+42 LOC)
- Done: `RoleGuard(allowedRoles ...string)` reads `LocalsUserRole`, 403 forbidden kalo tidak match. `ForceChangePassword()` reads `LocalsMustChangePassword` (new local), whitelist `/auth/me`, `/auth/change-password`, `/auth/logout`, `/auth/logout-all`. UserVerifier interface diperluas: `VerifyAccessToken` sekarang return `mustChange bool`. Helper `MustChangePasswordFromCtx(c)`.
- ⚠️ ForceChangePassword middleware BELUM di-wire ke routes — sengaja menunggu Task 1.D.1+1.D.2 supaya whitelist bisa di-test proper (with /auth/me + /auth/change-password endpoints live).
- Server build/vet/test PASS (`./internal/middleware/...` 0.014s).

**Task 1.E.1 — Lengkapi `cmd/seed-admin/main.go`** ✅ DONE (commit `768333f`, 2026-05-19, bundled dgn 1.C.2)
- Files: `backend/cmd/seed-admin/main.go` rewrite (drop stub, real flow) + `backend/internal/auth/repo.go` (+10 LOC `CountAdmins`)
- Done: connect DB → `repo.CountAdmins` → reject if >0 → `auth.HashPassword` (cost from cfg) → `repo.CreateUser` (Admin/Active/MustChangePassword=true) → `repo.LogAudit` action="admin_seeded" actor=NULL target=&newUserID. Idempotent: rerun → exit 1 dengan pesan "an admin already exists".
- E2E verified di server: seed-admin run pertama created `admin@sekolah.id` (UUID `8f6c7479-...`); rerun refused. POST /auth/login → 200 + real JWT tokens (`must_change_password=true`); /sessions w/ bearer → 200 (2 sessions setelah refresh); /refresh → 200 (new token pair, old rotated); /logout-all → 204; /sessions after → empty `[]`. **Full auth flow LIVE end-to-end.** ✅

#### 1.D Self Endpoints (`/auth/me`, change-password, sessions)

**Task 1.D.1 — GET /auth/me (return current user profile)** ✅ DONE (commit `188d2ab`, 2026-05-19, bundled dgn 1.D.2)
- File: `backend/internal/auth/handler.go` (Me handler) + `service.go` (Service.Me method)
- Done: GET /api/v1/auth/me dgn bearer → 200 `{user: {...}}` (PasswordHash hidden by json:"-"). Whitelisted di ForceChangePassword middleware.
- E2E PASS di server: bearer valid → 200 dgn user JSON.

**Task 1.D.2 — POST /auth/change-password (current_password + new_password)** ✅ DONE (commit `188d2ab`, 2026-05-19, bundled dgn 1.D.1)
- File: `backend/internal/auth/handler.go` (ChangePassword handler) + `service.go` (Service.ChangePassword + 3 sentinel errors)
- Done: POST /api/v1/auth/change-password dgn bearer + body `{current_password, new_password}` → 204. Flow: validate len(new) >=8 (`ErrWeakPassword`) → FindUserByID → VerifyPassword(current) (`ErrCurrentPasswordIncorrect` + audit `password_change_failed`) → check current != new (`ErrSamePassword`) → HashPassword (cost from cfg) → UpdateUserPassword (clears must_change_password=true) → RevokeAllRefreshByUser (reason=PasswordChanged) → audit `password_changed`.
- ⚠️ DECISION: Revoke ALL refresh (conservative default), bukan "except current". Frontend wajib re-login setelah change-password. Acceptable UX untuk bootstrap admin; bisa di-improve nanti dgn `current_refresh_token` di body kalo perlu.
- ForceChangePassword middleware wired ke protected group di `cmd/server/main.go`. Whitelist: /me, /change-password, /logout, /logout-all.
- E2E PASS di server: must_change=true admin → /me ✓ → /sessions 403 must_change → /change-password 204 → /sessions 200 (sessions empty after revoke-all) → login old pass 401, login new pass 200.

#### 1.E Admin Bootstrap CLI (full implementation)

**Task 1.E.1 — Lengkapi `cmd/seed-admin/main.go`** ✅ DONE — see Section 1.C above (bundled dgn 1.C.2 di commit `768333f`).

**Task 1.E.2 — Lengkapi `cmd/reset-admin/main.go`** ✅ DONE (commit `1cb0826`)
- Replace stub: flag `--email <email> --password <new>` (interactive kalau kosong) → find user role=admin → bcrypt new pass → update + revoke all refresh
- Verify: run di server, login admin pake password baru
- Implementation: validates role=admin (refuses non-admin), bcrypts new pass, calls `UpdateUserPassword`, best-effort `ResetFailedLogin`, `RevokeAllRefreshByUser(admin_reset)`, `LogAudit(admin_reset_via_cli, actor_id=NULL, target_id=user.ID)`. Best-effort on revoke + audit (does not abort post-update).
- Live E2E verified: `./bin/reset-admin --email admin@sekolah.id --password 'Reset-Test-2026!'` → revoked 1 token, old pass returns 401, new pass returns 200 (must_change_password=false), audit row inserted dgn actor_id=NULL.
- Note: locked-user unlock TODO (#53) — `UpdateUserPassword` clears `must_change_password` tapi tidak `status`. Logged warning kalau user.Status==Locked. Add repo method `UnlockUser` di task selanjutnya kalau dibutuhkan.

#### 1.F Admin Panel Endpoints

**Task 1.F.1 — Admin user CRUD endpoints** ✅ DONE (commit `102d750`)
- Routes: `GET /admin/users` (filter, search, paginate), `POST /admin/users` (toggle manual/generate password), `PATCH /admin/users/:id` (name only), `DELETE /admin/users/:id` (soft-suspend, gak hard delete)
- Body POST: `{name, email, role, password_strategy: manual|generate, password?}`
- Response POST: `{user, generated_password?}`
- Audit log per aksi (admin_user_created/admin_user_name_updated/admin_user_suspended) — actor_id + target_id + meta (role, strategy, old_name/new_name, previous_status)
- Implementation: new pkg `internal/admin` (handler.go 409 LOC + handler_test.go 622 LOC). New repo methods di `internal/auth/repo.go`: `ListUsers(filter, limit, offset)`, `UpdateUserName`, `SuspendUser`. 16-char crypto/rand password generator (charset stripped of ambiguous 0/O/1/l). Last-admin protection (cannot delete last admin) + cannot-delete-self. Mounted dgn `RoleGuard("admin")` + `BearerAuth` + `ForceChangePassword`.
- Live E2E verified: list (3 users), filter (?role=guru), search (?q=siswa), patch name, delete + status=suspended check, all 5 audit rows captured. Edge cases: last_admin_protected (400), email_already_exists (409), weak_password (400), invalid_role (400), invalid_id (400), no-bearer (401), siswa→/admin (403 insufficient_role).

**Task 1.F.2 — Admin user lifecycle endpoints** ✅ DONE (commit `e175944`)
- `POST /admin/users/:id/reset-password` (manual atau generate)
- `POST /admin/users/:id/suspend`, `POST /admin/users/:id/unsuspend`
- `POST /admin/users/:id/unlock`
- Semua: revoke all refresh user → audit log
- Implementation: 3 repo methods baru di `internal/auth/repo.go` (AdminResetUserPassword, UnsuspendUser, UnlockUser). 4 handler methods di `internal/admin/handler.go`. Reset-password set must_change_password=true + revoke refresh; suspend revoke + guards (last_admin, cannot_suspend_self, already_suspended); unsuspend guard not_suspended; unlock reset failed_login_count=0 + status=active + guard not_locked.
- Live E2E verified: reset manual + generate (16-char, login w/ new pass works, old pass 401), suspend dgn reason → audit + login returns user_suspended 403, suspend again → already_suspended 400, unsuspend → active, unsuspend again → not_suspended 400, lock via DB → unlock → status=active + failed_login_count=0, audit chain captured (created/password_reset×2/suspended/unsuspended/unlocked dgn meta lengkap).

**Task 1.F.3 — Admin role promote/demote (re-auth)** ✅ DONE (commit `4a83ef1`)
- `POST /admin/users/:id/role` body `{new_role, current_password}`
- Logic: verify current admin's password → cek bukan demote admin terakhir → update role → audit log dengan old_role + new_role
- Implementation: 1 repo method baru `UpdateUserRole`. Handler `ChangeUserRole` dgn `passwordVerifier` field (testable injection, default `auth.VerifyPassword`). Validation order: invalid_id → invalid_body → invalid_role → invalid_current_password (empty) → requester not found 401 → wrong password 401 invalid_current_password → target not found 404 → same_role 400 → last_admin_protected 400 → cannot_demote_self 400 → success. Revoke all refresh + audit (`admin_user_role_changed` dgn old_role/new_role meta) on success. Self-demote-self distinct dari last-admin (works dgn 2+ admin).
- Live E2E verified: wrong pass→401, invalid_role→400, same_role→400 (siswa→siswa), promote siswa→admin→200, self-demote primary admin (with 2 admins)→400 cannot_demote_self, demote calon admin→guru→200, audit chain {siswa→admin, admin→guru} captured.

**Task 1.F.4 — Admin sessions + audit + login attempts** ✅ DONE (commit `fb36219`)
- `GET /admin/users/:id/sessions`, `POST /admin/users/:id/revoke-sessions`
- `GET /admin/audit-log` (filter actor, target, action, since, until, paginate)
- `GET /admin/login-attempts` (filter email, success, since, until, paginate)
- Implementation: 2 repo methods baru di `internal/auth/repo.go` (ListAuditLogs + ListLoginAttempts dgn filter struct + total count). 4 handler methods di `internal/admin/handler.go`. Re-use ListUserSessions + RevokeAllRefreshByUser. Revoke-sessions audits dgn revoked_count + reason meta.
- Live E2E verified: list sessions (5 active), invalid_id 400, user_not_found 404, audit-log default 32 rows w/ pagination, filter action, invalid_actor_id, invalid_time, login-attempts default 24 rows, success=false 10 failed, invalid_success 400, revoke-sessions self-revoke 5 tokens (access token still valid until exp — expected since only refresh dies).

#### 1.G Frontend Auth + Self

**Task 1.G.1 — Login page wiring** ✅ DONE (commit `7b9cbb8`)
- Files: `frontend/app/(auth)/login/page.tsx`
- React Hook Form + Zod schema (email + password) + submit POST `/auth/login` via `lib/api.ts`
- On success: simpan access+refresh di Zustand + redirect: kalau `MustChangePassword` → `/me/security`, kalau admin → `/admin`, kalau guru → `/guru`, siswa → `/siswa`
- Implementation: bundled dgn shadcn init manual (no `npx shadcn` — file ditulis langsung dgn new-york style: button/card/input/label/form/toast/toaster + use-toast hook). Providers (TanStack QueryClient + Toaster) di-wire di root layout. lib/api.ts refactored — token sekarang dari Zustand store (`useAuthStore.getState().access`), ganti legacy `localStorage.lms.access` key. Snake_case→camelCase mapping untuk AuthUser di mutation onSuccess. Friendly Indonesian error toasts mapped per backend code (invalid_credentials/user_suspended/user_locked/too_many_requests). request_id surfaced in toast description.
- Live verified: server typecheck `npx tsc --noEmit` PASS (exit 0), `next build` PASS (8 static pages, /login=32.3 kB, all chunks served via Fiber Static), `curl /login` returns 200 + script tags `_next/static/chunks/*.js`, backend login API still 200 dgn admin role.

**Task 1.G.2 — Protected route HOC + auth refresh interceptor** ✅ DONE (commit `d092438`, 2026-05-20)
- Files: `frontend/lib/api.ts` (refresh interceptor + module-level mutex), `frontend/lib/auth-guard.tsx` (client guard waiting for zustand persist hydration), `frontend/app/(authed)/layout.tsx` (route group wrapper), `frontend/app/(authed)/me/*` (existing /me + /me/security moved into the group)
- Done: `lib/api.ts` extended with single-flight `refreshInFlight` promise so parallel 401s share one `/auth/refresh` round-trip; on success retries original request once with new bearer; on failure clears Zustand store + redirects `/login` (skipped if already on /login). Internal `skipRefresh` flag on `apiInner` prevents recursion when `/auth/refresh` itself returns 401. `AuthGuard` renders nothing until persist hydration finishes (avoids flash on hard reload), then enforces auth + force-change gate (whitelist `/me/security`). Route group `(authed)` keeps URL paths clean — no segment added.
- Live verified: server `npx tsc --noEmit` PASS (exit 0), `next build` PASS (8 static pages — /, /login, /me, /me/security, /lupa-password, /_not-found), all routes still served by Fiber Static (200).

**Task 1.G.3 — /me + /me/security pages full** ✅ DONE (commit `69f15b4`, 2026-05-20)
- Files: `frontend/app/(authed)/me/page.tsx` (191 LOC), `frontend/app/(authed)/me/security/page.tsx` (253 LOC)
- Done: `/me` GET `/auth/me` via TanStack Query (staleTime 60s), read-only profile (nama/email/role/status/last_login_at/created_at), formatted `Asia/Jakarta` via Intl. Logout button POST `/auth/logout` (best-effort, fail-closed) → clear store → /login. Force-change-password banner (#32) muncul kalau `user.must_change_password`, plus tombol Logout di-disable + toast peringatan kalau ditekan. `/me/security` form RHF + Zod (current/new/confirm dengan refine: confirm===new, new!==current, min 8 char), POST `/auth/change-password`, on 204 toast sukses + clear store + `/login` (server revoke all refresh tokens per #42, jadi client wajib re-login). Friendly errors mapped: invalid_current_password / weak_password / same_password. Back link ke `/me` di-disable (pointer-events-none + tabIndex=-1) selama mustChange=true.
- Live verified: server `npx tsc --noEmit` PASS (TSC_OK), `next build` PASS (8 static pages — /me=11.5 kB, /me/security=1.81 kB), curl http://127.0.0.1:8200/me=200 + /me/security=200 + /api/v1/readyz=200, lms-api active. FE-only change → no service restart needed.

**Task 1.G.4 — /me/perangkat — list active sessions + logout-all** ✅ DONE (commit `5ffae23`, 2026-05-20)
- Files: `frontend/app/(authed)/me/perangkat/page.tsx` (255 LOC), `frontend/app/(authed)/me/page.tsx` (+3 LOC link)
- Done: GET `/auth/sessions` via TanStack Query (staleTime 30s) → render list (masked JTI 4-dot-4, IP, issued/expires `Asia/Jakarta`, user-agent summary via heuristik browser+OS). Current session di-highlight pakai badge "Sesi ini" — best-effort decode unverified JWT payload refresh token di Zustand (atob + base64url normalize, payload.jti string check; UX hint, bukan trust boundary). Tombol "Logout dari semua perangkat" disabled saat loading/empty, POST `/auth/logout-all` → toast sukses + clear store + `/login`. Per-perangkat revoke ditunda v0.8 (no per-jti self-scope endpoint). /me dapet shortcut "Perangkat aktif" sebelah "Ganti password".
- Live verified: server `npx tsc --noEmit` PASS (TSC_OK), `next build` PASS (9 static pages — /me/perangkat=4.28 kB), curl /me/perangkat=200, /me=200, /api/v1/auth/sessions tanpa bearer=401 (expected), lms-api active.

#### 1.H Frontend Admin Panel

**Task 1.H.1 — Admin layout + sidebar** ✅ DONE (commit `d80d3a1`, 2026-05-20)
- Files: `frontend/app/(authed)/admin/layout.tsx` (212 LOC), `frontend/app/(authed)/admin/page.tsx` (97 LOC dashboard placeholder), `frontend/lib/role-guard.tsx` (45 LOC), `frontend/components/ui/dropdown-menu.tsx` (radix shadcn new-york port)
- Done: `(authed)/admin/layout.tsx` wraps shell — RoleGuard(allow="admin") redirect role mismatch ke landing role-spesifik (`/guru`/`/siswa`). Sidebar persisten md+ (Dashboard, Pengguna, Audit Log, Login Attempts) + active-state highlight via prefix match. Mobile: compact horizontal nav strip di header. Sticky header punya user dropdown (initials avatar dari `user.name`, label nama+email, item Profil → `/me`, Perangkat aktif → `/me/perangkat`, Logout best-effort POST `/auth/logout` → clear store → `/login` + toast). RoleGuard reusable: `allow` accept Role | Role[], render null saat redirect inflight (no flash). Dropdown-menu primitives di-port langsung (no `npx shadcn`) sesuai pola sebelumnya.
- Live verified: server `npx tsc --noEmit` PASS (TSC_OK), `next build` PASS (10 static pages — /admin=3.34 kB), curl /admin=200 + /admin/pengguna=200 (SPA fallback) + /admin/audit-log=200, lms-api active.

**Task 1.H.2 — /admin/pengguna list + filter** ✅ DONE (commit `1b34c97`, 2026-05-20)
- Files: `frontend/app/(authed)/admin/pengguna/page.tsx` (379 LOC)
- Done: TanStack Query (queryKey `['admin','users', { role, status, q, page }]`) hits `GET /api/v1/admin/users?role&status&q&page&page_size` dgn `keepPreviousData` (table tetap stabil saat page swap). Toolbar: search input debounced 300ms via `useDebounced` hook lokal, role select (admin/guru/siswa/all), status select (active/suspended/locked/all), Reset button (disabled saat no filter active). Table: Nama, Email, Role badge (violet/sky/slate tone), Status badge (emerald/amber/rose tone), Login terakhir Asia/Jakarta, Detail link → `/admin/pengguna/[id]`. 5-row skeleton saat loading; empty state membedakan "tidak ada match filter" vs "belum ada pengguna". Prev/Next pagination pakai `data.total_pages`. Page reset ke 1 setiap filter berubah. Tombol "Tambah pengguna" → `/admin/pengguna/baru` (form di 1.H.3).
- Live verified: `npx tsc --noEmit` PASS, `next build` PASS (11 static pages — /admin/pengguna=6.79 kB), curl /admin/pengguna=200, /api/v1/admin/users tanpa bearer=401 (expected).

**Task 1.H.3 — /admin/pengguna create form** ✅ DONE (commit `047790d`, 2026-05-20)
- Files: `frontend/app/(authed)/admin/pengguna/baru/page.tsx` (510 LOC)
- Done: RHF + Zod (name, email, role enum, password_strategy `manual|generate`, password conditional min 8 saat manual via superRefine). POST `/admin/users` body strict sesuai backend (`password` field di-omit saat strategy=generate). Two-card flow: form → success card setelah 201. Success card menampilkan password SEKALI per #31: copy button untuk password sendiri + combo "email / password", clipboard.writeText dengan fallback `execCommand` untuk non-secure context. Strategy chooser pakai radio cards (Generate otomatis / Ketik manual) dengan border highlight. Tombol pasca-sukses: Buka detail → `/admin/pengguna/[id]`, Tambah lagi (reset form), Selesai → list. Friendly errors: email_already_exists, weak_password, invalid_role, invalid_strategy, conflicting_password. **Tidak ada modal re-auth** — locked decision #52 cuma minta re-auth pada promote/demote (`/admin/users/:id/role`), backend create endpoint memang gak menerima `current_password`.
- Live verified: `npx tsc --noEmit` PASS, `next build` PASS (12 static pages — /admin/pengguna/baru=4.24 kB), curl /admin/pengguna/baru=200, lms-api active.

**Task 1.H.4 — /admin/pengguna detail** ✅ DONE (commits `3576c5e` BE GetUser, `e0c55a7` FE detail+dialogs, `5e2d7fc` lint fix, `6cd528e` static-export fix, 2026-05-20)
- Done: Backend `GET /api/v1/admin/users/:id` ditambah (handler + test + route registration). FE: shadcn Dialog primitive port (`@radix-ui/react-dialog`). Halaman `/admin/pengguna/detail?id=:id` (query string instead of dynamic segment karena static export tidak punya generateStaticParams runtime — rename `[id]` → `detail`). Header: nama + email + 3 badges (role/status/must-change-password) + 7 tombol aksi conditional. TabBar lightweight pakai useState (no extra deps): Detail (key-value table), Sesi Aktif (reuse `/admin/users/:id/sessions`), Riwayat Audit (dua section actor_id + target_id, masing-masing pagination), Login Attempts (filter by email, success badge, IP, UA, failure_reason). Modals: EditNameDialog (RHF+Zod, PATCH `/admin/users/:id`), ChangeRoleDialog (re-auth current_password wajib, locked #52), ResetPasswordDialog (manual/generate, password reveal once two-state form→success card, locked #31), SuspendDialog (alasan optional, destructive button), Unsuspend/Unlock confirm dialogs, RevokeSessionsDialog (alasan optional, destructive). Error handling per error code (`invalid_credentials`/`cannot_self_demote`/`cannot_self_suspend`/`weak_password`/`invalid_role`/`not_locked`/dst). TanStack Query: `setQueryData` after mutation untuk fresh data tanpa refetch + invalidate `['admin','users']` & `['admin','audit-log']`.
- Verify: tsc PASS, next build PASS (13 static pages, /admin/pengguna/detail = 11.6 kB), curl /admin/pengguna/detail=200, /api/v1/admin/users/<uuid>=401 (no auth, expected), lms-api active.
- Commit: `feat(fe-admin): user detail page with tabs + action dialogs` (e0c55a7) + lint/static-export hotfixes.

**Task 1.H.5 — /admin/audit-log + /admin/login-attempts list pages** ✅ DONE (commit `a45683e`, 2026-05-20)
- Done: Dua halaman list level-atas dengan filter form lengkap. `/admin/audit-log` — filter action (debounced 300ms), actor_id+target_id (UUID validated client-side dengan regex, invalid = skip param), since/until (HTML date inputs → RFC3339 UTC start/end-of-day). Tabel: Waktu Asia/Jakarta, action code (mono), actor_id (mono), target_id (mono), Meta (ExpandableMeta — JSON ≤80 chars rendered langsung, lebih panjang pakai toggle "Lihat detail meta"). `/admin/login-attempts` — filter email (debounced 300ms, server lowercases), success (semua/sukses/gagal native select), since/until. Tabel: Waktu, Email, Hasil badge (emerald sukses / rose gagal), IP, Perangkat (UA summarizer reuse pattern dari `/me/perangkat`), Alasan gagal. Kedua halaman pakai TanStack Query + `keepPreviousData`, page reset ke 1 setiap filter berubah, Prev/Next pagination berbasis `total_pages`. Empty state membedakan "tidak ada match filter" vs "belum ada data". 5-row skeleton saat loading.
- Verify: tsc PASS, next build PASS (13 static pages — /admin/audit-log=5.33 kB, /admin/login-attempts=5.3 kB), curl /admin/audit-log=200 + /admin/login-attempts=200, lms-api active.
- Commit: `feat(fe-admin): audit-log + login-attempts list pages` (a45683e).

#### 1.I E2E Manual Verify

**Task 1.I.1 — Bootstrap admin → bikin guru + siswa → login keduanya**
- Run on server: seed-admin → login via FE → bikin akun guru + siswa → login keduanya (force change pw flow) → dashboard nampil
- Verify: manual checklist + screenshot
- Commit: `docs: fase 1 e2e manual checklist passed`

**Task 1.I.2 — Verifikasi suspend kick session aktif + promote re-auth**
- Suspend user yang lagi login → next request → 401 + redirect login
- Promote guru → admin → modal re-auth muncul, salah pass tolak, bener jalan
- Commit: `docs: fase 1 e2e security checks passed`

---

### Fase 2 — Kelas, Enrollment & Bulk Import (3-4 hari)

#### 2.A Schema Kelas + Enrollment

**Task 2.A.1 — Migration `000003_kelas_enrollment.up.sql`** ✅ DONE (commit `1964b7b`, 2026-05-20)
- Tables: `kelas`, `enrollment`, `import_jobs`
- Indexes per Section 6.3
- Verify: migrate up + `\dt`
- Commit: `feat(migrations): 000003 kelas + enrollment + import_jobs`
- Shipped: 3 tabel + `kode_invite` UNIQUE + indexes (`idx_kelas_guru_id`, `idx_enrollment_siswa_id`, `idx_import_jobs_admin_status_expires`) + trigger `kelas_set_updated_at` (reuse `set_updated_at()` dari 000002). FK: kelas.guru_id RESTRICT, enrollment CASCADE, import_jobs.admin_id SET NULL. Verified di server: `migrate up` 54ms, schema_meta=`000003_kelas_enrollment`, 9 tabel total.

**Task 2.A.2 — Models + repo Kelas/Enrollment/ImportJob** ✅ DONE (commit `1964b7b`, 2026-05-20)
- Files: `backend/internal/kelas/{model,repo}.go`, `backend/internal/importjob/{model,repo}.go` (catat: `importjob` bukan `import` — Go reserved keyword)
- Verify: build
- Commit: `feat(kelas): GORM models + repo`
- Shipped: `Kelas` + `Enrollment` + enum `JoinedVia`/`EnrollmentStatus`; `ImportJob` + enum `Status`. `kelas.Repo`: Create, FindByID, FindByKodeInvite, ListByGuru/All (filter archived), UpdateBasic dgn optimistic concurrency (`WHERE id=? AND version=?` + reprobe → `ErrVersionConflict` vs `gorm.ErrRecordNotFound`), Archive/Unarchive (idempotent guard), Enroll dgn ON CONFLICT DO NOTHING returning `(inserted bool, err)`, FindEnrollment, ListEnrollmentsByKelas/Siswa, RemoveEnrollment (soft via status=removed). `importjob.Repo`: Create, FindByID(+ForAdmin scope), ListByAdmin, SetStatus (optional confirmed/completed timestamps), SetCounts/CredentialsPath/ErrorsJSON, ExpirePreviewBefore (transaction + `clause.Locking{Strength:"UPDATE"}` + bulk update). Verified server: `go build ./... && go vet ./... && go test ./...` semua hijau, no new deps.

#### 2.B Kelas CRUD (guru)

**Task 2.B.1 — Generate kode invite unik (6-char alnum)** ✅ DONE 2026-05-20
- Files: `backend/internal/kelas/code.go` + `code_test.go`
- Commit: `c14640d` (charset fix `9edba39` — drop `8`+`9` ambig sama `B`+`g`)
- Shipped: `GenerateKodeInvite(ctx, repo)`, charset `ACDEFGHJKMNPQRTUVWXYZ234567` (27 chars, 6 length = 387M kombinasi), `crypto/rand` source, max 10 retry via `repo.FindByKodeInvite`, `ErrKodeInviteCollision` saat exhausted. Test pakai `fakeFinder` mock + ambiguous-chars guard.

**Task 2.B.2 — Kelas CRUD service + handler** ✅ DONE 2026-05-20
- `GET /kelas` (guru → milik sendiri, admin → semua, query `include_archived=true|false`, pagination `page`+`page_size`)
- `POST /kelas` (guru-only: nama wajib, deskripsi opsional, default bobot 50/50)
- `GET /kelas/:id` (ownership guard: guru hanya kelasnya, admin semua)
- `PATCH /kelas/:id` (PARTIAL — body wajib `nama`+`version`; `deskripsi`/`bobot_*` opsional via pointer; mismatch → 409 `version_conflict`; bobot total ≠ 100 → 400 `invalid_bobot`)
- `POST /kelas/:id/archive` (idempotent: 409 `already_archived` kalau udah)
- `POST /kelas/:id/duplicate` (reduced scope: copy basic fields + regenerate kode invite, version=1, no archive carry; child catalog Bab/Materi dst masuk Fase 3)
- Optimistic concurrency via `WHERE id=? AND version=?` + auto bump version
- Audit log: `kelas_created`/`kelas_updated`/`kelas_archived`/`kelas_duplicated` dgn `target_kelas_id` terisi (siap untuk locked decision #59 guru audit scope)
- Middleware order: `BearerAuth → ForceChangePassword → RoleGuard(admin,guru)`
- Commit: `c14640d` (CRUD), `9edba39` (charset fix), `620594f` (PATCH partial fix — pointer fields)
- Verified server: build/vet/test PASS; E2E smoke 13 test scenario semua hijau (create/list/get/PATCH partial nama-only/PATCH bobot-only/version conflict/invalid bobot/duplicate/archive/cross-guru forbidden)

**Task 2.B.3 — FE guru: list kelas + create form** ✅ DONE 2026-05-20
- Files: `frontend/lib/kelas-api.ts` (typed API client), `frontend/app/(authed)/guru/layout.tsx` (shell + RoleGuard guru), `frontend/app/(authed)/guru/page.tsx` (dashboard), `frontend/app/(authed)/guru/kelas/page.tsx` (list + create dialog)
- Commit: `e0a84d3`
- Shipped: typed API wrapper (`listKelas/createKelas/getKelas/updateKelas/archiveKelas/duplicateKelas`); guru shell mirror dari admin (sidebar Dashboard+Kelas, dropdown profil/perangkat/logout); landing dashboard (total kelas + 3 recent kelas snapshot via TanStack Query); list view card grid 1/2/3 responsive dgn filter `include_archived`, pagination Prev/Next, kode invite copy-to-clipboard, archived badge; create dialog react-hook-form + zod (total bobot validasi = 100, default 50/50, friendly error mapping). Detail button DISABLED — Task 2.B.4 wire-up.
- Verified server: npm typecheck PASS, lint clean (1 warning lama di role-guard pre-existing), `npm run build` static export 17 pages (termasuk /guru + /guru/kelas), Fiber serve `/guru.html` + `/guru/kelas.html` → 200.

**Task 2.B.4 — FE guru: kelas detail (tab placeholder Siswa/Pengaturan/Pengumuman) + edit pakai version + duplicate button** ✅ DONE 2026-05-20
- Files: `frontend/app/(authed)/guru/kelas/detail/page.tsx` (query-param based detail, mirror /admin/pengguna/detail) + `frontend/app/(authed)/guru/kelas/page.tsx` (wire Detail link)
- Commit: `a0aac67` (detail page + duplicate/archive dialogs), `78e8832` (escape JSX double-quotes lint fix)
- Shipped: kelas detail page route `/guru/kelas/detail?id=:id` (static export friendly — pakai `useSearchParams` bukan dynamic [id] segment). Header: nama + status badge Aktif/Diarsipkan + kode invite copy-to-clipboard + tombol Refresh/Duplikat/Arsipkan. Tab nav Pengaturan/Siswa/Pengumuman (Siswa & Pengumuman placeholder pointer ke Task 2.C/2.D + Fase 3). Pengaturan tab: form edit (react-hook-form + zod) untuk nama/deskripsi/bobot dgn validasi total = 100, kirim PATCH dgn `version` field. 409 version_conflict → friendly toast + invalidateQueries → refetch → form auto re-sync via useEffect+form.reset. Form dinonaktifkan saat archived. ArchiveDialog dgn konfirmasi destructive (idempotent: 409 already_archived). DuplicateDialog dgn input `new_nama` opsional, success → router.push ke detail kelas baru. Wire link Detail di `KelasCard` (replace tombol disabled).
- Verified server: typecheck PASS, build static export 18 pages termasuk `/guru/kelas/detail` (6.34 kB), Fiber serve `/guru/kelas/detail.html` → 200.

#### 2.C Enrollment

**Task 2.C.1 — Siswa join via kode (rate limit 10/min)** ✅ DONE 2026-05-20
- Files: `backend/internal/kelas/enrollment_service.go` (+test), `backend/internal/kelas/enrollment_handler.go` (+test), `backend/internal/kelas/rate_limit.go`. Wire `cmd/server/main.go`.
- Commit: `2d94288` (feat) + `0eaec1e` (lint fix unused import)
- Shipped: `POST /api/v1/siswa/kelas/join` body `{kode_invite}`. Mounted under `/siswa` group dgn `BearerAuth → ForceChangePassword → RoleGuard(siswa)`. Rate-limit `JoinKodeRateLimit(10)` per (IP, user_id) per minute. Service flow: trim+UPPER kode (charset uppercase-only, tahan typo lowercase) → FindByKodeInvite → cek archived → cek prior removed enrollment (no silent re-activate, surface ErrEnrollmentRemoved) → repo.Enroll ON CONFLICT DO NOTHING → audit `siswa_joined_kelas` dgn `target_kelas_id` (locked decision #59 prep). Idempotent: pertama join 201 inserted=true, ulang 200 inserted=false. Sentinels: ErrKodeInviteEmpty/NotFound/KelasArchived/EnrollmentRemoved/AlreadyEnrolled. kelasRepo interface extended dgn Enroll + FindEnrollment; mockRepo + stubSvc updated.
- Verified server: build/vet/test PASS; E2E smoke 10 scenario hijau (lowercase normalize/idempotent/wrong kode 404/empty 400/role-guard 403 untuk guru/archived 409/audit log siswa_joined_kelas + siswa_join_kelas_noop terisi/enrollment row active+kode di DB).

**Task 2.C.2 — Admin assign siswa ke kelas (bulk supported)** ✅ DONE 2026-05-20
- `POST /admin/kelas/:id/enroll` body `{siswa_ids: []}`
- JoinedVia=admin
- Audit
- Commit: `feat(admin): assign siswa ke kelas`
- Shipped: `POST /api/v1/admin/kelas/:id/enroll` di file baru `backend/internal/admin/kelas_enroll.go` (struct `KelasEnrollHandler` terpisah supaya tes admin existing aman). Body `{siswa_ids: []}` (max 100). Hard precondition (4xx): invalid kelas id, kelas not found, kelas archived, body kosong/oversize/malformed. Per-siswa klasifikasi 3 bucket: `enrolled` (insert baru), `already_enrolled` (active prior), `invalid` dgn 6 reason codes — `invalid_uuid`, `duplicate_in_request`, `user_not_found`, `not_siswa`, `user_inactive`, `enrollment_removed`. Reuse `kelasRepo.Enroll(JoinedViaAdmin)` + `FindEnrollment` + `auth.Repo.FindUserByID/LogAudit` lewat 2 interface kecil (`kelasEnrollUserRepo`, `kelasEnrollKelasRepo`). Audit log per-siswa action `admin_assigned_siswa_to_kelas` dgn `meta.result` (`enrolled`/`already_enrolled`/`invalid_<reason>`), target_user_id=siswa, target_kelas_id=kelas. Race protection: insert race antara FindEnrollment dan Enroll diklasifikasi `already_enrolled`.
- Verified server: build/vet/test PASS (handler tests 5 case: mixed happy path + 5 invalid reasons, invalid uuid, kelas not found, kelas archived, empty/oversize body, malformed body); E2E smoke 10 scenario hijau (mixed payload 1 enrolled / 1 already / 6 invalid w/ all 6 reason codes asserted, idempotent re-call, kelas not found 404, archived 409, oversize 400, role mismatch guru→403, audit 6 rows ke-record, DB enrollment row joined_via=admin status=active).

**Task 2.C.3 — FE siswa dashboard + join kelas form**
- `frontend/app/siswa/page.tsx` (list kelas siswa) + `frontend/app/siswa/gabung/page.tsx`
- Visual + e2e
- Commit: `feat(fe-siswa): dashboard + join kelas`

**Task 2.C.4 — FE guru tab Siswa di kelas detail**
- List enrollment + remove button (admin scope only? lock decision: guru read-only di MVP)
- Commit: `feat(fe-guru): tab Siswa di kelas detail`

#### 2.D Bulk Import CSV

**Task 2.D.1 — CSV parser + validator**
- Files: `backend/internal/import/parser.go`
- Parse rows, validate (email format, name not empty, nama_lengkap, dst), dedupe by email
- Test dengan fixture CSV valid + invalid
- Commit: `feat(import): CSV parser + validator`

**Task 2.D.2 — Storage convention + upload CSV**
- `POST /admin/import-jobs` multipart file → simpan ke `./storage/uploads/import/<uuid>.csv`, parse, generate PreviewRowsJSON, insert ImportJob status=preview ExpiresAt=now+1h
- Response: `{job_id, valid_count, invalid_count, preview_url}`
- Commit: `feat(import): upload + preview ImportJob`

**Task 2.D.3 — Resume + cancel preview**
- `GET /admin/import-jobs/:id` (status preview only) → return PreviewRowsJSON
- `POST /admin/import-jobs/:id/cancel` → status=expired + delete file
- Commit: `feat(import): resume + cancel preview`

**Task 2.D.4 — Confirm import (preview → processing → completed)**
- `POST /admin/import-jobs/:id/confirm`
- Logic: status=processing → loop rows: bcrypt random pass → insert User → save credentials.csv ke `./storage/uploads/import/<uuid>-credentials.csv` → status=completed CompletedAt=now
- Audit log per user created
- Commit: `feat(import): confirm flow with credentials.csv`

**Task 2.D.5 — Download credentials.csv (one-shot, signed URL)**
- `GET /admin/import-jobs/:id/credentials.csv` (cek admin owner + ExpiresAt)
- Auto-cleanup file 1 jam after CompletedAt
- Commit: `feat(import): credentials download with TTL`

**Task 2.D.6 — Hourly cleanup cron**
- Files: `backend/internal/import/cleanup.go` (run on app start: ticker 1h)
- Logic: find ImportJob status=preview AND ExpiresAt < now → set expired + delete file
- Commit: `feat(import): hourly expired cleanup`

#### 2.E FE Admin Import

**Task 2.E.1 — /admin/import-csv page (drag-and-drop upload)**
- Visual: file picker, parsing progress, error rows
- Commit: `feat(fe-admin): import CSV upload`

**Task 2.E.2 — Preview tabel persistent (admin bisa close + balik)**
- Read job_id dari URL, GET preview, render table
- Commit: `feat(fe-admin): import preview persistent`

**Task 2.E.3 — Confirm + modal sukses + download credentials.csv**
- Visual: confirm button → POST → poll status → modal download
- Commit: `feat(fe-admin): import confirm + credentials download`

#### 2.F E2E Manual Fase 2

**Task 2.F.1 — Bikin kelas + invite kode + siswa join**
- Manual: guru login → bikin kelas → copy kode → siswa login → join → muncul di dashboard
- Commit: `docs: fase 2 e2e flow guru-siswa passed`

**Task 2.F.2 — Bulk import 5 siswa**
- Manual: bikin sample.csv → upload → preview → confirm → download credentials → 5 user baru bisa login
- Commit: `docs: fase 2 e2e bulk import passed`

---

### Current Next Step (Section 18)

**Berikut: Task 2.C.3 — FE siswa dashboard + join kelas form.** Task 2.C.2 SELESAI: backend `POST /api/v1/admin/kelas/:id/enroll` hidup (commit `a79903b`), 3 buckets `enrolled`/`already_enrolled`/`invalid` dgn 6 reason codes (`invalid_uuid`, `duplicate_in_request`, `user_not_found`, `not_siswa`, `user_inactive`, `enrollment_removed`), max 100 siswa_ids per request, audit per-siswa `admin_assigned_siswa_to_kelas` dgn meta.result. E2E 10 scenario hijau (partial success mixed bucket, idempotent re-call, kelas not found 404, archived 409, oversize 400, role mismatch 403). Task 2.C.3 spec: FE siswa dashboard di `frontend/app/(authed)/siswa/page.tsx` (RoleGuard siswa, list kelas yang sudah dijoin via `GET /siswa/kelas` — endpoint perlu dibuat dulu) + form join via kode invite di `frontend/app/(authed)/siswa/join/page.tsx` (input kode, POST `/siswa/kelas/join`, error mapping 404/409 ke pesan UX yang ramah). Backend support: tambah `GET /api/v1/siswa/kelas` (list enrollment siswa current via `kelasRepo.ListEnrollmentsBySiswa` + JOIN kelas detail) sebelum FE. Commit: `feat(siswa): dashboard kelas + join form`. Setelah ini Task 2.C.4 FE guru tab Siswa di kelas detail.

QA Fase 1 v0.7.2 ditunda — lu bisa run kapan-kapan via creds dummy yang udah di-seed; cara reset/seed ulang ada di catatan terdahulu (`ssh rdpkhorur "cd /home/ubuntu/lms/backend && set -a && source /home/ubuntu/lms/.env && set +a && go run ./cmd/seed-dummy"`).

> Catatan eksekusi: pakai inline approach default. Kalau task tertentu butuh research/scaffolding berat (mis. 1.G.2 auth interceptor + 1.H.4 admin user detail), bisa delegasi ke `codex` atau `claude-code` per task.

> Subagent flow note: Codex `--full-auto` fail di Windows (CreateProcessWithLogonW 1056) — pakai `--yolo`. Codex kadang post-commit tweak kosmetik (em-dash dll), kita amend untuk fix konsistensi (Option B pattern).
