# LMS Project — Roadmap & Living Plan

> Status: v0.9.8 — **Fase 4 ✅ DONE 14/14 = 100% complete** 2026-05-21. 4.A.4 tugas duplicate `3600188` (R2 CopyObject + status reset draft); 4.D.2 siswa dashboard riwayat — BE `5d160b6`+`9d5eda2` (ListMine cross-kelas JOIN) + FE `6f49e14` (/siswa/tugas page + sidebar nav); 4.E.2 pending counter — BE `a4f14a4` (GET /guru/pending-counts cumulative across guru-owned kelas) + FE `34aff41` (sidebar badge polled 30s + dashboard card). Activity feed full di-defer Fase 7 (locked #39 cursor pagination).
> Owner: User (guru) + Apis (assistant)
> Last updated: 2026-05-21 (Fase 4 closed — duplicate + riwayat siswa + pending counter shipped)

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

## 0. Locked Decisions (v0.9.1)

| # | Keputusan | Pilihan |
|---|-----------|---------|
| 1 | Skala guru | Multi-teacher (flat, no multi-tenant) |
| 2 | Backend | Go + **Fiber** + GORM + PostgreSQL |
| 3 | Frontend | Next.js 14 + TS + Tailwind + shadcn/ui + Zustand + TanStack Query |
| 4 | Frontend build mode | **Static export** (`output: 'export'`) — di-serve oleh Go Fiber sebagai static, mirip fb-bot |
| 5 | Jenis soal ujian | Pilihan Ganda (MCQ) saja |
| 6 | Storage materi | **Cloudflare R2** (S3-compatible object storage) — semua file user (materi, tugas, submission, soal, import CSV) di R2, tidak di local disk |
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
| 44 | Health/readiness split | `/api/v1/healthz` (liveness, return 200 selalu kalau process hidup, no DB) + `/api/v1/readyz` (readiness, cek DB ping + R2 reachable via `HeadBucket` + return 503 kalau ada yang fail). systemd `ExecStartPost` pakai readyz. Loadbalancer/uptime monitor pakai readyz. |
| 45 | Remedial snapshot policy | Saat reset attempt: HasilSoalBab/HasilUjian + JawabanBab/Jawaban + SoalAssignment di-soft-delete (`DeletedAt`). Attempt baru bikin **assignment baru fresh** (refetch SoalBab/Soal aktif sekarang). AuditLog catat: actor, target_siswa, target_bab/ujian, reason, jumlah_soal_lama, jumlah_soal_baru, soal_diff (added/removed IDs). Siswa lihat soal baru — penting kalau guru udah edit/tambah soal antar attempt. |
| 46 | File upload hardening | (1) Mime detect via `http.DetectContentType` (sniff isi 512 byte pertama, jangan trust client `Content-Type`); validate SEBELUM upload ke R2. (2) Allowlist eksplisit per kategori: tugas (pdf, docx, jpg, png, zip), gambar soal (jpg, png, webp), materi (pdf, mp4, jpg, png), submission (pdf, docx, jpg, png, zip), import (csv only). (3) Filename sanitize: object key di R2 = `<kategori>/<uuid>.<ext>` (lihat #58); `OriginalFilename` di DB column terpisah untuk download UX. (4) Gambar soal: resize on upload (max 1920px, quality 85) pake `disintegration/imaging` SEBELUM `PutObject` ke R2. (5) PDF tugas max 20MB, gambar 5MB, materi video max 100MB. (6) Block executable mime explicit (application/x-executable, application/x-msdownload, application/x-sh). (7) R2 bucket BUKAN public — diakses lewat presigned GET URL (lihat #62). |
| 47 | Global rate limit | Selain `/auth/login` (10/15m per IP+email — locked #75 tweak from initial 5), tambahin: per-IP global 120 req/menit (Fiber `limiter` middleware), `/auth/refresh` 10/menit per refresh token, `/kelas/join` 10/menit per IP (cegah brute force kode invite), upload endpoints 30/menit per user. In-memory store cukup MVP. |
| 48 | Bab progress formula | Per siswa per bab: `progress_persen = round( (w_materi × pct_materi + w_latihan × pct_latihan + w_ulangan × pct_ulangan + w_tugas × pct_tugas) / total_w )` dengan default bobot equal (25/25/25/25), skip komponen yang gak ada (mis. bab tanpa tugas → bobot tugas dropped, sisanya re-normalize). pct_materi = `materi_dibaca / total_materi`. pct_latihan = `1 if ada attempt latihan else 0`. pct_ulangan = `1 if HasilSoalBab(mode=ulangan, status=submitted/expired) ada else 0`. pct_tugas = `submission_graded / total_tugas`. Display: progress bar + tooltip breakdown. |
| 49 | Request ID & observability | Middleware bikin `X-Request-ID` (UUID v4 atau dari header kalau ada) di semua request, propagate ke slog context (`request_id`, `user_id`, `path`, `method`). Response header echo balik. Error response include `request_id` supaya user bisa report ke admin. Dipasang dari Fase 0, bukan Fase 8. |
| 50 | Test coverage target | Per package backend: auth/admin/soalbab/ujian/nilai minimal 70% line coverage. Frontend critical path (login, form bikin user, kerjain ulangan, submit tugas) wajib ada Vitest unit + Playwright E2E (Fase 8). CI gate: `go test -cover ./...` fail kalau di bawah threshold. |
| 51 | Data retention policy | LoginAttempt 30 hari (auto-cleanup). AuditLog **forever** (compliance, kalau perlu archive ke cold storage di v1). Submission file: retain sampai kelas di-archive + 1 tahun, lalu hard-delete file (DB row tetap untuk nilai history). HasilSoalBab/HasilUjian deleted_at: hard delete setelah 1 tahun + audit log. Backup pg_dump: retain 30 hari rolling, monthly archive 1 tahun. Cleanup task daily cron di server. |
| 52 | Multi-admin promotion | Admin baru bisa dibikin via `/admin/users` create form (role=admin). Tapi promote/demote dari guru→admin atau sebaliknya wajib **re-auth**: admin yang melakukan harus re-input password sendiri di modal (POST `/admin/users/:id/role` { role, current_password }). AuditLog catat actor + target + role_lama + role_baru. Tidak ada self-demote (admin gak bisa demote dirinya sendiri kalau dia satu-satunya admin). |
| 53 | Admin lock-out recovery | `cmd/seed-admin` cuma jalan kalau belum ada admin. Kalau admin satu-satunya kena lock/forget password: `cmd/reset-admin` CLI minta email + password baru, override lewat akses fisik server. Production: butuh SSH access. AuditLog: `actor_id=NULL` + `action='admin_reset_via_cli'`. Tidak ada self-service recovery — by design (akses fisik = trust boundary). |
| 54 | CSV import preview persistence | Upload CSV → R2 `import/<uuid>.csv` (lihat #58/#61) → ImportJob status=`preview` (PreviewRowsJSON jsonb + valid_count + invalid_count + ObjectKey). Confirm → status=`processing` → `completed`. Cancel atau timeout 1 jam tanpa confirm → status=`expired`, `s3.DeleteObject` + DB row tetap untuk audit. Admin bisa close tab tanpa lose preview state — reload `/admin/pengguna/import` resume preview kalau status=preview. |
| 55 | Activity feed cursor | `GET /guru/feed?cursor=BASE64&limit=20` pakai opaque cursor `(at_unix_micro, id)` di-base64. Default 20 item. Response: `{ events: [...], next_cursor }`. Polling 30s pakai `cursor=null` (latest 20) buat refresh; load-more pakai cursor. Cegah duplicate/skip kalau dua event timestamp sama. |
| 56 | Concurrent edit version | Tambah field `Version int default 1` di Bab, Kelas, SoalBab, Soal, UlanganBabSetting, Ujian. Increment tiap update. Request PATCH wajib include `version` di body, backend cek match → reject 409 + `{ error, current_version }` kalau mismatch. UI tampil "Konten ini diubah orang lain — refresh dulu". Cegah dua tab/device guru sama overwrite tanpa sadar. |
| 57 | Auth boundary explicit | **Endpoint tanpa auth (anon allowed):** `/auth/login`, `/auth/refresh`, `/healthz`, `/readyz`, static files (`/`, `/login`, `/lupa-password`). **Semua lain butuh auth.** Tambahan: enrollment check di endpoint kelas-scope (siswa hanya akses kelas yang dia enrolled, guru hanya akses kelas yang dia owner). Middleware order: ratelimit → request-id → auth → role-guard → enrollment-guard. |
| 58 | Storage path convention | **R2 object key** dengan kategori prefix: `<kategori>/<uuid>.<ext>` dimana kategori = `tugas`, `soal`, `materi`, `submission`, `import`. Single bucket `lms-prod` (per-env bucket — `lms-dev` untuk staging). Tidak hierarki by bab/kelas — orphan cleanup lebih simple via "select uuid not in (select object_key from <ref tables>)". `OriginalFilename` disimpan di DB column terpisah untuk download UX. Saat duplicate kelas/bab → `CopyObject` ke key uuid baru (jangan share). DB column rename: `FilePath` → `ObjectKey` (semua referensi file di tabel Materi/Tugas/Submission/Soal/SoalBab/ImportJob). |
| 59 | Guru audit scope | `GET /guru/kelas/:id/audit?action=<filter>&limit=50` — guru bisa lihat audit log yang berkaitan kelas miliknya: action subset (`hasil_reset`, `bab_archived`, `bab_published`, `siswa_kicked`, `tugas_deleted`). Hanya entry dengan `target_kelas_id=<id>`. Berguna untuk transparansi kalau admin bantu reset attempt. |
| 60 | Frontend env strategy | `NEXT_PUBLIC_API_BASE` di-bake at build time (static export limit). **Production**: rebuild dengan `NEXT_PUBLIC_API_BASE=/api/v1` (same-origin). **Dev**: `.env.development.local` set `NEXT_PUBLIC_API_BASE=http://localhost:8200/api/v1`. Dokumentasikan di `docs/DEPLOY.md`: kalau base URL berubah, FE wajib rebuild. |
| 61 | Storage backend — Cloudflare R2 | **Backend**: `aws-sdk-go-v2` (`config`, `credentials`, `service/s3`, `service/s3/types`, `feature/s3/manager`) pointing ke R2 endpoint `https://<account_id>.r2.cloudflarestorage.com` dengan `region="auto"` + path-style addressing (`UsePathStyle=true`). **Bucket strategy**: single bucket per environment — `lms-prod` (production) / `lms-dev` (workspace/staging). Object key format mengikuti #58. **Env vars** (semua di `.env`, jangan di-commit): `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET` (e.g. `lms-prod`), `R2_ENDPOINT` (auto-derive dari ACCOUNT_ID kalau kosong), `R2_PRESIGN_TTL_SECONDS` (default 900 = 15 menit). **Tidak ada public R2 dev URL / custom domain** di MVP — semua akses lewat presigned URL (lihat #62). **Health**: `/readyz` panggil `s3.HeadBucket(R2_BUCKET)` sekali on-startup + cache hasil 30 detik (jangan call tiap request). **Wrapper package**: `backend/internal/storage/r2.go` expose interface `Storage { PutObject, GetObject, HeadObject, DeleteObject, CopyObject, PresignGet, PresignPut }` supaya domain code (kelas/materi/tugas/...) gak depend langsung ke aws-sdk. **Test**: stub `Storage` interface di domain tests; integration test panggil R2 real cuma di task khusus (gated by env flag). |
| 62 | Upload flow & access control | **Upload (MVP)**: client → `POST /api/v1/<scope>/upload` (multipart) → backend validate (mime sniff, size, role/ownership) → resize image kalau perlu → `s3.PutObject(bucket, <kategori>/<uuid>.<ext>)` → insert row dgn `ObjectKey` + `OriginalFilename` + `MimeType` + `SizeBytes` → return `{ object_key, original_filename, size_bytes }`. **Tidak ada direct browser→R2 upload** di MVP — bandwidth dobel diterima trade-off untuk simpel + auth langsung di backend. (Future v0.9+ bisa migrate ke `PresignPut` direct upload tanpa breaking FE contract karena FE cuma kirim multipart.) **Download/view**: client minta `GET /api/v1/files/:object_key/url` (atau endpoint scoped: `GET /api/v1/tugas/:id/file-url`) → backend cek auth + ownership/enrollment → `PresignGet(object_key, ttl=R2_PRESIGN_TTL_SECONDS)` → return `{ url, expires_at }` → client redirect / `<a href>` / `<img src>` ke URL itu. **Inline vs attachment**: presigned URL set `ResponseContentDisposition` ke `inline; filename="<original>"` untuk gambar (di-render langsung di `<img>`) dan `attachment; filename="<original>"` untuk PDF/doc/zip (force download). **Caching**: presigned URL gak boleh di-cache lebih lama dari TTL. FE TanStack Query: `staleTime = 10 * 60 * 1000` (10 menit, di bawah TTL 15) supaya selalu fresh URL sebelum expired. **Audit**: log presign issuance untuk file sensitif (submission, credentials.csv) — `action='file_url_issued'`, `target_id=<entity_id>`, `meta={object_key, ttl}`. |
| 63 | Materi tipe (Fase 3) | **Lock 3 tipe saja**: `pdf` (upload ke R2 `materi/<uuid>.pdf`), `youtube` (link video, simpan video_id 11-char saja), `markdown` (text body inline di DB). **Drop direct video upload** dari Section 10 line 918 (bandwidth + cost R2 mahal untuk video; YouTube embed cukup untuk Fase 3). Field `Tipe enum('pdf','youtube','markdown')`. Untuk `pdf`: pakai `ObjectKey/OriginalFilename/MimeType/SizeBytes`. Untuk `youtube`: simpan video_id di `Konten` (text). Untuk `markdown`: simpan body markdown di `Konten` (text). Future v0.9 bisa tambah `audio`/`video` tipe kalau perlu. |
| 64 | Materi PDF size cap | **Max 20MB per file** (e-book chapter, slide PDF cukup). Constant `MaxMateriBytes = 20 * 1024 * 1024` di `backend/internal/storage/r2.go` atau scoped per-domain. Mime allowlist: `application/pdf` only. Reject 413 `payload_too_large` kalau exceed. (CSV import tetap 5MB cap terpisah, tidak share constant.) |
| 65 | YouTube URL validation | **Strict regex parse → simpan video_id 11-char saja**, embed via `https://www.youtube-nocookie.com/embed/<id>` (privacy-enhanced, no tracking cookie sampai user click play). Backend helper `parseYouTubeID(url string) (string, error)` di `backend/internal/materi/youtube.go`. Accept formats: `youtube.com/watch?v=`, `youtu.be/`, `youtube.com/shorts/`, `youtube.com/embed/`. Reject 400 `invalid_youtube_url` kalau tidak match (FE friendly error: "Tempel link YouTube standar — `youtube.com/watch?v=...` atau `youtu.be/...`"). FE simpan + display embed URL hasil reconstruct dari video_id. |
| 66 | Pengumuman dismiss state | **Passive timestamp display** — no read receipt table, no dismiss state per siswa (jawaban Section 12 #2). Pengumuman muncul di list kelas + bab, sort `created_at DESC`. UI: badge "Baru" kalau `created_at` < 7 hari sejak now; badge hilang setelah > 7 hari. Per-siswa read state out-of-scope MVP (defer ke v0.9+ kalau perlu). Tidak ada `MateriRead`-equivalent untuk Pengumuman. |
| 67 | Bab reorder UX | **Bulk update `urutan` field** via `POST /kelas/:id/bab/reorder` body `{order: [bab_id1, bab_id2, ...]}`. Backend: transaction loop `UpdateColumn("urutan", index)` per bab_id + cek `kelas_id=<:id>` ownership + cek `version` per row (tolak 409 kalau ada bab di-edit paralel). Lebih simpel dari before/after pivot pattern. FE: drag-and-drop list pakai `@dnd-kit/core` + optimistic update + invalidate on settled. |
| 68 | Bab progress Fase 3 partial | **Fase 3 progress = materi-only**, re-normalize otomatis sesuai locked decision #48. `progress_persen = round(pct_materi × 100)` dimana `pct_materi = materi_dibaca / total_materi` (0 kalau bab kosong materi). Komponen latihan/ulangan/tugas masih 0/null di Fase 3 (belum implement) — auto-skip via re-normalize rule #48. Formula final lengkap aktif setelah Fase 4 + 5 + 6 ship. UI: progress bar + tooltip "Berdasarkan materi dibaca (N/M)". |
| 69 | Materi cleanup strategy | **Hard delete on Materi.Delete** — DeleteObject R2 dipanggil di service.Delete (mirror Cancel pattern dari ImportJob 2.D.4). Compensating delete: kalau DB Delete fail setelah R2 DeleteObject, log `audit.materi_r2_orphan` + tetap return 500 (R2 orphan toleransi). Skip cron untuk Fase 3 — kalau ada race/orphan akumulasi, audit log + manual purge. Reuse Cleaner pattern (skill `go-cleanup-cron-ctx-bound`) di Fase 8 polish kalau orphan rate signifikan. Bab archive (Status=archived) tidak hapus materi — siswa cuma gak lihat lagi (filter via Bab.Status='published'). |
| 70 | Submission resubmit strategy | **Single-row + version bump** — 1 row Submission per `(TugasID, SiswaID)` UNIQUE constraint. Resubmit overwrite konten + bump `Version` field (locked #56 pattern). History per-attempt out-of-scope MVP — audit trail di `AuditLog` cukup (action `submission_submitted`/`submission_resubmitted` w/ meta `{old_object_keys, new_object_keys}`). Compensating R2 delete buat attachment lama (locked #69 pattern) saat resubmit. Future v1: kalau perlu multi-attempt history, bikin tabel `SubmissionAttempt` dgn FK ke `Submission`. |
| 71 | Late submission gating | **Hard-block kalau `IzinkanLate=false`** — backend reject 403 `deadline_passed` di Submit endpoint kalau `now > Tugas.Deadline AND Tugas.IzinkanLate=false`. Kalau `IzinkanLate=true`, accept submission + set `Submission.IsLate=true` + audit `submission_late`. FE siswa tampilkan banner "Late penalty xx%" sebelum submit kalau lewat deadline + IzinkanLate=true; disable submit button kalau IzinkanLate=false (defensive — backend tetap source of truth). Penalty calc di grading: `NilaiSetelahPenalty = round(NilaiAsli × (1 - PenaltyPersen/100), 2)`. Penalty 0% = gak ada potongan (allow late tapi free). Audit `tugas_graded` capture `nilai_asli`/`penalty_persen`/`is_late`/`nilai_setelah_penalty`. |
| 72 | Submission attachment policy | **Optional by default** — Submission boleh punya 0..N attachment, masing-masing object di R2 `submission/<uuid>.<ext>`. Per-tugas guru bisa set `WajibAttachment bool default false` — kalau true, Submit reject 400 `attachment_required` kalau `len(attachments)==0`. Allowlist mime ikut locked #46 (submission: pdf, docx, jpg, png, zip). Cap size 20MB per file (mirror materi PDF locked #64). Cap count: max 5 attachment per submission (anti-abuse). Konten teks (`Catatan`) optional, max 50KB (mirror pengumuman/materi markdown). Resubmit: replace seluruh attachment set (DELETE old object keys + INSERT new dalam tx). |
| 73 | Submit transition concurrency | **Pakai `SELECT ... FOR UPDATE` di tx + idempotent guard** mirror locked #43. Submit endpoint: BEGIN → SELECT submission FOR UPDATE → cek `Status NOT IN ('graded','returned')` (kalau graded → 409 `already_graded`, return existing) → cek deadline + IzinkanLate (locked #71) → UPSERT submission row + R2 PutObject attachments + bump Version → audit log → COMMIT. Compensating R2 cleanup di defer kalau tx rollback. Grade endpoint juga pakai `FOR UPDATE` + cek `Status='submitted'` sebelum update ke `graded`. Status enum: `draft` (siswa save draft, optional MVP) | `submitted` (siswa submit, awaiting grade) | `graded` (guru kasih nilai) | `returned` (guru return for revision, optional MVP — defer). |
| 74 | Tugas attachment policy | **Lampiran soal/instruksi guru** — Tugas optional punya 0..N attachment di R2 `tugas/<uuid>.<ext>`. Allowlist mime ikut locked #46 (tugas: pdf, docx, jpg, png, zip). Cap size 20MB per file. Cap count: max 5 attachment per tugas. Hard delete on Tugas.Delete dgn compensating R2 cleanup (locked #69 pattern). Kalau Tugas di-archive (status=archived, similar bab), attachment tetap ada di R2 — siswa enrolled tetap bisa download via presigned URL untuk submission yg udah grade history. Future cleanup: archive + 1 tahun → hard delete (locked #51 retention). |
| 75 | Login rate limit decay + threshold | **Threshold 10/15m per (email+IP)** (raised dari initial 5, locked #47 update). **Auto-clear failed attempts on success**: ketika user login sukses, password change sukses, atau admin password reset sukses → DELETE failed `login_attempts` rows for that email dalam window 15 menit. Counter hard-lock akun (`maxCumulativeFailedLogins=10` di `users.failed_login_count`) tetap berlaku untuk attack pattern beneran (cumulative across windows). **Two-layer consistency**: layer DB (`Service.Login` + `failed_login_attempts`) DAN layer Fiber in-memory limiter (`auth.LoginRateLimit`) sama-sama hanya count failed (`SkipSuccessfulRequests=true`); success ga consume budget di mana pun. Rationale: typo + browser autofill old-password + multi-device combine sering trigger lock untuk real users; pola "user proves they know password → counter reset" mirror pattern industri. UX impact: setelah login sukses, retry dari mistype tidak bayar penalty 15 menit. Implemented commits `8ad9f60` (DB layer + threshold) + `6044d2f` (Fiber memory layer). |

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
- **File upload**: Fiber multipart → backend validate → `aws-sdk-go-v2/service/s3` `PutObject` ke Cloudflare R2 (lihat decision #61/#62)
- **Object storage**: Cloudflare R2 (S3-compatible) lewat `aws-sdk-go-v2`
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
- File: **Cloudflare R2** (S3-compatible, path-style, `region="auto"`, endpoint `https://<account_id>.r2.cloudflarestorage.com`)
- Bucket: `lms-prod` (production) / `lms-dev` (workspace/staging) — single bucket per env
- Object key: `<kategori>/<uuid>.<ext>` (kategori = `tugas`, `soal`, `materi`, `submission`, `import`)
- Akses file: presigned GET URL (TTL 15 menit) lewat backend, bucket non-public
- Upload: client → backend multipart → `s3.PutObject` (no direct browser→R2 di MVP)
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
- Materi: upload PDF (max 20MB), link YouTube (parsed video_id), teks markdown — per bab atau kelas. **3 tipe lock di Fase 3 (locked #63)** — drop direct video upload, YouTube embed cukup.
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
Materi     { ID, KelasID, BabID(*), Judul, Tipe, Konten, ObjectKey(*), OriginalFilename(*), MimeType(*), SizeBytes(*), Urutan, CreatedAt }
MateriRead { MateriID, SiswaID, ReadAt }                              // PK composite
Tugas      { ID, KelasID, BabID(*), Judul, Deskripsi, Deadline, MaxNilai, AttachmentObjectKey(*), AttachmentOriginalFilename(*), AttachmentMimeType(*), AttachmentSizeBytes(*), IzinkanLate(bool), PenaltyPersen(int 0-100), CreatedAt }
Submission { ID, TugasID, SiswaID, Konten, AttachmentObjectKey(*), AttachmentOriginalFilename(*), AttachmentMimeType(*), AttachmentSizeBytes(*), SubmittedAt, IsLate(bool), Nilai(*), NilaiSetelahPenalty(*), Feedback, GradedAt(*), Version }

// Soal Bab + gambar
SoalBab    { ID, BabID, Pertanyaan, GambarSoalObjectKey(*), GambarSoalOriginalFilename(*), OpsiA..E(*), GambarOpsiAObjectKey..E(*), GambarOpsiAOriginalFilename..E(*), JawabanBenar, Poin, Mode(latihan|ulangan), Urutan, Version(int default 1), CreatedAt }
UlanganBabSetting { BabID(PK), DurasiMenit, MulaiAt(*), SelesaiAt(*), ShuffleSoal, ShuffleOpsi, JumlahSoalRandom(*), TampilkanNilaiLangsung, IzinkanReviewSetelahSubmit(default false), WaktuBukaReview(*), Aktif, Version(int default 1) }
HasilSoalBab { ID, BabID, SiswaID, Mode(latihan|ulangan), AttemptKe, MulaiAt, SubmitAt(*), TotalNilai(*), Status(berlangsung|submitted|expired), DeletedAt(*) }
JawabanBab   { ID, HasilSoalBabID, SoalBabID, JawabanSiswa(*), Benar, Poin }
EventBab     { ID, HasilSoalBabID, Tipe(tab_switch|blur|focus|paste), At }

// Ulangan Harian + Soal bisa pakai gambar juga
Ujian      { ID, KelasID, Judul, DurasiMenit, MulaiAt, SelesaiAt, ShuffleSoal, ShuffleOpsi, JumlahSoalRandom(*), TampilkanNilaiLangsung, IzinkanReviewSetelahSubmit(default false), WaktuBukaReview(*), Version(int default 1), CreatedAt }
Soal       { ID, GuruID(pemilik bank), UjianID(*), Pertanyaan, GambarSoalObjectKey(*), GambarSoalOriginalFilename(*), OpsiA..E(*), GambarOpsiAObjectKey..E(*), GambarOpsiAOriginalFilename..E(*), JawabanBenar, Poin, Topik, Version(int default 1), CreatedAt }
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
- **Materi.Tipe (locked #63)**: enum `pdf|youtube|markdown` (3 tipe lock di Fase 3, drop direct video upload). Untuk `pdf`: pakai `ObjectKey/OriginalFilename/MimeType/SizeBytes` (max 20MB, locked #64). Untuk `youtube`: simpan **video_id 11-char saja** di `Konten` text (parsed dari URL via `parseYouTubeID`, locked #65) — embed lewat `youtube-nocookie.com/embed/<id>`. Untuk `markdown`: simpan body markdown di `Konten` text. Hard delete + R2 cleanup compensating (locked #69).
- **Tugas**: `IzinkanLate` default false. `PenaltyPersen` 0-100, jadi nilai max submission late = `MaxNilai × (100 - PenaltyPersen) / 100`.
- **Submission**: `Version` increment tiap resubmit; baris terbaru saja yang dipake (atau pakai 1 row dengan overwrite). Default: **1 row, overwrite** — hemat storage. `IsLate` di-set saat submit, `NilaiSetelahPenalty` dihitung backend pas grading.
- **SoalBab/Soal**: gambar disimpan di Cloudflare R2 — `ObjectKey` format `soal/<uuid>.<ext>` (lihat decision #58/#61), `OriginalFilename` + `MimeType` + `SizeBytes` di DB column terpisah. Gambar opsi opsional (untuk soal "pilih gambar").
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
- `GET /readyz` — readiness, cek DB ping + R2 reachable (`HeadBucket`, cached 30s). Return 503 kalau ada yang fail. Dipake uptime monitor.

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
- `POST   /admin/import-csv/:job_id/cancel` — status preview → expired + `s3.DeleteObject` ke R2 ObjectKey
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
│   │   ├── storage/          # R2 client wrapper (aws-sdk-go-v2/s3)
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
- Backend: **R2 storage client wrapper** (`internal/storage`) + bucket bootstrap (workspace bucket `lms-dev`, prod bucket `lms-prod`) + readyz `HeadBucket` cache (lihat #61)
- Backend: **ImportJob lifecycle** — upload (status=preview, PreviewRowsJSON, ExpiresAt=now+1h, CSV disimpan di R2 `import/<uuid>.csv`), GET resume preview, confirm (preview→processing→completed), cancel (preview→expired + `s3.DeleteObject`), hourly cleanup expired jobs
- Backend: Bulk CSV import siswa (parser, validator) + **generate password per siswa + credentials.csv di-upload ke R2 `import/<uuid>-credentials.csv` + presigned download sekali (TTL 15 menit) + auto-cleanup `s3.DeleteObject` 1 jam setelah CompletedAt**
- Backend: **R2 object-key convention** — `<kategori>/<uuid>.<ext>` di kolom `ObjectKey`, OriginalFilename + MimeType + SizeBytes di DB column terpisah
- Frontend admin: import CSV (drag-and-drop, preview tabel persistent — admin bisa close tab + balik tanpa lose state, confirm, modal sukses dengan link download credentials.csv), list kelas (read-only)
- Frontend guru: dashboard list+create kelas + tombol Duplicate, kelas detail (tab Siswa, tab Pengaturan/bobot, tab Pengumuman placeholder), edit form pakai version (409 handler "konten ke-update orang lain")
- Frontend siswa: dashboard, gabung kelas via kode

### Fase 3 — Bab & Materi + Pengumuman + Bab Status (3-4 hari)

> **Locked decisions Fase 3 (v0.8.1):** #63 Materi 3-tipe (pdf/youtube/markdown — drop video upload) | #64 PDF max 20MB | #65 YouTube strict video-id parse + nocookie embed | #66 Pengumuman passive timestamp (no dismiss state) | #67 Bab reorder bulk urutan | #68 Bab progress Fase-3-partial = materi-only re-normalize | #69 Materi hard-delete + compensating R2 cleanup.

- Backend: Bab CRUD (guru) + reorder bulk endpoint (#67) + **status (draft/published/archived)** + **Version field** (optimistic concurrency) + duplicate (copy materi/pengumuman; soal/tugas masuk Fase 4-5)
- Backend: Materi CRUD dengan field `bab_id` nullable, **3 tipe `pdf|youtube|markdown` (locked #63)** — `pdf` upload ke R2 `materi/<uuid>.pdf` (max 20MB, mime `application/pdf`, locked #64), `youtube` simpan video_id 11-char hasil `parseYouTubeID` (locked #65), `markdown` body inline di DB. PDF akses lewat presigned GET URL endpoint scoped (`GET /materi/:id/file-url`). Hard delete + R2 cleanup (locked #69).
- Backend: MateriRead endpoint (siswa mark-as-read, idempotent)
- Backend: endpoint siswa list bab (cuma published) + detail bab dengan progress Fase-3-partial = materi-only re-normalize (locked #68 + formula 6.4)
- Backend: Pengumuman CRUD (per-kelas atau per-bab, sort `created_at DESC`, no dismiss state — locked #66)
- Frontend guru:
  - Tab "Bab" di kelas detail: list bab dengan status badge, drag-and-drop urutan via `@dnd-kit/core` (locked #67), create/edit/delete/archive/publish/unpublish/duplicate, edit form pakai version (409 → "konten ke-update orang lain, refresh dulu")
  - `/guru/kelas/detail/bab?id=&bid=` shell dengan tabs (Materi / Soal placeholder / Tugas placeholder / Pengumuman / Pengaturan) + status badge di header (static-export friendly query-param routing, mirip /guru/kelas/detail)
  - Tab Materi di bab: create dialog dengan radio jenis (PDF upload / YouTube link / Markdown editor), list + edit/delete
  - Tab Pengumuman per kelas + per bab: compose markdown, edit, delete; badge "Baru" kalau < 7 hari (locked #66)
- Frontend siswa:
  - `/siswa/kelas/detail?id=` list bab published (urut, judul, deskripsi, **progress bar dengan tooltip "Berdasarkan materi dibaca (N/M)"** — Fase-3-partial locked #68) + section pengumuman (read-only, sort newest first)
  - `/siswa/kelas/detail/bab?id=&bid=` detail bab dengan tab Materi (viewer + auto mark-read on open) + section pengumuman bab
  - Materi viewer: PDF iframe via presigned URL TTL 15m, YouTube embed `youtube-nocookie.com/embed/<id>` (locked #65), react-markdown

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
  - Orphan R2 objects per prefix (`soal/`, `materi/`, `tugas/`, `submission/`, `import/`) — cross-check ke kolom `*ObjectKey` di tabel terkait, `s3.DeleteObject` per orphan, log count
  - ImportJob credentials.csv expired (>1 jam after CompletedAt) — `s3.DeleteObject` ke CredentialsObjectKey
  - LoginAttempt >30 hari
  - RefreshToken expired & revoked >7 hari
  - HasilSoalBab/HasilUjian deleted_at >1 tahun → hard delete + audit log
  - Submission file: kelas archived + 1 tahun → `s3.DeleteObject` ke AttachmentObjectKey (DB row tetap untuk nilai history)
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
- Image storage growth: gambar soal numpuk di R2; cleanup task (Fase 8) untuk hapus orphan objects yang gak ke-reference (`s3.ListObjectsV2` per prefix `soal/` + cross-check ke kolom `GambarSoalObjectKey` / `GambarOpsiA..EObjectKey`)
- R2 reachability: kalau Cloudflare R2 down/credentials expired, semua upload + presigned URL gagal. Mitigasi: `/readyz` cek `HeadBucket` + alert; queue upload retry dengan exponential backoff (Fase 8). Tidak ada fallback local disk.
- Presigned URL leak: URL valid sampai TTL habis (default 15 menit). Mitigasi: TTL singkat + audit log `file_url_issued` untuk file sensitif (submission, credentials.csv) + jangan log URL ke stdout/file.
- R2 cost: outbound bandwidth gratis (Cloudflare zero egress fee), tapi storage + Class A operations (PutObject/CopyObject/DeleteObject) berbayar di atas free tier (10GB storage + 1M Class A ops/bulan). Monitor di Cloudflare dashboard; resize gambar pre-upload + cleanup orphan = control utama.
- **Password lifecycle**: password awal cuma muncul SEKALI di modal — kalau admin lupa salin, satu-satunya jalan reset ulang. Kasih copy button + confirmation sebelum tutup modal.
- **CSV credentials file leak**: object di R2 valid untuk download lewat presigned URL TTL singkat (15 menit) + auto-cleanup `s3.DeleteObject` 1 jam setelah CompletedAt. Bucket non-public, jadi gak ada cara akses tanpa presigned URL. Audit `file_url_issued` setiap kali presign di-issue.
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

## 12. Open Decisions Tersisa (v0.8.0)

1. **Notifikasi**: bentuk apa, kapan trigger, polling/SSE/websocket — bedah di v0.8 setelah Fase 0-3 jalan.
2. ~~**Pengumuman dismiss state per siswa**: sekedar "udah dilihat" atau ada read receipt? — diputuskan saat Fase 3 implementasi.~~ **RESOLVED v0.8.1 → locked #66** (passive timestamp display, no dismiss state, no read receipt table; badge "Baru" kalau < 7 hari).
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
- Storage: **Cloudflare R2** (lihat decision #61) — bucket `lms-prod`, kredensial di `.env` (`R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET`); tidak ada folder lokal di server
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
5. **`.env`**: `cp .env.example .env`, isi: `DATABASE_URL`, `JWT_SECRET_KEY`, `PORT=8200`, `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET=lms-prod` (atau `lms-dev` untuk workspace), opsional `R2_ENDPOINT` + `R2_PRESIGN_TTL_SECONDS=900`, `ENV=production`
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

### Changelog v0.7.2 → v0.8.0
- **Storage strategy migrated**: local disk (`./storage/uploads/`) → **Cloudflare R2** (S3-compatible). Berlaku untuk semua kategori file: tugas, soal (gambar), materi, submission, import CSV.
- **Locked decisions revised**:
  - #6 Storage materi → Cloudflare R2 (bukan local disk)
  - #44 Health/readiness split → readyz cek `HeadBucket` ke R2, bukan storage dir writable
  - #46 File upload hardening → resize + mime sniff dilakukan SEBELUM `s3.PutObject`, R2 bucket non-public, akses lewat presigned GET URL
  - #54 CSV import preview persistence → CSV disimpan di R2, cancel = `s3.DeleteObject`
  - #58 Storage path convention → R2 object key `<kategori>/<uuid>.<ext>`, single bucket per env (`lms-prod` / `lms-dev`), kolom DB rename `FilePath` → `ObjectKey`
- **Locked decisions baru**:
  - #61 Storage backend — Cloudflare R2: aws-sdk-go-v2 + path-style + endpoint resolver, env vars, wrapper interface `Storage` di `internal/storage`
  - #62 Upload flow & access control: client → backend multipart → R2 (no direct browser→R2 di MVP), download lewat presigned GET URL TTL 15 menit + audit `file_url_issued`
- **Section 3 Tech Stack**: tambah `aws-sdk-go-v2` sebagai object storage client; `File upload` line di-update.
- **Section 4 Storage & Infra**: bullet `./storage/uploads/...` diganti dengan R2 detail (bucket, key format, presigned URL, no direct browser upload).
- **Section 6 Data Model**: kolom `FilePath`/`AttachmentPath`/`GambarSoal`/`GambarOpsiA..E` ganti jadi `ObjectKey` + `OriginalFilename` + `MimeType` + `SizeBytes` (atau `*ObjectKey` + `*OriginalFilename` untuk gambar soal/opsi). SoalBab + Soal updated.
- **Section 7 API**: `/readyz` deskripsi cek R2; ImportJob cancel pakai `s3.DeleteObject`.
- **Section 9 Project Structure**: `storage/uploads/` directory dihapus dari tree; `internal/storage/` di-anotate sebagai R2 wrapper.
- **Section 10 Phasing**: Fase 2 nambah Task 2.D.0 (R2 wrapper bootstrap) sebelum Task 2.D.1; ImportJob lifecycle di-update untuk R2; Fase 3 materi storage diarahkan ke R2; Fase 8 cleanup tasks pakai `s3.DeleteObject` per orphan dan submission expiry.
- **Section 11 Risks**: 4 risiko baru — orphan R2 objects, R2 reachability, presigned URL leak, R2 cost. Risiko CSV credentials leak di-update untuk pola R2 + presigned URL.
- **Section 13 Deploy**: storage path lokal diganti dengan referensi R2 + env vars di `.env.example`.
- **Section 18 Task-by-Task**: `Task 2.C.4` di-detail (read-only, list-enrollments endpoint baru); `Task 2.D.0` task baru (R2 wrapper) sebelum 2.D.1; `Task 2.D.2-2.D.5` rewrite untuk R2 (PutObject preview CSV, presigned credentials download); Current Next Step section di-tulis ulang dengan pre-requisite eksternal Cloudflare R2 (bucket + token + env vars).

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

## 18. Task-by-Task Implementation Plan (Fase 0-3)

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

**Task 2.C.3 — FE siswa dashboard + join kelas form** ✅ DONE 2026-05-20
- `frontend/app/siswa/page.tsx` (list kelas siswa) + `frontend/app/siswa/gabung/page.tsx`
- Visual + e2e
- Commit: `feat(fe-siswa): dashboard + join kelas`
- Shipped (backend prep): `GET /api/v1/siswa/kelas` di file baru `backend/internal/kelas/my_kelas_handler.go` + service method `Service.ListMyKelas` + tipe `MyKelasItem/MyKelasResult`. Filter status=active (removed enrollment hidden), pagination ?page=&page_size= (max 100), tolerant terhadap dangling enrollment (skip kalau kelas hilang). `kelasRepo` interface ditambah `ListEnrollmentsBySiswa`; mockRepo + stubSvc updated. Tests: handler happy path 200 + pagination defaults/cap (commit `952fe01`).
- Shipped (FE): `frontend/app/(authed)/siswa/layout.tsx` shell (RoleGuard siswa, sidebar Dashboard + Gabung Kelas, header user menu mirror guru), `app/(authed)/siswa/page.tsx` dashboard (list MyKelasItem, empty-state CTA, joined_via badge), `app/(authed)/siswa/gabung/page.tsx` form react-hook-form+zod dgn auto-uppercase + error mapping 6 code (`kode_invite_required/not_found/kelas_archived/enrollment_removed/forbidden/too_many_requests`) ke pesan UX ramah, `lib/siswa-api.ts` typed client (commit `2a4b9c9`).
- Verified server: typecheck PASS, npm build PASS (20 pages, /siswa 4.36 kB + /siswa/gabung 3.36 kB); E2E smoke 10 scenario hijau (siswa tok ok, baseline GET /siswa/kelas total=2, guru bikin kelas baru, siswa join via kode 201, GET total naik 2→3 dgn kelas baru kelihatan, archive kelas → tetap kelihatan selama enrollment active, soft-remove enrollment → hidden, role-guard guru→403, no-auth→401, FE static /siswa.html + /siswa/gabung.html serve 200).

**Task 2.C.4 — FE guru tab Siswa di kelas detail** ✅ DONE 2026-05-20
- Backend prep (perlu dibuat dulu): `GET /api/v1/kelas/:id/enrollments?page=&page_size=` — list enrollment kelas, hydrate dgn user (nama, email, joined_via, joined_at, status). Service method baru `Service.ListEnrollmentsByKelas`. Filter active-only by default (`?status=all` untuk admin lihat removed). Auth: guru-owner OR admin (lihat #59 + canManage).
- FE: `frontend/app/(authed)/guru/kelas/detail/page.tsx` swap PlaceholderTab "Siswa" jadi table real. Kolom: Nama, Email, Bergabung via, Tanggal join. Pagination + empty state + role-aware (admin nanti dapet kolom Aksi remove di task terpisah).
- Locked decision: guru read-only di MVP — tombol remove **tidak ada** (admin scope, dibahas di Fase 2 backlog atau v0.9). Catat di komentar code biar reviewer berikutnya tau.
- Commit: `feat(kelas+fe-guru): list-enrollments endpoint + tab Siswa di kelas detail (Task 2.C.4)` (commit `cc5f57c`) + vet fix `d79cfd3`.
- Shipped (backend): `Service.ListEnrollmentsByKelas(kelasID, callerID, role, in)` di `service.go` + `Handler.ListEnrollments` di file baru `backend/internal/kelas/enrollments_handler.go` + route `GET /api/v1/kelas/:id/enrollments` di `cmd/server/main.go`. NewService skrng butuh 3 args: repo + audit + users (`userLookup` interface implement *auth.Repo). Default filter status=active; admin opt-in `?include_removed=true`. Total dari repo dipertahankan apa adanya supaya page math konsisten saat filter berubah. Tolerate dangling enrollment (siswa user yg udah dihapus) — di-skip silently mirip ListMyKelas. Hard precondition (4xx): bad uuid → 400 invalid_id, kelas not found → 404 not_found, foreign guru → 403 forbidden.
- Shipped (FE): `lib/kelas-api.ts` tambah `listKelasEnrollments` + types `EnrollmentItem/EnrollmentListResponse/EnrollmentStatus/EnrollmentJoinedVia`. `app/(authed)/guru/kelas/detail/page.tsx` `PlaceholderTab "Siswa"` diganti jadi `SiswaTab` — table real (Nama, Email, Bergabung via, Tanggal), pagination 20/page, refresh button, empty state, error mapping (forbidden + request_id). Read-only di MVP: tombol remove **tidak ada**, ada catatan inline + footer `Read-only di MVP. Untuk mengeluarkan siswa, hubungi admin.`
- Tests: 4 service-level (happy hydrate, hides_removed default + admin include opt-in, forbidden non-owner + admin allowed, not_found, dangling user skipped) + 4 handler-level (happy w/ pagination, include_removed query propagate, invalid_id 400, forbidden 403). Plus mockRepo dapet `RemoveEnrollment` test helper (vet fix d79cfd3) + `ListEnrollmentsByKelas` mock.
- Verified server: `go build ./... && go vet ./... && go test ./...` semua hijau (admin/auth/kelas/middleware ok); `npx tsc --noEmit` PASS, `npm run build` PASS (20 pages, /guru/kelas/detail naik 4.13kB → 7.1kB).
- Live smoke E2E: **deferred** — user gak izinkan systemctl restart lms-api di sesi ini (service aktif). Build/vet/test passing dianggap cukup sebagai evidence struktural. Routing wiring (`kelasGroup.Get("/:id/enrollments", ...)`) bisa di-curl saat restart berikutnya. Commit `cc5f57c` siap di-deploy kapan pun.

#### 2.D Bulk Import CSV

**Task 2.D.0 — R2 storage client wrapper + bucket bootstrap (prerequisite untuk semua upload)**

Pecah jadi dua sub-step supaya gak idle nungguin credentials user.

**2.D.0.a — Storage interface + MockStorage skeleton ✅ DONE 2026-05-20 (commit `1887aef`)**
- Files shipped: `backend/internal/storage/storage.go` (interface + BuildKey + legacy compat), `backend/internal/storage/mock.go` (in-memory MockStorage, thread-safe), `backend/internal/storage/factory.go` (R2Config + NewStorage factory + ErrR2NotImplemented), `backend/internal/storage/storage_test.go` (BuildKey happy/error, IsValidCategory, MockStorage round-trip + missing + idempotent delete + PresignGet + invalid put + concurrent + R2Config IsConfigured + factory fallback/fail/not-implemented)
- Config: `config.StorageConfig.R2` extended (AccountID/AccessKeyID/SecretAccessKey/Bucket/PresignTTLSec) + loaded dari `R2_*` env vars dengan default PresignTTL 900s
- `cmd/server/main.go` boot wire: `storage.NewStorage(cfg.R2, FactoryOptions{AllowMockFallback: true})` → log `r2_configured` + backend type. Saat 2.D.0.b belum landing, fallback ke MockStorage (warn-level log).
- Verified server: build OK, vet OK, `go test ./...` ALL_TEST_OK (admin/auth/kelas/middleware/storage)
- Live smoke deferred — gak butuh restart, behavior pre-eksisting tetap

**2.D.0.b — Real R2 client (aws-sdk-go-v2) ✅ DONE 2026-05-20 (commits `ecd26a9` + `0b36e9f` + `2b8ab41`)**
- Files shipped: `backend/internal/storage/r2.go` (R2Client implement Storage via aws-sdk-go-v2: Put/Get/Delete/Exists/PresignGet/HeadBucket; endpoint `https://<account>.r2.cloudflarestorage.com`, region "auto", path-style); `backend/internal/storage/r2_test.go` (integration test gated `R2_INTEGRATION=1` + bad-creds rejection test); `backend/internal/health/health.go` updated (R2 HeadBucket cache 30s, 2-failure threshold, 5s probe timeout); `backend/cmd/server/main.go` (Storage wired into Handler, AllowMockFallback gated to non-prod, **boot prewarm** 30s budget non-fatal)
- go.mod / go.sum: `github.com/aws/aws-sdk-go-v2` v1.41.7 + service/s3 v1.101.0 + smithy-go v1.25.1; toolchain bumped 1.22 → 1.24
- Verified server: `go test ./...` ALL_TEST_OK; `R2_INTEGRATION=1 go test ./internal/storage/... -run TestR2Client` PASS (4.18s roundtrip + 5.78s bad-creds); restart `lms-api` boot log `storage ready r2_configured=true backend=*storage.R2Client` + `r2 prewarm ok bucket=lms-dev elapsed=1.158s`; readyz `status=ready db=ok storage=ok (r2:lms-dev, cached)` 45-60ms
- Live notes: server pertama hit IPv6 ke Cloudflare R2 broken (`2606:4700:2ff9::1` no route), happy-eyeballs fallback IPv4 ~5-13s. Boot prewarm sebelum app.Listen primes cache → ExecStartPost curl /readyz langsung dapet cached-OK, gak timeout.
- Commit: `ecd26a9` feat(storage): real Cloudflare R2 client + readyz integration; `0b36e9f` build(go): add aws-sdk-go-v2 deps; `2b8ab41` perf(server): pre-warm R2 HeadBucket at boot

**Task 2.D.1 — CSV parser + validator ✅ DONE 2026-05-20 (commits `a5adf68` + `1323f47`)**
- Files shipped: `backend/internal/importjob/parser.go` (Parse + ParseResult + Row + sentinel errors), `backend/internal/importjob/parser_test.go` (18 cases)
- Header alias detection (`nama|name|nama_lengkap|full_name|fullname`, `email|e-mail|alamat_email`, `kode_kelas|kode|kode_invite|invite_code`)
- Delimiter auto-detect (`,` atau `;` — common Excel locale Indonesia), UTF-8 BOM tolerated, fail-fast pada non-UTF-8 (Excel users: re-save as "CSV UTF-8")
- Per-row validate: nama 1-100 chars, email RFC `net/mail.ParseAddress` (max 254 RFC 5321), kode max 32. Email lowercased + trimmed, kode uppercased + trimmed (DB pakai citext)
- Dedup by lowercased email — first occurrence wins (Valid), berikutnya `RowDuplicate` dengan reference ke baris pertama. Invalid rows TIDAK claim email
- Hard limits: `MaxCSVBytes=5MB`, `MaxCSVRows=5000` (sentinel `ErrCSVTooLarge`/`ErrTooManyRows`)
- LineNo termasuk header (data row pertama LineNo=2) untuk UI error message
- Output: `ParseResult{Rows, Stats{Total/Valid/Invalid/Duplicates}}` — Rows serialize 1:1 ke `PreviewRowsJSON` di Task 2.D.2
- Verified server: `go test ./internal/importjob/... -count=1 -v` PASS (18 tests), full `go test ./...` ALL_TEST_OK
- Live deploy: TIDAK perlu (pure parser, gak wired ke route — wiring di Task 2.D.2)
- Commit: `a5adf68` feat(importjob): CSV parser + validator; `1323f47` fix: rowsEqual helper for Row{} (vet fix)

**Task 2.D.2 — R2 upload + preview CSV ✅ DONE 2026-05-20 (commits `b01159d` + `aa9d9b8` + `8abd406`)**
- Migration 000004: `ALTER TABLE import_jobs ADD COLUMN object_key TEXT` (applied di workspace DB)
- Files shipped: `backend/internal/importjob/service.go` (Service.PreviewUpload: mime sniff → Parse → R2 PutObject `import/<job_uuid>.csv` → DB Create dengan compensating R2 delete kalo DB gagal); `backend/internal/importjob/handler.go` (POST /admin/import-csv/upload multipart, sentinel→HTTP code mapping, audit log `import_csv_uploaded`); `backend/internal/importjob/service_test.go` (7 cases); `backend/internal/importjob/handler_test.go` (13 cases dgn 9 sentinel mapping subtests)
- Model: `ImportJob.ObjectKey *string` (column `object_key`)
- Wired di `cmd/server/main.go`: `importjob.NewService(repo, objectStore, 0)` → `Handler` di `adminGroup.Post("/import-csv/upload", ...)`
- Limits: `MaxCSVBytes=5MB` (handler enforced via LimitReader), preview row cap 200 default (parser stats reflect full count)
- Compensating delete: kalo R2 PutObject sukses tapi DB Create gagal, `DeleteObject(objectKey)` best-effort (warn-log kalau juga gagal)
- Verified server: `go test ./...` ALL_TEST_OK; live restart applied
- Live smoke E2E (5/5 PASS):
  - happy: 201 + ImportJob row + R2 object + audit log entry
  - missing nama column: 400 `missing_nama_column`
  - missing file: 400 `missing_file`
  - binary disguised csv: 415 `unsupported_mime`
  - no auth: 401 `unauthorized`
- R2 verify: HeadObject 142 bytes content-type `text/csv; charset=utf-8`, body content match raw upload
- Audit verify: row dengan action `import_csv_uploaded` target_type `import_job` meta `{filename, object_key, total_rows, valid_count, invalid_count}`
- Cleanup: smoke test data dihapus dari R2 + DB
- Commits: `b01159d` feat(importjob): R2 upload + preview ImportJob endpoint; `aa9d9b8` test(importjob): service + handler tests; `8abd406` test fix: BodyLimit di test app

**Task 2.D.3 — Resume + cancel preview** ✅ DONE 2026-05-20
- Routes wired di `cmd/server/main.go`:
  - `GET /api/v1/admin/import-csv/:job_id` → resume preview (scope: admin owner only). Status: 200 (preview), 404 not_found, 409 not_in_preview, 410 preview_expired
  - `POST /api/v1/admin/import-csv/:job_id/cancel` → flip preview→cancelled + best-effort R2 DeleteObject. Status: 200, 404, 409 (idempotent guard)
- Service: `Service.GetPreview` (decode PreviewRowsJSON + scope by adminID + TTL check tanpa mutate state) + `Service.Cancel` (status flip dulu, R2 delete sesudahnya supaya orphan diserap cron, gak preview→hilang-object race)
- Sentinels baru: `ErrJobNotFound` (404), `ErrJobExpired` (410), `ErrJobNotInPreview` (409)
- Audit action baru: `import_csv_cancelled` meta `{filename, object_key}`
- New status enum value: `StatusCancelled` ("cancelled") — distinct dari `StatusExpired` (cron-driven) supaya audit trail bisa beda admin-cancel vs auto-expire
- Tests: 9 service + 6 handler (total importjob package: 16 service + 19 handler + 18 parser = 53 cases)
- Live smoke 7/7 PASS:
  - GET resume happy: 200 dengan preview rows + status=preview + filename
  - GET invalid uuid: 400 `invalid_job_id`
  - GET random uuid: 404 `not_found`
  - GET no auth: 401 `unauthorized`
  - POST cancel happy: 200 status=cancelled
  - POST cancel idempotent: 409 `not_in_preview`
  - GET after cancel: 409 `not_in_preview`
- R2 verify: HeadObject post-cancel returns 404 NoSuchKey (object dihapus)
- Audit verify: 2 rows for same job_id — `import_csv_uploaded` + `import_csv_cancelled`
- Cleanup: smoke data dihapus dari R2 + DB
- Operational fix: ExecStartPost retry budget bumped 10→30 detik supaya R2 prewarm 10.5s slot gak failed-to-start (IPv6 happy-eyeballs sudah dimitigate via boot-prewarm tapi prewarm itu sendiri berdurasi >10s)
- Commit: `601a4c8` feat(importjob): GET resume + POST cancel preview endpoints (Task 2.D.3)

**Task 2.D.4 — Confirm import (preview → processing → completed)** ✅ DONE 2026-05-20
- Route wired: `POST /api/v1/admin/import-csv/:job_id/confirm` (admin-scoped, `FindByIDForAdmin`)
- Lifecycle: preview → processing (lock at status flip with ConfirmedAt=now) → completed (CompletedAt=now). Always 200 with partial success surfaced via `errors_json` + `failures` array; never all-or-nothing 4xx
- Service deps wired in main.go via setters: `importSvc.SetUserCreator(authRepo)` + `importSvc.SetKelasRepo(kelasRepo)` (avoids constructor ballooning + circular import)
- Source-of-truth re-parse: re-fetch raw CSV from R2 ObjectKey + `Parse` again. PreviewRowsJSON is capped 200 rows so cannot drive batch creation
- Per-row flow: pre-check duplicate via `FindUserByEmail` → `GeneratePassword()` (12 char alfanumerik, `crypto/rand` w/ rejection sampling for uniform distribution, ~71 bits entropy) → `auth.HashPassword` (bcrypt) → `CreateUser` (role=siswa, status=active, must_change_password=true, created_by_id=admin) → optional `FindByKodeInvite` + `Enroll(JoinedViaAdmin)`. Each step's failure recorded in `ConfirmFailure` but never aborts overall call
- Stable reason codes: `invalid_row`, `duplicate_in_db`, `user_create_error`, `hash_error`, `kelas_not_found`, `enroll_error` — FE maps to UI copy
- Decision: kelas_not_found does NOT roll back user creation (user dibuat, admin enroll manual nanti). Konsisten dgn partial-success pattern
- Credentials.csv: `csv` package render `email,password,kode_kelas,nama_kelas` → R2 PutObject `credentials/<job_uuid>.csv` (CategoryCredentials baru di storage whitelist) → SetCredentialsPath
- Failure modes:
  - R2 GetObject fail → flip job=failed, return `confirm_failed` 500
  - Re-parse fail (corruption) → flip job=failed, `confirm_failed`
  - `pr.Stats.Total < job.TotalRows` → ErrConfirmRowsMismatch → 409 `rows_mismatch` (admin re-upload)
  - R2 PutObject credentials fail → users sudah created tapi gak bisa rollback → flip failed, errors_json populated, `confirm_failed`
- New sentinels: `ErrConfirmRowsMismatch` (409 rows_mismatch), `ErrInternalConfirm` (500 confirm_failed)
- New audit action: `import_csv_confirmed` meta `{filename, object_key, credentials_object_key, success_count, fail_count}`
- New StatusCompleted/StatusFailed flow distinct dari StatusCancelled (admin cancel) + StatusExpired (cron auto-expire)
- Tests: 8 service + 3 handler. Total importjob package: 16+8 svc + 19+3 hdl + 18 parser = 64 cases
- Live smoke 6/6 PASS:
  - Step 1 upload: 4 rows (3 valid + 1 invalid email) → 201 preview
  - Step 2 confirm happy: 200 status=completed success_count=3 fail_count=2 (1 invalid_row + 1 kelas_not_found) credentials_object_key populated
  - Step 3 idempotent: 409 not_in_preview
  - Step 4 invalid uuid: 400 invalid_job_id
  - Step 5 random uuid: 404 not_found
  - Step 6 no auth: 401
- DB verify: 3 siswa rows created (must_change_password=true), 1 enrollment row to "Matematika 7A Smoke" via JoinedViaAdmin
- R2 verify: HEAD credentials/...csv 215 bytes text/csv; 3 password rows 12 chars each; nama_kelas filled when match, empty when kelas_not_found
- E2E proof: login Apis pake password generated → 200 + access_token + must_change_password=true (bcrypt round-trip works)
- Cleanup: 3 users + 1 enrollment + 2 audit + 1 import_job + 2 R2 objects deleted
- Commit: `da0fe4c` test(importjob): add Confirm tests; preceded by `d5234b1` wip + `563df99` test stub extension

**Task 2.D.5 — Download credentials.csv (presigned, TTL-bound)** ✅ DONE 2026-05-20
- Route wired: `GET /api/v1/admin/import-csv/:job_id/credentials.csv` (admin-scoped, FindByIDForAdmin)
- Lifecycle guards (in order): not_found(404) → not_completed(409) → credentials_expired(410, CompletedAt+1h) → credentials_missing(404, when CredentialsCSV NULL or R2 lost object). Single 500 path = download_failed (R2 presign failure)
- Storage interface extended: `PresignGetDownload(ctx, key, ttl, filename)` — sets `ResponseContentDisposition: attachment; filename="…"; filename*=UTF-8''…` (RFC 5987 + ASCII fallback) so browser saves file with stable name instead of UUID-based key
- Handler: 302 redirect to presigned URL + audit `file_url_issued` meta `{object_key, filename, ttl_sec}`
- New sentinels: `ErrJobNotCompleted` (409 not_completed), `ErrCredentialsExpired` (410 credentials_expired), `ErrCredentialsMissing` (404 credentials_missing), `ErrInternalDownload` (500 download_failed)
- New const: `CredentialsTTL = 1 * time.Hour` (matches roadmap spec; cleanup cron at 2.D.6 deletes after this window)
- Service.SetPresignTTL injects `cfg.Storage.R2.PresignTTLSec` (default 900s = 15m, clamped [60s, 24h] in R2 client)
- MockStorage + stubStore mirror new method (filename echoed via query param so tests can assert without parsing real Content-Disposition)
- Tests: 6 service + 3 handler. Total importjob package: 22+6 svc + 22+3 hdl + 18 parser = 71 cases
- Live smoke 6/6 PASS:
  - 302 + Location ke `https://<acct>.r2.cloudflarestorage.com/lms-dev/credentials/<uuid>.csv?X-Amz-Algorithm=...&X-Amz-Expires=900&response-content-disposition=attachment%3B...&X-Amz-Signature=...`
  - Presigned URL → 200 OK, `Content-Type: text/csv; charset=utf-8`, `Content-Disposition: attachment; filename="credentials-<uuid>.csv"; filename*=UTF-8''…`, body 4 lines (header + 3 creds, password 12 char alfanumerik)
  - Invalid uuid → 400 invalid_job_id; random uuid → 404 not_found; no auth → 401
- Audit verify: 2x `file_url_issued` rows w/ object_key=credentials/<uuid>.csv, filename=credentials-<uuid>.csv, ttl_sec=900
- Cleanup: 3 users + 1 enrollment + 4 audit + 1 import_job + 2 R2 objects deleted
- Commits: `1b46030` feat(import): credentials presigned download with TTL (Task 2.D.5); `0bcc092` test(importjob): register credentials.csv route in test app

**Task 2.D.6 — Hourly cleanup cron** ✅ DONE 2026-05-20 (commits `a9dbbc3` + `2dd9edb`)
- Files: `backend/internal/importjob/cleanup.go` (196 LOC) + `cleanup_test.go` (293 LOC) + extend `repo.go` (+45 LOC `ExpireCredentialsBefore`) + `cmd/server/main.go` (wire `Cleaner.Run` goroutine bound ke `rootCtx`)
- Two sweeps per tick (cadence `CleanupInterval = 1h`, initial sweep on boot):
  1. Preview expiry: `Repo.ExpirePreviewBefore(now)` (existing) — flips preview→expired tx-locked + best-effort `s3.DeleteObject(ObjectKey)` per row
  2. Credentials eviction: new `Repo.ExpireCredentialsBefore(now - CredentialsTTL)` — query completed+credentials_csv IS NOT NULL+completed_at < cutoff, tx-locked null-out credentials_csv (status stays `completed`), best-effort delete `credentials/<uuid>.csv` from R2
- Per-row R2 errors → `slog.Warn` + counted (`PreviewObjectsErr`, `CredentialsObjectsErr`); never abort the loop. Repo errors from one sweep do NOT block the other (errors.Join).
- Concurrency: `select { ctx.Done() | t.C }`; DeleteObject uses `context.Background()` so ctx cancel mid-tick doesn't half-orphan.
- Tests (5 new): `RunOnce_PreviewHappy`, `RunOnce_PreviewNoRows`, `RunOnce_CredentialsHappy`, `RunOnce_CredentialsDeleteError`, `RunOnce_RepoError`, `Run_ContextCancel`. All ALL_TEST_OK (`go test ./internal/importjob/...` 0.409s).
- Live smoke PASS:
  - Created 1 preview job + 1 completed job via real /admin/import-csv flow
  - `UPDATE expires_at = NOW() - 2h` + `UPDATE completed_at = NOW() - 2h`
  - `systemctl restart lms-api` → initial sweep fired:
    - log: `importjob cleanup: preview swept expired=1 r2_deleted=1 r2_errors=0`
    - log: `importjob cleanup: credentials swept evicted=1 r2_deleted=1 r2_errors=0`
  - DB verify: preview row → status=`expired`; completed row → `credentials_csv = NULL` (status stays `completed`)
  - R2 verify (boto3 head_object): `import/<preview>.csv` GONE ✓; `credentials/<completed>.csv` GONE ✓; `import/<completed>.csv` EXISTS (preserved as forensic — only credentials evicted per spec)
  - Endpoint sanity: `GET /admin/import-csv/<completed>/credentials.csv` → 410 `credentials_expired` (TTL guard fires before missing-pointer guard since cutoff matches)
- Cleanup: 1 user + 0 enroll + 3 audit + 2 import_jobs + 1 R2 object deleted; users_left=0, jobs_left=0
- **Fase 2.D = 6/6 DONE; Fase 2 progress = 18/20** (sisa 2 task = Fase 2.E FE Admin Import, out-of-scope BE roadmap)

#### 2.E FE Admin Import — ✅ ALL DONE 2026-05-21 (commit `0f3772e`)

**Task 2.E.1 — /admin/import-csv page (drag-and-drop upload)** ✅
- File baru: `frontend/lib/import-api.ts` (232 LOC) — types + uploadImportCSV (multipart, hand-rolled fetch karena api() force JSON), getImportPreview, cancelImport, confirmImport, downloadCredentialsCSV (manual redirect handling)
- File baru: `frontend/app/(authed)/admin/import-csv/page.tsx` (614 LOC) — single state machine driven by ?job_id query string (Next 14 static export pattern, mirror /admin/pengguna/detail)
- UploadCard: drag-and-drop + file picker, client-side validation (.csv, max 5MB, non-zero), `onDragOver`/`onDrop` handlers, contoh CSV format collapsible
- Sidebar: tambah entry `Import CSV` antara Pengguna + Audit Log dengan FileSpreadsheet icon

**Task 2.E.2 — Preview tabel persistent (admin bisa close + balik)** ✅
- `useQuery(['admin','import-csv',jobID])` enabled saat ?job_id present, retry=false, staleTime=5s
- Auto-drop ?job_id saat 410 expired / 409 not_in_preview / 404 not_found via toast + router.replace
- PreviewCard: header dengan filename + valid/invalid/total counters + ExpiresAt; table dengan row status pill (valid/invalid/duplicate); error notes column; "trimmed rows" hint kalau preview_rows < total_rows; "0 valid → upload ulang" warning
- Cancel button → cancelImport → toast + back to upload card; Confirm button gated (status=preview && valid_count>0)

**Task 2.E.3 — Confirm + modal sukses + download credentials.csv** ✅
- Confirm mutation → SuccessDialog (shadcn Dialog) shows X akun berhasil + Y gagal + per-row failure table dengan confirmReasonLabel mapping (invalid_row/duplicate_in_db/user_create_error/hash_error/kelas_not_found/enroll_error)
- Download button → downloadCredentialsCSV (fetch dengan bearer header, redirect:'manual', baca Location header dari 302) → window.open(URL,'_blank') agar attachment Content-Disposition trigger save-as
- Close dialog → router.replace('/admin/import-csv') untuk start fresh

**Build verify (server)**: `npm run build` ALL OK; 21 pages (was 20); `/admin/import-csv = 12.4 kB / 130 kB First Load JS`; `lib/role-guard.tsx` warning pre-existing (unrelated, useMemo suggestion).

**Live smoke 5/5 PASS via curl:**
1. Upload 3-row CSV (2 valid + 1 invalid email) → 201 valid=2 invalid=1 total=3
2. GET resume → status=preview filename=smoke-2e.csv
3. POST confirm → 200 status=completed success=2 fail=1 credentials=credentials/<uuid>.csv
4. GET credentials.csv → 302 Location=`https://<acct>.r2.cloudflarestorage.com/lms-dev/credentials/<uuid>.csv?X-Amz-...`
5. Fetch presigned → 200 OK, `Content-Disposition: attachment; filename="credentials-<uuid>.csv"; filename*=UTF-8''…`, body 3 lines (header + 2 creds dengan password 12 char)
- Cleanup: 2 users + 0 enroll + 4 audit + 1 import_job + 2 R2 objects deleted

**Fase 2 = 20/20 DONE 100%.** Backend (Kelas + Enrollment + Bulk Import via R2) + Frontend (admin shell + import-csv page) full-stack complete. Pivot ke Fase 3 (Bab & Materi + Pengumuman).

#### 2.F E2E Manual Fase 2

**Task 2.F.1 — Bikin kelas + invite kode + siswa join**
- Manual: guru login → bikin kelas → copy kode → siswa login → join → muncul di dashboard
- Commit: `docs: fase 2 e2e flow guru-siswa passed`

**Task 2.F.2 — Bulk import 5 siswa**
- Manual: bikin sample.csv → upload → preview → confirm → download credentials → 5 user baru bisa login
- Commit: `docs: fase 2 e2e bulk import passed`

---

### Fase 3 — Bab & Materi + Pengumuman + Bab Status (3-4 hari)

> Locked decisions: #63 Materi 3-tipe (pdf/youtube/markdown) | #64 PDF max 20MB | #65 YouTube strict video-id parse | #66 Pengumuman passive timestamp | #67 Bab reorder bulk urutan | #68 Bab progress Fase-3-partial = materi-only re-normalize | #69 Materi hard-delete + R2 cleanup compensating.
> Estimasi: 8-10 hari inline, 4-5 hari kalau delegasi codex untuk CRUD scaffolding (3.A.1 + 3.A.2 + 3.C.1 + 3.C.2 + 3.F.1).
> Konvensi sub-fase: 3.A Bab BE (4 task) | 3.B Bab FE Guru (2 task) | 3.C Materi BE (4 task) | 3.D Materi FE (2 task) | 3.E Bab Siswa + Progress (2 task) | 3.F Pengumuman (3 task) = **17 task total**.

#### 3.A Bab Backend

**Task 3.A.1 — Migration `000005_bab.up.sql` + Bab GORM model + repo dasar** ✅ DONE 2026-05-21 (commit `aafcfa4` + renumber 000004→000005 dalam `<next-commit>`)
- Files: `backend/migrations/000005_bab.up.sql` + `down.sql`, `backend/internal/bab/{model,repo}.go`
- Schema (locked Section 6): `bab(id uuid pk, kelas_id uuid fk→kelas restrict, nomor int, judul text, deskripsi text, urutan int, status text default 'draft', version int default 1, created_at timestamptz, archived_at timestamptz null)`. Note: `archived_at` di-keep untuk tombstone hard archive — `Status='archived'` workflow guru tetap di kolom `status` (tunggal kolom enum, not bool). Cek catatan Section 6.1 line 478: gabung jadi 1 enum, **drop archived_at di Bab** (kelas tetap pakai). Update model + migration sesuai.
- Indexes: `(kelas_id, urutan)` btree, `(kelas_id, status)` btree (filter siswa published-only).
- Trigger `bab_set_updated_at` reuse `set_updated_at()` dari 000002 (kalau perlu `updated_at` — atau skip dan rely on `version` bump saja; Fase 2 kelas pattern → ada `updated_at`. **Decision: tambah `updated_at` di Bab juga**, konsistensi).
- Repo: `Create`, `FindByID`, `ListByKelas(kelas_id, includeArchived bool, statusFilter *string) []Bab`, `UpdateBasic(id, version int, fields map)` dgn optimistic concurrency `WHERE id=? AND version=?` + reprobe → `ErrVersionConflict` vs `gorm.ErrRecordNotFound` (mirror `kelas.Repo.UpdateBasic`), `Archive(id, version)` (idempotent guard 409 already_archived), `UpdateStatus(id, version, status)` (transition guard: draft↔published↔archived).
- Verify: `go build ./... && go test ./...` + `migrate up` di workspace → cek `\d bab` show schema + indexes.
- Commit: `feat(migrations): 000004 bab + status enum + version`, `feat(bab): GORM model + repo + optimistic concurrency`

**Task 3.A.2 — Bab CRUD service + handler (Create/List/Get/Patch/Archive)** ✅ DONE 2026-05-21 (commit `377eed8`; live E2E smoke 11/11 PASS)
- Files: `backend/internal/bab/{service,handler,handler_test}.go`. Wire di `cmd/server/main.go` group `/api/v1` dgn middleware order `BearerAuth → ForceChangePassword → RoleGuard(admin,guru) → kelasOwnershipGuard`.
- Endpoints (Section 7):
  - `POST /kelas/:id/bab` body `{nomor, judul, deskripsi}` (urutan auto = max+1, status default draft, version=1)
  - `GET /kelas/:id/bab` — list, query `?status=draft|published|archived&include_archived=true`
  - `GET /bab/:id` — detail
  - `PATCH /bab/:id` body `{nomor?, judul?, deskripsi?, urutan?, status?, version}` partial pointer fields (mirror `kelas` Patch pattern dari 2.B.2). Status transition: draft↔published↔archived semua valid (no funnel constraint MVP).
  - `POST /bab/:id/archive` (idempotent 409 already_archived; sets `status='archived'`, no separate `archived_at`)
- Audit log: `bab_created/bab_updated/bab_status_changed/bab_archived` dgn `target_kelas_id` + `meta={bab_id, status_lama?, status_baru?}` (locked #59 prep).
- Ownership guard: kelas dari URL `:id` (untuk POST/GET list) atau Bab.KelasID (untuk GET/PATCH/Archive by bab id) wajib `kelas.guru_id=current_user_id` (atau admin role).
- Verify: build/vet/test + handler tests (happy path + version conflict 409 + ownership 403 + archived bab patch reject).
- Commit: `feat(bab): CRUD service + handler + audit log`

**Task 3.A.3 — Bab reorder bulk endpoint** ✅ DONE 2026-05-21 (commit `6b0f041`; live E2E smoke 6/6 PASS)
- Files: `backend/internal/bab/reorder.go` (+ handler test)
- Endpoint: `POST /kelas/:id/bab/reorder` body `{order: [bab_id1, bab_id2, ...], versions: {bab_id: version, ...}}`
- Service: transaction loop `UpdateColumn("urutan", index)` per bab_id + cek `kelas_id=<:id>` ownership + cek `version=versions[bab_id]` per row + auto bump version. Kalau ANY row mismatch version → tx rollback + 409 `version_conflict` body `{conflicts: [{bab_id, current_version}, ...]}`.
- Validate: `len(order)` harus = jumlah bab di kelas (no add/remove via reorder). Duplicate bab_id → 400 `duplicate_in_order`. Bab dari kelas lain di order → 400 `bab_not_in_kelas`.
- Audit: 1 entry per call `bab_reordered` dgn `target_kelas_id` + `meta={order: [...]}`.
- Verify: handler test mixed scenarios + race protection (version conflict mid-tx).
- Commit: `feat(bab): bulk reorder endpoint with version guard`

**Task 3.A.4 — Bab duplicate endpoint** ✅ DONE 2026-05-21 (commit `fcbf532`; live E2E smoke 8/8 PASS)
- Files: `backend/internal/bab/duplicate.go` (+ handler tambahan + 7 tests)
- Endpoint: `POST /bab/:id/duplicate` body `{judul?}` → bikin bab baru status=`draft`, version=1, urutan=max+1, copy `nomor` + `deskripsi`. Default judul = `<source_judul> (Salinan)`.
- **MVP scope (bab-only):** copy fields bab saja. Child copy (materi PDF + pengumuman) di-defer karena Materi+Pengumuman tabel belum ada (Task 3.C.1 + 3.F.1).
- **Extension hook:** `Service.SetChildCopier(childCopier)` interface — pas Materi+Pengumuman selesai, wire concrete impl di main.go yang copy DB rows + R2 CopyObject + compensating delete on rollback. Service.Duplicate sudah jalan inside tx, child copy plug in seamlessly. `meta.materi_count` + `meta.pengumuman_count` siap di audit log (sekarang 0).
- Guards: source 404 → 404 `not_found`; kelas archived → 409 `kelas_archived`; source archived → 409 `already_archived`; non-owner → 403 `forbidden`.
- Audit: `bab_duplicated` w/ meta `{source_bab_id, new_bab_id, new_judul, new_urutan, materi_count, pengumuman_count}` verified live.
- Tests: 7 handler tests (no-body / custom judul / 404 / 403 / 409 kelas_archived / 409 already_archived / invalid uuid / bad json). Total bab handler tests: 30.
- Live E2E smoke 8/8 PASS — login admin → create source bab urutan=1 → duplicate no-body (judul auto `(Salinan)`, urutan=2, draft, v1, deskripsi copied) → duplicate w/ custom judul (urutan=3) → 404 bogus id → archive source → 409 already_archived → 400 invalid uuid → cleanup.
- Commit: `feat(bab): duplicate endpoint POST /bab/:id/duplicate (Task 3.A.4)`

#### 3.B Bab Frontend Guru

**Task 3.B.1 — Tab "Bab" di kelas detail page (list + DnD reorder + create/edit/archive/duplicate)** ✅ DONE 2026-05-21 (commits `97d7b28` feat + `4cc70dc` chore deps + `c474a36` lint fix; server build/typecheck/lint PASS).
- New deps: `@dnd-kit/core@6.1.0` + `@dnd-kit/sortable@8.0.0` + `@dnd-kit/utilities@3.2.2`. package-lock.json + next-env.d.ts pulled dari server post-install.
- New files:
  - `frontend/lib/bab-api.ts` — typed client (listBab, createBab, getBab, updateBab, archiveBab, duplicateBab, reorderBab) + `friendlyBabError(err, action)` helper untuk Indonesian copy mapping (invalid_body, version_conflict, forbidden, not_found, already_archived, kelas_archived, reorder_missing_bab, etc).
  - `frontend/components/bab/BabSortableCard.tsx` — sortable card w/ GripVertical handle (left, listener-bound), CardBody (nomor + judul + StatusBadge + truncated deskripsi), ActionsMenu dropdown (Edit/Duplikat/Arsipkan). Plus exported `BabCardReadOnly` untuk archived-kelas case.
  - `frontend/components/bab/BabReorderList.tsx` — DnD list w/ @dnd-kit (DndContext + SortableContext + verticalListSortingStrategy + PointerSensor distance=4 + KeyboardSensor). Optimistic mutation: onMutate cancel+snapshot+setQueryData, onError rollback + 409 invalidate, onSettled invalidate.
  - `frontend/components/bab/BabFormDialog.tsx` — reusable Create+Edit dialog. Zod schema: nomor int 1-999, judul max 200, deskripsi max 2000, status enum 'draft'|'published' (archive via dedicated dialog). Re-sync defaults on open. Edit kirim version + invalidate on 409.
  - `frontend/components/bab/ArchiveBabDialog.tsx` — destructive confirm. 409 already_archived → informative toast (non-destructive variant) + invalidate + close.
  - `frontend/components/bab/DuplicateBabDialog.tsx` — input judul opsional, placeholder show "<src> (Salinan)" preview. Toast w/ new bab nomor+urutan.
  - `frontend/components/bab/BabListSection.tsx` — orchestrator. useQuery key `['guru','kelas','bab', kelasID, includeArchived]` staleTime 15s, computes `nextNomor = max(items.nomor)+1` (useMemo'd), wires 4 dialogs, toggle "Tampilkan yang diarsipkan", refresh button, kelas-archived banner.
- Modified `frontend/app/(authed)/guru/kelas/detail/page.tsx`:
  - `TabKey` extended `'bab' | 'pengaturan' | 'siswa' | 'pengumuman'`.
  - TABS array: Bab → Pengaturan → Siswa → Pengumuman (Bab default).
  - Initial state: `useState<TabKey>('bab')`.
  - Render slot: `{tab === 'bab' && <BabListSection kelasID={kelas.id} archived={archived} />}`.
- Server verify: `npm install` added 6 packages, `tsc --noEmit` clean, `next build` static export 21/21 PASS, `next lint` clean for new files (1 pre-existing role-guard warning unchanged).
- Bundle impact: `/guru/kelas/detail` 14 kB → 26.9 kB (DnD + 6 komponen + react-hook-form forms; akseptabel).
- Caveats: BabFormDialog status select hanya expose draft/published (un-archive bukan MVP — re-create from duplicate kalau perlu). Archive dialog idempotent terhadap 409 already_archived (informative, bukan error). Reorder pakai versions map dari current cache snapshot — kalau race vs guru lain, 409 akan rollback + force refresh sesuai locked #56.
- Commit: `feat(fe-guru): tab bab di kelas detail + dnd reorder + crud dialogs`

**Task 3.B.2 — `/guru/kelas/detail/bab` shell page (sub-tabs Materi/Pengumuman/Pengaturan, Soal+Tugas placeholder)** ✅ DONE 2026-05-21 (commit `5282cad`; server build/typecheck/lint PASS, 22/22 static pages).
- New file: `frontend/app/(authed)/guru/kelas/detail/bab/page.tsx` — query-param routing `?id=<kelas>&bid=<bab>` (static-export-friendly mirror Task 2.B.4).
- Header: nomor + judul + StatusBadge + breadcrumb "Kelas <nama>" (kelas name dari useQuery getKelas) + Refresh/Edit/Duplikat/Arsipkan buttons.
- Reuses Task 3.B.1 dialogs as-is: BabFormDialog (mode='edit'), ArchiveBabDialog, DuplicateBabDialog. Duplicate sukses memanggil baru `DuplicateBabDialog.onSuccess(newBab)` → router.push ke `/guru/kelas/detail/bab?id=<kelas>&bid=<newBab>` (smooth nav ke salinan).
- Sub-tabs (5 total): Materi/Soal/Tugas/Pengumuman pakai `PlaceholderTab` w/ pointer ke task masing-masing. Pengaturan TAB AKTIF: inline `PengaturanTab` form react-hook-form + zod (nomor 1-999, judul max 200, deskripsi max 2000, status enum draft|published) → PATCH dgn `version` field; 409 → toast + invalidate + form re-sync via React.useEffect([defaults]).
- Loading skeleton (3 placeholder blocks) + error states 404/403/generic dgn link kembali ke `/guru/kelas/detail`.
- Component changes (REUSE-ready helpers):
  - `frontend/components/bab/BabSortableCard.tsx` — judul jadi `<Link href="/guru/kelas/detail/bab?id=...&bid=...">` (hover:underline). Readonly variant `linkToDetail={false}` saat kelas archived.
  - `frontend/components/bab/DuplicateBabDialog.tsx` — added optional `onSuccess?: (newBab: Bab) => void` prop.
- Server verify: `tsc --noEmit` clean, `next lint` clean for new files (1 pre-existing role-guard warning unchanged), `next build` static export 21→22 pages PASS.
- Bundle impact: `/guru/kelas/detail` 26.9 kB → 7.12 kB (Next codesplit ngangkat dialog komponen ke shared chunk antara `/detail` dan `/detail/bab`); `/guru/kelas/detail/bab` 4.18 kB / 200 kB First Load.
- Caveats: Pengaturan TAB sengaja BUKAN reuse penuh BabFormDialog body — inline form lebih cocok di full page (no Dialog wrapper, has cancel-to-pristine button). Status select sama2 cuma expose draft/published (un-archive bukan MVP, archive lewat tombol header).
- Commit: `feat(fe-guru): bab detail shell page + sub-tabs scaffold`

#### 3.C Materi Backend

**Task 3.C.1 — Migration `000006_materi.up.sql` + Materi GORM model + repo** ✅ DONE 2026-05-21 (commit `7772f63`; server migrate up→6 + down/up roundtrip verified, build/test clean).
- Files shipped: `backend/migrations/000006_materi.up.sql` + `down.sql`, `backend/internal/materi/{model,repo}.go`.
- Schema applied: `materi(id, kelas_id FK kelas RESTRICT, bab_id FK bab SET NULL nullable, judul, tipe enum pdf|youtube|markdown CHECK, konten, object_key/original_filename/mime_type/size_bytes nullable, urutan, version, timestamps)` + CHECK constraint `materi_tipe_payload_chk` enforcing tipe↔payload coherence (pdf MUST have R2 fields, youtube/markdown MUST NOT).
- Indexes: `(kelas_id, bab_id, urutan)` general, `(kelas_id, tipe)` for filter, partial `(bab_id, urutan) WHERE bab_id IS NOT NULL` for bab-scoped queries.
- `materi_read(materi_id FK CASCADE, siswa_id FK CASCADE, read_at, PK composite + idx siswa_id)` shipped for progress calc (locked #68).
- Repo lengkap: Create, FindByID, MaxUrutan(BabFilter), ListByKelas(BabFilter any|null|eq), ListByBab, CountByKelas, CountByBab, UpdateBasic dgn optimistic concurrency (#56), Delete returns ObjectKey, MarkRead idempotent ON CONFLICT DO NOTHING returns (read, wasNew), CountReadByBabSiswa untuk progress numerator. Tipe immutable after Create (per roadmap — caller harus delete + recreate).
- Verify: server `go vet`, `go build ./...`, `go test ./...` cached PASS, migrate up/down roundtrip clean.

**Task 3.C.2 — Materi CRUD endpoints — youtube + markdown (no upload)** ✅ DONE 2026-05-21 (commit `6e76b4c`; server build + vet + tests PASS).
- Files shipped: `backend/internal/materi/{service,handler,youtube,handler_test,youtube_test}.go`. Wired di `backend/cmd/server/main.go` (kelasGroup + materiGroup).
- `parseYouTubeID` supports 4 formats + http/https + www/m subdomain + youtube-nocookie + scheme-less input. 25 test cases (14 happy paths covering format variations + uppercase host + underscore/dash id + 11 error paths covering missing id, short/long id, invalid chars, ftp scheme, non-YouTube hosts).
- Endpoints:
  - `POST /kelas/:id/materi` body `{bab_id?, judul, tipe in ('youtube','markdown'), konten}` — youtube parsed → store video_id; markdown body capped via `MaxMarkdownBytes = 50KB`. tipe='pdf' rejects with `tipe_unsupported`.
  - `GET /kelas/:id/materi?bab_id=<uuid|null>` — list scoped, ownership guarded. `bab_id=null` maps ke filter `bab_id IS NULL`.
  - `GET /materi/:id` — detail.
  - `PATCH /materi/:id` body `{judul?, konten?, urutan?, version}` — tipe immutable. konten edit re-parsed per `existing.Tipe`: youtube re-parse URL, markdown size-validate, pdf rejects with `tipe_immutable`. No-op PATCH (no field changed) returns existing tanpa bump version.
  - `DELETE /materi/:id` — hard delete; returns `{materi_id, tipe, object_key?, pending_r2_cleanup?}`. ObjectKey populated only when tipe=pdf — signal untuk 3.C.3 compensating R2 sweep.
- Service: `assertBabInKelas` validates bab→kelas FK saat bab_id supplied. `canManageKelas` (admin all, guru own). Audit log materi_created/updated/deleted dgn `TargetKelasID` populated untuk guru audit scope (#59). `marshalMeta` JSON.
- Handler tests: 14 cases (Create youtube happy, Create pdf 400 tipe_unsupported, Create kelas_archived 409, Create bab_not_in_kelas 400, List ?bab_id=null, List bab_id=invalid 400, List bab_id=<uuid>, Update version_conflict 409, Update missing version 400, Update tipe_immutable 409, Get not_found 404, Get forbidden 403, Delete markdown no object_key, Delete pdf returns object_key+pending_r2_cleanup, invalid UUID 400).
- Verify: server `go vet ./...`, `go build ./...`, `go test ./...` (materi 0.343s) PASS — live restart skip per user policy (req. permission), unit tests + build cukup koheren.

**Task 3.C.3 — Materi PDF upload + presigned download** ✅ DONE 2026-05-21 (commit `8c2b495`; server vet + build + tests PASS).
- Files shipped: `backend/internal/materi/upload.go` + `upload_test.go` (16 test cases). `service.go` + `handler.go` extended (Delete compensating cleanup, Upload+PresignFileURL service methods + handler routes). `cmd/server/main.go` wires `objectStore` ke `NewService` + mounts upload + file-url routes.
- `Service.Upload(ctx, kelasID, callerID, role, in UploadInput, ip, ua)` pipeline:
  1. R2 store nil check → `ErrR2Required` (503 r2_unavailable)
  2. judul + body validation → `ErrInvalidInput`
  3. size cap `MaxMateriBytes = 20MB` → `ErrPayloadTooLarge` 413 (locked #64)
  4. `http.DetectContentType` first 512B; require `application/pdf` prefix → `ErrUnsupportedMime` 415 (locked #46/#63)
  5. `findKelasOrForbidden` + ArchivedAt check + `assertBabInKelas`
  6. uuid → `object_key = "materi/<uuid>.pdf"` via `storage.BuildKey(CategoryMateri, ...)` (locked #58/#61)
  7. `MaxUrutan(BabFilter)` for next slot
  8. `store.PutObject`; on fail return `ErrUploadFailed` 500 r2_put_failed (no DB row yet)
  9. `repo.Create` with sanitized `OriginalFilename`; on fail run compensating `store.DeleteObject` in background ctx + audit `materi_r2_orphan` if cleanup itself fails
  10. audit `materi_uploaded` + return `*Materi`
- `Service.PresignFileURL`: ownership guard, reject non-pdf with `ErrTipeUnsupported`, `PresignGetDownload(key, PresignTTL=15m, original_filename)` for inline disposition (locked #62), audit `materi_file_url_issued`.
- `sanitizeFilename`: strips path separators, NUL, traversal segments; caps at 200 chars; falls back to `materi.pdf` on empty.
- `Service.Delete` extended: tipe=pdf triggers compensating `R2.DeleteObject` after DB delete; on R2 fail logs `materi_r2_orphan` (with reason + err) + sets `r2_orphan:true` flag in `materi_deleted` audit meta. Non-pdf rows skip R2 call entirely (no-op).
- Endpoints:
  - `POST /api/v1/kelas/:id/materi/upload` (multipart): FormFile pre-check + `io.LimitReader` (defense in depth) + judul/bab_id parsing → svc.Upload → 201 `{materi, object_key, original_filename, size_bytes}`.
  - `GET /api/v1/materi/:id/file-url` → 200 `{url, expires_at, original_filename, mime_type}`.
- 16 test cases shipped:
  - Service.Upload: happy (sanitizes traversal in filename, persists object_key+size), mime mismatch (R2 stays empty), payload too large, DB fail with compensating R2 delete (R2 stays empty), kelas archived (R2 stays empty), bab not in kelas (R2 stays empty), R2 store unconfigured.
  - Service.PresignFileURL: pdf happy + audit `materi_file_url_issued`, non-pdf rejected `ErrTipeUnsupported`, not_found.
  - Service.Delete: pdf w/ R2 delete happy, pdf w/ R2 fail logs `materi_r2_orphan` + flags `r2_orphan:true` in `materi_deleted` meta, non-pdf skips R2 call.
  - Handler.Upload: multipart happy, mime mismatch 415, missing file 400.
  - Handler.FileURL: happy, non-pdf 400 `tipe_unsupported`.
- Verify: server `go vet ./...`, `go build ./...`, `go test ./...` all PASS — live restart skip per user policy (lihat catatan deploy di Current Next Step).

**Task 3.C.4 — MateriRead endpoint (siswa mark-as-read)** ✅ DONE 2026-05-21 (commit `caad20a`; server vet + build + tests PASS).
- Files: `backend/internal/materi/read.go` (3997 bytes) + `backend/internal/materi/read_test.go` (8536 bytes — 11 test cases). Wiring: `service.go` (enrollmentLookup interface + 6th NewService arg), `handler.go` (materiService interface +MarkRead), `cmd/server/main.go` (mount + pass kelasRepo as enrollmentLookup), `upload_test.go`/`handler_test.go` (fixtures extended).
- Endpoint: `POST /api/v1/siswa/materi/:id/read` (siswa-only via existing siswaGroup BearerAuth + ForceChangePassword + RoleGuard(siswa)). Returns `{materi_id, read_at, was_new}` on 200; 403 untuk guru/admin + non-enrolled siswa + removed-enrollment siswa; 404 untuk materi missing; 400 untuk invalid uuid.
- Idempotent: `Repo.MarkRead` `INSERT ... ON CONFLICT DO NOTHING` returns `wasNew bool`. First call → was_new=true, subsequent calls → was_new=false (read_at preserved dari original timestamp).
- Service guard chain: role check (siswa-only defensive) → materi find (ErrNotFound mapped) → enrollment lookup `kelas.Repo.FindEnrollment(kelasID, siswaID)` (missing → ErrForbidden, status≠active → ErrForbidden) → repo MarkRead idempotent.
- Audit: skipped per design (read events too chatty). slog level=Debug records per-call: `materi_id`, `siswa_id`, `was_new` for ops introspection only.
- 11 test cases shipped: Service.MarkRead happy/idempotent/guru-rejected/admin-rejected/materi-not-found/no-enrollment/removed-enrollment/enroll-nil-disabled (8) + Handler.MarkRead happy 200 with payload assertion / forwards-Forbidden 403 / invalid-uuid 400 (3).
- Verify: local `go build ./...` PASS, `go vet ./...` PASS, `go test ./internal/materi/...` PASS 1.107s, `go test ./...` PASS. Server `go vet ./...` + `go build ./...` + `go test ./...` PASS, materi pkg 0.022s.
- Commit: `caad20a feat(materi): siswa MarkRead idempotent endpoint + enrollment guard`

#### 3.D Materi Frontend

**Task 3.D.1 — Tab Materi di bab detail (guru) — create dialog + list + edit/delete** ✅ DONE 2026-05-21 (commit `eeca652`; server typecheck + `next build` PASS, /guru/kelas/detail/bab page bundle 54.3 kB).
- Files shipped: `frontend/lib/youtube.ts` (port `parseYouTubeID` mirror locked #65 4 formats: youtu.be, youtube.com/watch, /shorts, /embed; tryParseYouTubeID + youtubeEmbedURL + youtubeWatchURL), `frontend/lib/materi-api.ts` (typed client: listMateri, getMateri, createMateri, updateMateri, deleteMateri, getMateriFileURL, uploadMateriPDF multipart hand-rolled fetch + bearer; friendlyMateriError), `frontend/components/materi/{MarkdownEditor,YouTubeInput,PdfUpload,MateriCreateDialog,MateriEditDialog,MateriList}.tsx`.
- Wiring: `frontend/app/(authed)/guru/kelas/detail/bab/page.tsx` Materi tab swap PlaceholderTab → `<MateriList kelasID babID contextLabel disabled={archived} />`. PlaceholderTab tetap idle untuk soal/tugas/pengumuman.
- Create dialog: radio cards 3-tipe (PDF/YouTube/Markdown). Per-tipe field — pdf=`<PdfUpload>` drag-drop (.pdf+20MB FE preflight, locked #46/#64), youtube=`<YouTubeInput>` URL → live tryParseYouTubeID + youtube-nocookie iframe preview (locked #65), markdown=`<MarkdownEditor>` split write/preview pakai react-markdown + remark-gfm + char counter cap 50KB locked #63. Submit pdf → multipart `/kelas/:id/materi/upload`, yt+md → JSON `/kelas/:id/materi`. Tipe lock di state — tidak bisa swap mid-create.
- Edit dialog: tipe IMMUTABLE (locked #63, server reject `tipe_immutable`). Hanya judul + konten editable. Untuk youtube: rebuild watch URL dari video_id (`youtubeWatchURL`) supaya user lihat URL aslinya, server re-parse via `parseYouTubeID`. Untuk pdf: konten read-only, ganti file = delete+create. Optimistic concurrency `version` di body (locked #56); 409 → toast + invalidate untuk refetch + form re-sync via React.useEffect.
- List: TanStack Query `['guru','materi','list',kelasID,babID??'free']` staleTime 15s. Card per materi dengan icon (`FileText`/`Youtube`/`Type`) + judul + tipe-badge + meta (urutan, version, size_bytes utk pdf, video_id utk yt). DropdownMenu `<MoreVertical>` align=end: Buka PDF (tipe pdf only, presigned URL via `getMateriFileURL` → `window.open` no-opener) → Edit → Hapus. Delete confirm dialog separate; PDF response `pending_r2_cleanup` → toast note "akan dibersihkan oleh sweeper".
- FE-side validation: PDF mime check (application/pdf atau filename `.pdf`) + size cap 20MB pre-upload (server tetap re-validate via `http.DetectContentType` locked #46). Markdown TextEncoder bytes cap 50KB. YouTube live regex 11-char [A-Za-z0-9_-] (mirror backend `videoIDRe`). Server tetap final authority — FE cuma UX guard.
- Verify: server `npx tsc --noEmit` PASS, `npm run build` PASS (22 static routes generated, 1 ESLint warning di lib/role-guard.tsx pre-existing — not from this task). Local skip karena no runtime deps. Live deploy belum (FE static export auto-served oleh `lms-api` binary; binary perlu rebuild + restart untuk pick up `frontend/.next` dir baru — blocked oleh user di sesi ini).
- Commit: `eeca652 feat(fe-guru): materi tab — create dialog 3-tipe + list + edit/delete`

**Task 3.D.2 — Siswa materi viewer (PDF iframe + YouTube embed + react-markdown) + auto mark-read** ✅ DONE 2026-05-21 (commit `d08df3f`; server typecheck + `next build` PASS, 22 static routes — viewer komponen standalone, belum di-import bundle ke /siswa page karena dipakai di Task 3.E.2).
- Files shipped: `frontend/components/materi/useMarkMateriRead.ts` (shared hook fire-and-forget mutation, retry=false, silent on error by default; opts notifyOnError + onSuccess), `frontend/components/materi/PdfViewer.tsx` (TanStack Query presigned URL staleTime 10min — server TTL 15min sisain 5min buffer locked roadmap §3.D.2; iframe browser-native render 70vh min 480px; debounce 2s mark-read), `frontend/components/materi/YouTubeEmbed.tsx` (youtube-nocookie iframe locked #65; defensive 11-char video_id regex; mark-read on mount tanpa debounce), `frontend/components/materi/MarkdownView.tsx` (react-markdown + remark-gfm prose render; mark-read on mount), `frontend/components/materi/MateriViewer.tsx` (dispatcher switch-by-tipe + standar header card dgn judul + tipe badge + meta; hideHeader prop opsional).
- Files diubah: `frontend/lib/materi-api.ts` — tambah `MarkReadResponse` type + `markMateriRead(id)` helper untuk POST `/siswa/materi/:id/read` (siswaGroup BearerAuth + ForceChangePassword + RoleGuard(siswa) di backend, plus enrollment guard di service.MarkRead, locked #25 idempotent ON CONFLICT DO NOTHING).
- Mark-read strategy:
  - PDF debounce 2s → hindari fire saat user scroll-by tab (locked roadmap §3.D.2). Mounting dianggap intensional setelah 2s kontak.
  - YouTube + Markdown fire-on-mount langsung (load iframe / render markdown udah cukup signal).
  - Hook fire-and-forget: tidak invalidate query, error silent default. Read state ditampilin lewat bab detail siswa endpoint (Task 3.E.1) yang refetch saat user navigate balik.
- Verify: server `npx tsc --noEmit` PASS, `npm run build` PASS (22 static routes generated). Bundle /guru/kelas/detail/bab tetap 54.3 kB (viewer belum di-import — dipakai nanti di Task 3.E.2 page yg belum ada). No new ESLint warnings.
- Belum di-wire ke `/siswa/kelas/detail/bab` page — itu Task 3.E.2 yang depend Task 3.E.1 BE GET endpoints siswa bab list + detail. Komponen 3.D.2 standalone, siap import.
- Commit: `d08df3f feat(fe-siswa): materi viewer 3-tipe + auto mark-read`

#### 3.E Bab Siswa + Progress

**Task 3.E.1 — GET endpoints siswa bab list + bab detail dgn progress** ✅ DONE 2026-05-21 (commit `c0d795a`; server `go vet`/`go build`/`go test ./...` all PASS, siswabab pkg 16 new test cases hijau di 0.013s, materi pkg re-test 0.056s).
- Files baru: `backend/internal/siswabab/student.go` + `backend/internal/siswabab/student_test.go`. **PAKET TERPISAH** — bukan di `backend/internal/bab/` karena `materi` package udah depend ke `bab` (untuk `bab.Bab` di duplicate flow), jadi `bab` gak bisa import `materi` balik. Siswa flow yang butuh dua-duanya hidup di paket sendiri.
- Files diubah: `backend/internal/materi/repo.go` (3 method baru: `CountByBabBatch` + `CountReadByBabBatch` + `ListReadIDsByBabSiswa` — single-query group-by per bab id, zero-fill missing groups via pre-allocated map; hindari N+1), `backend/cmd/server/main.go` (mount handler routes + import siswabab package).
- Endpoint: `GET /api/v1/siswa/kelas/:id/bab` mounted under `siswaGroup` (BearerAuth + ForceChangePassword + RoleGuard(siswa)). Returns `{ items: [SiswaBabItem], total }` dengan progress per bab. Filter implicit `Status=published` (siswa gak lihat draft/archived). Response shape: `{ id, nomor, judul, deskripsi, urutan, status, progress: { persen, breakdown: { materi: { pct, w: 1.0 } }, bab_kosong, materi_read, materi_total } }`. Komponen Fase 4-7 (latihan/ulangan/tugas) belum di-include — landingnya nanti dengan pct=null + w=0 di breakdown.
- Endpoint: `GET /api/v1/siswa/bab/:id` mounted di route group yang sama. Returns `{ bab: SiswaBabItem, materi: [SiswaMateriCard] }` dengan `sudah_dibaca` per materi. Bab dengan status≠published → 404 `not_found` (hindari leak draft/archived ke siswa).
- Authorization defensive (Service re-checks meski handler udah RoleGuard supaya unit test gak butuh fiber app):
  - callerRole MUST be siswa (admin/guru → ErrForbidden)
  - kelas lookup → kalau missing → ErrForbidden (collapse ke 403 supaya siswa gak bisa probe kelas existence via 404 vs 403)
  - enrollment MUST be `EnrollmentActive` (missing OR `EnrollmentRemoved` → ErrForbidden)
  - bab status≠published → ErrNotFound
- Performance: 1 query untuk `bab.ListByKelas(Status=published)` + 2 batched queries untuk progress (`CountByBabBatch` + `CountReadByBabBatch` group by `bab_id`, zero-fill di Go). GetSiswa pakai `materi.ListByBab` + `materi.ListReadIDsByBabSiswa` (Pluck) — total 4 round trip per detail (Bab.FindByID + enrollment + materi list + read-IDs).
- Progress formula: `materi_read / materi_total × 100` rounded ke 2 desimal (locked #68 + Section 6.4). Zero-materi → `bab_kosong=true` + `persen=0`. Test coverage: `TestComputeProgress_BoundaryCases` (kosong, all-read, none-read, third 33.33, two thirds 66.67).
- 16 test cases shipped: Service.ListSiswa happy/non-siswa/non-enrolled/removed-enrollment/missing-kelas (5) + Service.GetSiswa happy/draft-hidden/archived-hidden/non-enrolled (4) + computeProgress 5 sub-cases (1 with 5 sub-tests) + Handler ListSiswa happy/forbidden-guru/invalid-id/unexpected-500 (4) + Handler GetSiswa happy/draft-404 (2).
- Verify: server `go vet ./...` PASS, `go build ./...` PASS, `go test ./internal/siswabab/...` PASS 16/16 cases di 0.013s. Full repo `go test ./...` PASS.
- Migration: tidak ada migration baru (re-pakai materi_read table dari 000006_materi).
- Commit: `c0d795a feat(bab): siswa list bab + detail with progress fase-3-partial`

**Task 3.E.2 — `/siswa/kelas/detail` (list bab + progress) + `/siswa/kelas/detail/bab` (materi viewer)** ✅ DONE 2026-05-21 (commit `3a69ddb`; server `npx tsc --noEmit` PASS, `npm run build` PASS — 24 static routes generated, dua route baru `/siswa/kelas/detail` 2.65 kB + `/siswa/kelas/detail/bab` 8.6 kB First Load 169 kB).
- Files baru:
  - `frontend/lib/siswa-bab-api.ts` — typed client untuk endpoint Task 3.E.1: `listSiswaBab(kelasID)` (GET `/siswa/kelas/:id/bab`) + `getSiswaBab(babID)` (GET `/siswa/bab/:id`). Types mirror BE: `SiswaBabItem`, `SiswaBabProgress` (persen + breakdown + bab_kosong + materi_read/total), `SiswaMateriCard` (strip guru-only fields), `SiswaBabBreakdownItem` (pct nullable + w).
  - `frontend/components/siswa/SiswaBabProgressBar.tsx` — inline Tailwind progress bar (no shadcn Progress yet). Color tier: muted (kosong/0%), primary (1-99%), emerald (100%). Native title tooltip "X dari Y materi sudah dibaca (Z%)". role=progressbar + aria-valuenow untuk a11y. Size variant `sm` (list) + `md` (header detail).
  - `frontend/components/siswa/siswaCardToMateri.ts` — adapter `SiswaMateriCard` (strip guru fields) → `Materi` shape lengkap. Field missing diisi safe defaults (null/0/''). Dipakai sebelum pass ke `<MateriViewer hideHeader>`.
  - `frontend/app/(authed)/siswa/kelas/detail/page.tsx` — kelas detail page (`?id=:kelasID`). Static-export friendly via query string (mirror pola `/guru/kelas/detail`). Hydrate kelas info dari `listMyKelas` (cari id di items) — tidak ada `GET /siswa/kelas/:id` dedicated. Bab list pakai `listSiswaBab` — handle 403 forbidden (gak enroll) + empty state ramah. BabCard click → push ke `/siswa/kelas/detail/bab?id=&bid=`.
  - `frontend/app/(authed)/siswa/kelas/detail/bab/page.tsx` — bab detail page (`?id=:kelasID&bid=:babID`). Header: nomor + judul + breadcrumb + progress bar md size. Tab Materi default — list expandable card (klik buka `<MateriViewer hideHeader>` sebagai body). Mark-read auto-fired oleh viewer subcomponents (mount/debounced 2s untuk PDF). Setelah 3s open, parent invalidate query detail (read state) + parent list (progress per bab). Tab Soal/Tugas/Pengumuman placeholder pointer ke Fase 4-5 + Task 3.F (mirror GuruBabDetail pattern).
- Files diubah:
  - `frontend/app/(authed)/siswa/page.tsx` — enable tombol "Buka" di list kelas (sebelumnya disabled span). Sekarang asChild Link ke `/siswa/kelas/detail?id=:id`.
- Read-state UX:
  - Card tampilkan badge "✓ Dibaca" kalau `sudah_dibaca=true`.
  - PDF mark-read debounce 2s + parent refetch 3s setelah open → cukup buffer.
  - YouTube + Markdown fire-on-mount langsung (load iframe / render markdown udah cukup signal).
- Static export safety: pakai query string (`?id=&bid=`) bukan dynamic route segments, konsisten dengan `/guru/kelas/detail` (Task 2.B.4) + `/admin/pengguna/detail`.
- Verify: server `npx tsc --noEmit` PASS, `npm run build` PASS (24 static routes, +2 dari sebelumnya). Bundle `/siswa/kelas/detail/bab` 8.6 kB / 169 kB First Load (vs guru `/guru/kelas/detail/bab` 11 kB / 251 kB — siswa view lebih ringan karena gak include MateriCreate/Edit dialogs). Local skip karena no runtime deps.
- Commit: `3a69ddb feat(fe-siswa): kelas detail + bab detail pages with progress + materi viewer`

#### 3.F Pengumuman

**Task 3.F.1 — Migration `000007_pengumuman.up.sql` + Pengumuman model + repo + CRUD endpoints** ✅ DONE 2026-05-21 (commit `cf8c5bc`; server `go vet` PASS, `go build ./...` PASS, `go test ./internal/pengumuman/... -v` 18/18 PASS di 0.062s, `go test ./...` ALL packages PASS, migration up applied to dev DB clean — schema verified via `\d pengumuman`).
- Files baru: `backend/migrations/000007_pengumuman.up.sql` + `down.sql`, `backend/internal/pengumuman/{model,repo,service,handler,handler_test}.go`.
- Files diubah: `backend/cmd/server/main.go` — import + wire pengumumanRepo/Svc/Handler, mount routes (POST/GET di kelasGroup, GET/PATCH/DELETE di pengumumanGroup, GET siswa-scope di siswaGroup).
- Schema: `pengumuman(id uuid pk, kelas_id uuid fk→kelas RESTRICT, bab_id uuid? fk→bab SET NULL, judul text, isi text default '', created_by_id uuid fk→users RESTRICT, status text default 'published' check in ('published','archived'), version int default 1, created_at, updated_at timestamptz)`. Three indexes: `(kelas_id, created_at DESC)` primary list query, `(bab_id, created_at DESC) WHERE bab_id IS NOT NULL` partial bab-scoped, `(kelas_id, status, created_at DESC)` siswa published-only filter. Trigger `pengumuman_set_updated_at` reuses `set_updated_at()` dari 000002. `schema_meta` updated to `'000007_pengumuman'`.
- Endpoints (mounted in main.go):
  - `POST /api/v1/kelas/:id/pengumuman` (guru/admin own kelas) body `{judul, isi, bab_id?}`. 201 + `{pengumuman}`. Sentinel mapping: `kelas_archived` 409, `bab_not_in_kelas` 400.
  - `GET /api/v1/kelas/:id/pengumuman?bab_id=<uuid|null>&status=<published|archived>&limit=<int>` (guru/admin) — full visibility, status optional. 200 `{items, total}`.
  - `GET /api/v1/siswa/kelas/:id/pengumuman?bab_id=&limit=` (siswa enrolled) — service ALWAYS forces `status=published` (handler-passed status ignored), enrollment guard via `assertEnrolled`.
  - `GET /api/v1/pengumuman/:id` — guru/admin owner full visibility; siswa enrolled + `status=published` only (archived → 404, no info leak).
  - `PATCH /api/v1/pengumuman/:id` (guru/admin owner) body `{version, judul?, isi?, status?}`. Optimistic concurrency #56. Audit `pengumuman_archived` saat status flip published→archived, `pengumuman_updated` else.
  - `DELETE /api/v1/pengumuman/:id` (guru/admin owner) hard delete. 200 `{pengumuman_id}`. Audit `pengumuman_deleted`.
- Authorization (defensive — service re-checks meski handler RoleGuard):
  - Create/Update/Delete: `findKelasOrForbidden` (admin all, guru own kelas).
  - List/Get guru/admin: full visibility incl. archived.
  - List siswa: `assertEnrolled` (active enrollment) + force `Status=&StatusPublished` di repo filter.
  - Get siswa: archived → `ErrNotFound` (no info leak), enrollment required.
- Locked decisions referenced: #56 optimistic concurrency, #66 passive timestamp (no per-siswa read receipt — FE pakai created_at vs last_seen client-side untuk badge "Baru"), #20 bab nullable.
- Caps: `MaxJudulBytes` 200 chars, `MaxIsiBytes` 50KB (mirror materi markdown locked roadmap §3.C.2). `DefaultListLimit` 50, `MaxListLimit` 200.
- Audit log entries (4 actions): `pengumuman_created`, `pengumuman_updated`, `pengumuman_archived` (status flip), `pengumuman_deleted`.
- Service constructor signature: `NewService(repo, kelas, bab, enroll, audit)`. Wired di main.go pakai `pengumuman.NewService(pengumumanRepo, kelasRepo, babRepo, kelasRepo, authRepo)` — `kelasRepo` dipakai dua kali untuk `kelasLookup` + `enrollmentLookup` karena method-set overlap (sama pola dengan siswabab Task 3.E.1).
- Test coverage (handler_test.go): 18 cases — 14 service-level + 4 handler smoke. Service: create happy/empty-judul/isi-too-long/kelas-archived/not-owner/bab-not-in-kelas, list siswa force-published-only/not-enrolled, list guru full-visibility, get siswa archived-hidden, update version-conflict, update archive-audit-action, delete not-found/happy. Handler: create happy, list invalid status, update version conflict, delete not found.
- Verify: server full repo `go test ./...` PASS (admin/auth/bab/importjob/kelas/materi/middleware/pengumuman/siswabab/storage all OK). Migration up applied to dev DB; `\d pengumuman` confirms 3 indexes, status CHECK, FK kelas/bab/users, set_updated_at trigger.
- Commit: `cf8c5bc feat(pengumuman): migration 000007 + CRUD endpoints (Task 3.F.1)`

**Task 3.F.2 — FE guru: tab Pengumuman di kelas detail + bab detail (compose + edit + archive)** ✅ DONE 2026-05-21 (commit `1ab48f7`; server `npx tsc --noEmit` PASS, `npm run build` PASS — 22 static routes, no lint warnings).
- Files baru:
  - `frontend/lib/pengumuman-api.ts` — typed client mirror BE Task 3.F.1: types `Pengumuman/PengumumanStatus`, CRUD (`listPengumuman`/`listSiswaPengumuman`/`getPengumuman`/`createPengumuman`/`updatePengumuman`/`deletePengumuman`), helpers `isPengumumanNew` (7-day threshold per #66) + `friendlyPengumumanError`. `bab_id` semantics: undefined=no filter, null='null' (kelas-wide), uuid=bab-scoped — sama BE.
  - `frontend/components/pengumuman/PengumumanComposer.tsx` — dialog buat pengumuman baru. Reuse `<MarkdownEditor>` dari sub-fase 3.D.1, cap 200 char judul + 50KB isi (mirror BE `MaxJudulBytes`/`MaxIsiBytes`). Auto-derive scope dari prop `babID` (null=kelas-wide, uuid=bab-scoped).
  - `frontend/components/pengumuman/PengumumanEditDialog.tsx` — edit dialog (judul/isi/status). Optimistic concurrency #56 — kirim version dari snapshot, 409 → invalidate + refetch + re-sync. Status field via `<StatusOption>` cards (published/archived radio).
  - `frontend/components/pengumuman/PengumumanList.tsx` — guru list dengan status filter tabs (all/published/archived). Card expandable markdown body (chevron toggle), badge "Baru" kalau <7 hari (locked #66, calc client-side), aksi via DropdownMenu: Edit / Archive / Aktifkan / Hapus. Status invalidation: invalidate ALL 3 filter variants pas mutation supaya status flip pindah bucket.
- Files diubah:
  - `frontend/app/(authed)/guru/kelas/detail/page.tsx` — tab Pengumuman placeholder diganti `<PengumumanList kelasID={kelas.id} babID={null} contextLabel="Pengumuman ke seluruh siswa kelas {nama}." disabled={archived} />`.
  - `frontend/app/(authed)/guru/kelas/detail/bab/page.tsx` — tab Pengumuman placeholder diganti `<PengumumanList kelasID babID contextLabel="Pengumuman untuk Bab {nomor} — {judul}." disabled={archived} />`.
- Locked decisions: #56 optimistic concurrency, #66 passive timestamp + 7-day "Baru" threshold, #20 BabID nullable.
- Verify: server `npx tsc --noEmit` PASS, `npm run build` PASS. Bundle `/guru/kelas/detail` 6.99 kB / 252 kB (vs sebelum +negligible — markdown deps sudah ada dari materi 3.D.1). `/guru/kelas/detail/bab` 9.61 kB / 254 kB.
- Commit: `1ab48f7 feat(fe-pengumuman): combined Task 3.F.2+3.F.3 — guru CRUD + siswa read-only`

**Task 3.F.3 — FE siswa: read-only pengumuman list di kelas detail + bab detail** ✅ DONE 2026-05-21 (commit `1ab48f7`; sama dengan 3.F.2 — combined ship satu commit).
- Files baru: `frontend/components/pengumuman/PengumumanReadList.tsx` — read-only list pakai `listSiswaPengumuman` (BE force `status=published` + enrollment guard). Card expandable markdown body, badge "Baru" client-side (<7 hari, locked #66), no mark-read action (passive timestamp). Optional `expandFirst` prop untuk auto-expand pengumuman terbaru di kelas page.
- Files diubah:
  - `frontend/app/(authed)/siswa/kelas/detail/page.tsx` — tambah Card section "Pengumuman kelas" di bawah Bab list, pakai `<PengumumanReadList kelasID babID={null} expandFirst emptyState="Belum ada pengumuman dari guru." />`. Auto-expand pengumuman pertama supaya siswa langsung lihat update terbaru.
  - `frontend/app/(authed)/siswa/kelas/detail/bab/page.tsx` — tab Pengumuman placeholder diganti Card + `<PengumumanReadList kelasID babID emptyState="Belum ada pengumuman untuk bab ini." />`.
- Verify: server `npx tsc --noEmit` PASS, `npm run build` PASS. Bundle `/siswa/kelas/detail` 3.21 kB / 165 kB (markdown deps shared via chunk — minimal increase). `/siswa/kelas/detail/bab` 9.91 kB / 172 kB.
- Lint cleanup commit `6d3cc6f`: wrap `now` di `React.useMemo` pakai `dataUpdatedAt` untuk hindarin `react-hooks/exhaustive-deps` warning, dan wrap `items` di useMemo agar useEffect deps stable.
- Commit: `1ab48f7` (combined) + `6958676` (strict-null guard `items[0]`) + `6d3cc6f` (lint cleanup).

---

### Fase 4 — Tugas + Submission + Late + Resubmit (3-4 hari)

> Locked decisions: #70 Submission single-row + version bump | #71 Late hard-block kalau `IzinkanLate=false` + penalty calc `NilaiAsli × (1 - PenaltyPersen/100)` | #72 Submission attachment optional + per-tugas `WajibAttachment` + cap 5 file × 20MB | #73 Submit `SELECT FOR UPDATE` tx + idempotent + 409 `already_graded` | #74 Tugas attachment cap 5 file × 20MB + R2 prefix `tugas/`.
> Estimasi: 18-22 task. Konvensi sub-fase: 4.A Tugas BE (4 task) | 4.B Tugas FE Guru (2 task) | 4.C Submission BE (4 task) | 4.D Submission FE Siswa (2 task) | 4.E Submission Review FE Guru (2 task) = **14 task total**.

#### 4.A Tugas Backend

**Task 4.A.1 — Migration `000008_tugas.up.sql` + Tugas GORM model + repo dasar** ✅ DONE 2026-05-21 (commit `b6a2cf9`; server `go vet` PASS, `go build ./...` PASS, migrate up→8 + down/up roundtrip clean, schema verified `\d tugas` + `\d tugas_attachment`).
- Files shipped: `backend/migrations/000008_tugas.up.sql` + `down.sql`, `backend/internal/tugas/{model,repo}.go`.
- Schema applied: `tugas(id uuid pk, kelas_id FK kelas RESTRICT, bab_id FK bab SET NULL nullable, judul, deskripsi, deadline timestamptz nullable, izinkan_late bool, penalty_persen smallint CHECK 0-100, wajib_attachment bool, status enum CHECK draft|published|archived, version int, created_by_id FK users RESTRICT, timestamps)` + `tugas_attachment(id, tugas_id FK CASCADE, object_key, original_filename, mime_type, size_bytes, created_at)`.
- Indexes verified: `(kelas_id, status, created_at DESC)` general, `(bab_id, status) WHERE bab_id IS NOT NULL` partial, `(kelas_id, deadline) WHERE deadline IS NOT NULL` partial, `(tugas_id)` on attachment.
- Repo lengkap: Create, FindByID (Preload Attachments), ListByKelas/ListByBab w/ ListFilter (Status + BabFilter + Limit), CountByKelas/CountByBab, UpdateBasic dgn optimistic concurrency #56 (auto-bump version + updated_at, fields map approach untuk partial patch), Delete returns []ObjectKey untuk compensating R2 cleanup (locked #69), AddAttachment, FindAttachmentByID, ListAttachmentsByTugas, CountAttachmentsByTugas (cap enforcement #74), DeleteAttachment returns ObjectKey, DB() exposes *gorm.DB.
- Verify: server `go vet` clean, `go build ./...` PASS, migrate up→8 + down→7 + up→8 roundtrip clean, schema PSQL verified.

**Task 4.A.2 — Tugas CRUD service + handler (Create/List/Get/Patch/Delete)** ⏳ NEXT
- Files: `backend/internal/tugas/{service,handler,handler_test}.go`. Wire di `cmd/server/main.go` group `/api/v1` dgn middleware order `BearerAuth → ForceChangePassword → RoleGuard(admin,guru) → kelasOwnershipGuard`.
- Endpoints (Section 7 + locked #58/#62):
  - `POST /kelas/:id/tugas` body `{judul, deskripsi?, bab_id?, deadline?, izinkan_late?, penalty_persen?, wajib_attachment?, status?}` (defaults: status=draft, version=1, izinkan_late=false, penalty_persen=0, wajib_attachment=false). Validate: penalty_persen 0-100, deadline boleh null tapi kalau set wajib > now (warn 400 `deadline_in_past` kalau gak), bab_id (kalau set) wajib `kelas_id=<:id>` (400 `bab_not_in_kelas`).
  - `GET /kelas/:id/tugas?bab_id=<uuid|null>&status=<draft|published|archived>&limit=<int>` — guru/admin full visibility, siswa enrolled force `status=published` (mirror pengumuman 3.F.1 pattern).
  - `GET /tugas/:id` — guru/admin owner full; siswa enrolled + status=published only (archived/draft → 404, no info leak).
  - `PATCH /tugas/:id` body `{version, judul?, deskripsi?, bab_id?, deadline?, izinkan_late?, penalty_persen?, wajib_attachment?, status?}` partial pointer fields (mirror `kelas.Patch`). Optimistic concurrency #56. Status transition: draft↔published↔archived. Bab_id mutable (boleh pindah bab, atau ke null = kelas-wide).
  - `DELETE /tugas/:id` hard delete + compensating R2 cleanup untuk semua tugas_attachment ObjectKeys + cascade delete submission + submission_attachment via FK. Audit `tugas_deleted` w/ meta `{tugas_id, attachment_count, submission_count}`. **Note**: kalau ada submission graded, tetap allow delete tapi audit log capture pre-delete count untuk transparansi (locked #59 guru audit scope).
- Audit log: `tugas_created/tugas_updated/tugas_status_changed/tugas_deleted` dgn `target_kelas_id` + meta lengkap.
- Ownership guard: kelas dari URL `:id` (POST/list) atau `Tugas.KelasID` (GET/PATCH/DELETE by tugas id) wajib `kelas.guru_id=current_user_id` atau admin.
- Verify: build/vet/test + handler tests (happy + version conflict 409 + ownership 403 + archived kelas reject + deadline_in_past warning + penalty_persen out of range).
- Commit: `feat(tugas): CRUD service + handler + audit log (Task 4.A.2)`

**Task 4.A.3 — Tugas attachment upload endpoint (multipart)** ✅ DONE 2026-05-21 (commit `55fb86a`; server `go vet` PASS, `go build ./...` PASS, `go test ./internal/tugas/... -v` 23/23 PASS, full repo `go test ./...` ALL PASS).
- Files shipped: `backend/internal/tugas/{attachment,attachment_handler}.go`. Service signature refactored: `NewService(repo, kelas, bab, enroll, audit, store)` — store optional (nil = disable upload path), all 17 test calls bumped to 6 args.
- Endpoints (4 total) wired ke `tugasGroup` (siswa role allowed, service branches by role):
  - `POST /tugas/:id/attachments` (multipart `file`) — guru/admin owner only. Validate via locked #46 (mime sniff via `http.DetectContentType` 512-byte, allowlist `pdf, docx (sniffs as application/zip), jpg, png, zip`), cap 20MB per file, cap 5 per tugas (count+1>5 → 400 attachment_limit_reached). Object key `tugas/<uuid>.<ext>`. R2 PutObject → DB insert → compensating R2 DeleteObject if DB fails (locked #62 + #69 pattern). Audit `tugas_attachment_uploaded` + `tugas_attachment_orphan`.
  - `GET /tugas/:id/attachments` — guru/admin owner OR siswa enrolled + tugas published.
  - `DELETE /tugas/:id/attachments/:attID` — guru/admin owner. Hard delete + R2 DeleteObject compensating cleanup. Audit `tugas_attachment_deleted` + orphan log if R2 fail.
  - `GET /tugas/:id/attachments/:attID/url` — guru/admin owner OR siswa enrolled + tugas published. Presigned GET TTL 15m, attachment disposition. Audit `tugas_attachment_url_issued`.
- Error mapping: 400 invalid_id/missing_file/attachment_limit_reached, 403 forbidden, 404 not_found, 409 kelas_archived, 413 payload_too_large, 415 unsupported_mime, 500 r2_put_failed, 503 r2_unavailable.
- Constants: `MaxTugasAttachmentBytes=20MB`, `MaxAttachmentsPerTugas=5`, `PresignTTL=15min`.
- Note docx detection: stdlib `http.DetectContentType` returns `application/zip` for both .zip and .docx (DOCX = ZIP container). Allowlist accepts both — original filename extension preserved separately for download UX.
- Verify: server `go vet` clean, `go build ./...` PASS, 23/23 tests PASS dalam 0.015s, full repo `go test ./...` PASS.
- Caveats: integration test untuk R2 PutObject path butuh real R2 atau mock — tests yang aktif sekarang cover service logic (sentinel errors + audit log) tapi gak end-to-end PutObject. Real upload smoke ditunda ke saat 4.B.1 FE guru udah bisa hit endpoint dari browser.

**Task 4.A.4 — Tugas duplicate endpoint** ✅ DONE (commit `3600188`)
- Files: `backend/internal/tugas/duplicate.go` (NEW), `backend/internal/storage/{storage,r2,mock}.go` (CopyObject method).
- Endpoint: `POST /tugas/:id/duplicate` body `{judul?}` → 201 + `{tugas: <new>}`. Default judul = `<source_judul> (Salinan)`. Status reset ke `draft`, version=1, semua field source (deskripsi, deadline, izinkan_late, penalty_persen, wajib_attachment, bab_id) ikut.
- Storage: tambah `Storage.CopyObject(srcKey, dstKey)` ke interface — R2Client pakai `s3.CopyObject` server-side (no body transfer), MockStorage deep-copies bytes. R2 attachment di-copy ke `tugas/<uuid_baru>.<ext>` per source attachment.
- Compensating: R2 CopyObject FIRST → DB tx (tugas + attachment rows) → kalau tx fail, DeleteObject untuk semua copied keys. Pre-flight uniqueness via UUID, no race.
- Guards: source 404 (`not_found`), kelas archived 409 (`kelas_archived`), source archived 400 (`invalid_input`), non-owner 403 (`forbidden`), R2 unavailable 503 (`r2_unavailable`).
- Audit: `tugas_duplicated` w/ meta `{source_tugas_id, new_tugas_id, new_judul, attachment_count}`.
- Verify: server `go vet` clean, `go build` PASS, smoke E2E live: ORIGINAL published+v=1 → DUPLICATE draft+v=1 + auto-suffix `(Salinan)` ✅; non-owner guru2 → 403; missing id → 404; custom judul accept ✅.
- Commit: `feat(tugas): duplicate endpoint w/ R2 CopyObject (Task 4.A.4)`

#### 4.B Tugas Frontend Guru

**Task 4.B.1 — Tab "Tugas" di kelas detail page + bab page (list + filter status + create/edit/archive + attachment manager)** ✅ DONE (commit `c4acf54`, merged kelas-wide + bab page sekaligus)
- Files baru:
  - `frontend/lib/tugas-api.ts` — typed client + helpers (listTugas, listSiswaTugas, getTugas, createTugas, updateTugas, deleteTugas, listAttachments, uploadAttachment, deleteAttachment, getAttachmentURL, formatDeadline, isOverdue) + constants (MAX_TUGAS_DESKRIPSI_BYTES 50KB, MAX_TUGAS_JUDUL_LENGTH 200, MAX_TUGAS_ATTACHMENT_BYTES 20MB, MAX_TUGAS_ATTACHMENTS 5, TUGAS_ATTACHMENT_ACCEPT) + `friendlyTugasError(err, action)` Indonesian copy mapping (16 error codes).
  - `frontend/components/tugas/TugasComposer.tsx` — Create dialog: judul, deskripsi markdown (reuse MarkdownEditor), deadline picker (datetime-local → ISO UTC saat submit), izinkan_late checkbox, penalty_persen 0-100 conditional, wajib_attachment checkbox, publish-immediately checkbox (default draft). On success: invalidate + auto-open Edit dialog (via `onCreated` callback) supaya guru langsung upload attachment.
  - `frontend/components/tugas/TugasEditDialog.tsx` — 2-section dialog: (1) Edit metadata + status picker 3-mode (draft/published/archived) + dirty-check submit guard + optimistic concurrency (kirim version snapshot). (2) Attachment manager: list (count badge + filename + mime + size) + Upload button (multipart, mime allowlist, 20MB pre-validate, 5-cap pre-validate) + Buka (presigned 15-min, opens new tab) + Delete (window.confirm guard).
  - `frontend/components/tugas/TugasList.tsx` — orchestrator card. useQuery key `['guru','tugas','list', kelasID, babID??'kelas-wide', statusFilter]` staleTime 15s. Status filter tabs (all/draft/published/archived), invalidate ALL filter variants on mutation. Card per tugas: chevron expand markdown deskripsi, judul + status badge + "Lewat deadline" red badge kalau overdue+published, attachment count badge, deadline + version + penalty + wajib lampiran meta. Dropdown menu: Edit/Publish (if draft)/Arsipkan (if published)/Aktifkan (if archived)/Hapus (confirm dialog).
- Files diubah:
  - `frontend/app/(authed)/guru/kelas/detail/page.tsx` — TabKey extended `'tugas'`, TABS array tambah `{key:'tugas', label:'Tugas', Icon:ClipboardList}` (urutan: Bab → Pengaturan → Siswa → Tugas → Pengumuman), wire `<TugasList kelasID babID={null} contextLabel="Tugas kelas-wide untuk {nama}." disabled={archived} />`.
  - `frontend/app/(authed)/guru/kelas/detail/bab/page.tsx` — placeholder `<PlaceholderTab>` diganti `<TugasList kelasID babID={babID} contextLabel="Tugas untuk Bab {nomor} — {judul}." disabled={archived} />`.
- Verify: server `npx tsc --noEmit` PASS, `npm run build` PASS (22 static routes — kelas-wide page 258 kB, bab page 261 kB), Go `build ./cmd/server` PASS (binary path = `/home/ubuntu/lms/backend/bin/lms-api`, NOT `/usr/local/bin`).
- Restart: `sudo systemctl restart lms-api` → active PID 2229459. Smoke: healthz=200, tugas/:id=401, attachments=401, attachments/:attID/url=401 (semua auth-required, bukan 404 routes-not-found). BE 4.A.1-3 + FE 4.B.1 live bersamaan dengan 1× restart.
- Caveats: shadcn `Checkbox` component belum scaffolded di repo, pakai native `<input type="checkbox" className="size-4 rounded border-input">`. Composer save attachment butuh tugas_id, jadi auto-open Edit dialog setelah create supaya guru langsung bisa upload (UX 1-shot flow).
- Commit: `feat(fe-tugas): TugasComposer + EditDialog + List + wire 2 pages (Task 4.B.1)` (covers 4.B.2 juga karena bab page sekalian).

**Task 4.B.2 — Tab "Tugas" di bab detail page** ✅ DONE (covered by 4.B.1 commit `c4acf54`)
- Sudah ter-wire bersama 4.B.1: prop `babID={babID}` di bab page = bab-scoped, prop `babID={null}` di kelas page = kelas-wide. Filter & invalidate keys handle 'kelas-wide' vs uuid string. Composer bawa `babID` ke `bab_id` di POST. Tidak butuh commit terpisah.

#### 4.C Submission Backend

**Task 4.C.1 — Migration `000009_submission.up.sql` + Submission GORM model + repo** ✅ DONE (commit `55296be`)
- Files: `backend/migrations/000009_submission.up.sql` + `down.sql`, `backend/internal/submission/{model,repo}.go`.
- Schema: `submission(id uuid pk, tugas_id uuid fk→tugas CASCADE, siswa_id uuid fk→users RESTRICT, catatan text default '', status text default 'submitted' check (status in ('submitted','graded','returned')), is_late bool default false, nilai_asli numeric(5,2) null, penalty_persen_applied smallint null, nilai_setelah_penalty numeric(5,2) null, feedback text default '', graded_by_id uuid? fk→users SET NULL, graded_at timestamptz null, version int default 1, submitted_at timestamptz default now(), updated_at timestamptz default now(), unique(tugas_id, siswa_id))`.
- `submission_attachment(id uuid pk, submission_id uuid fk→submission CASCADE, object_key text not null, original_filename text not null, mime_type text not null, size_bytes bigint not null, created_at timestamptz)` — locked #72.
- Indexes: `(tugas_id, status)` (rekap guru filter graded vs ungraded), `(siswa_id, submitted_at DESC)` (history per siswa), `(graded_by_id, graded_at DESC) WHERE graded_by_id IS NOT NULL` partial (audit guru grading).
- Trigger `submission_set_updated_at` reuse `set_updated_at()`.
- Repo: `Upsert(tx, sub)` (SELECT FOR UPDATE atau INSERT ON CONFLICT (tugas_id, siswa_id) DO UPDATE), `FindByTugasSiswa(tugasID, siswaID)` w/ Preload Attachments, `FindByID(id)` w/ Preload, `ListByTugas(tugasID, statusFilter, limit)` (rekap guru), `ListBySiswa(siswaID, kelasID, limit)` (siswa view history), `UpdateGrade(tx, id, version, nilai, penalty, nilai_final, feedback, graded_by, graded_at)` optimistic concurrency, `DeleteAttachmentsBySubmission(tx, submissionID)` returns ObjectKeys, `AddAttachment(tx, att)`, `LockForUpdate(tx, tugasID, siswaID)` raw `SELECT ... FOR UPDATE`. `schema_meta` update to `'000009_submission'`.
- Verify: migrate up applied dev DB → `\d submission` + `\d submission_attachment` confirm UNIQUE constraint + FK CASCADE.
- Commit: `feat(migrations): 000009 submission + attachment`, `feat(submission): GORM model + repo + FOR UPDATE`

**Task 4.C.2 — Submission Submit endpoint (siswa upload + late detection)** ✅ DONE (commit `6200d16`)
- Files: `backend/internal/submission/{service,handler,handler_test}.go`. Wire di `cmd/server/main.go` group `/api/v1/siswa`.
- Endpoint: `POST /siswa/tugas/:id/submit` (multipart form, fields `catatan`, `files[]`) — RoleGuard(siswa) + EnrollmentGuard(via tugas.KelasID).
- Flow (locked #73):
  - BEGIN tx → SELECT tugas + (lock submission row if exists FOR UPDATE) → guards:
    - tugas.Status != 'published' → 404 `not_found`
    - tugas.Deadline set + now > Deadline + tugas.IzinkanLate=false → 403 `deadline_passed` (locked #71)
    - existing submission.Status IN ('graded','returned') → 409 `already_graded`
    - tugas.WajibAttachment=true + len(files)==0 → 400 `attachment_required`
    - len(files) > 5 → 400 `attachment_limit_reached`
  - per file: mime sniff + size cap (locked #46/#72) → reject 400 `invalid_attachment` early
  - kalau resubmit (existing row): collect old ObjectKeys → DELETE submission_attachment rows → defer R2 DeleteObject di goroutine post-commit
  - per new file: R2 PutObject → INSERT submission_attachment
  - is_late = (tugas.Deadline != null && now > Deadline)
  - UPSERT submission (status='submitted', is_late, version+1, submitted_at=now)
  - Audit `submission_submitted` (atau `submission_resubmitted` kalau version > 1) w/ meta `{tugas_id, attachment_count, is_late, old_object_keys (kalau resubmit)}` — locked #71 audit `submission_late` if is_late=true
  - COMMIT.
- Compensating R2 cleanup: defer cancel-style — kalau tx rollback, R2 DeleteObject untuk semua newly-PutObject keys.
- Verify: handler tests (happy + late accept w/ flag + late hard-block + already_graded + wajib_attachment + attachment cap + resubmit version bump + R2 compensating on rollback).
- Commit: `feat(submission): submit endpoint w/ late + idempotent + FOR UPDATE (Task 4.C.2)`

**Task 4.C.3 — Submission Get/List + presigned attachment URL** ✅ DONE (commit `6200d16`)
- Endpoints:
  - `GET /siswa/tugas/:id/submission` — siswa, return own submission (kalau ada) atau 404 + tugas info (deadline, izinkan_late, penalty_persen) untuk pre-fill UI.
  - `GET /tugas/:id/submissions?status=` — guru/admin owner, list submission per tugas (rekap untuk grading).
  - `GET /submission/:id` — guru/admin owner OR siswa pemilik. Preload attachments + grade fields.
  - `GET /submission/:id/attachments/:attID/url` — guru/admin owner OR siswa pemilik. Presigned GET TTL 15m, `attachment` disposition. Audit `file_url_issued`.
- Caveats: siswa GET own submission HARUS pakai endpoint `/siswa/tugas/:id/submission` (auto-derive submission via tugas+siswa), bukan `/submission/:id` direct (yg butuh ownership lookup ekstra).
- Verify: handler tests (siswa hanya lihat own + guru lihat semua di kelas + cross-kelas 403 + 404 untuk submission yg gak ada).
- Commit: `feat(submission): get + list + presigned URL (Task 4.C.3)`

**Task 4.C.4 — Submission Grade endpoint (guru kasih nilai + penalty calc)** ✅ DONE (commit `6200d16`)
- Endpoint: `POST /submission/:id/grade` — guru/admin owner. Body `{nilai_asli (0-100, decimal-2), feedback?, version}`.
- Flow (locked #71/#73):
  - BEGIN tx → SELECT submission FOR UPDATE → cek `Status='submitted'` (kalau `graded` → 409 `already_graded`, kalau `returned` → 400 `cannot_grade_returned` defer MVP) + version match
  - Penalty calc: kalau `is_late && tugas.PenaltyPersen > 0` → `nilai_setelah_penalty = round(nilai_asli * (1 - PenaltyPersen/100), 2)` else `nilai_setelah_penalty = nilai_asli`. `penalty_persen_applied = is_late ? tugas.PenaltyPersen : 0`.
  - UPDATE submission (status='graded', nilai_asli, penalty_persen_applied, nilai_setelah_penalty, feedback, graded_by_id, graded_at=now, version+1)
  - Audit `tugas_graded` w/ meta `{submission_id, tugas_id, siswa_id, nilai_asli, penalty_persen_applied, nilai_setelah_penalty, is_late}`
  - COMMIT.
- Defer MVP: regrade endpoint (`PATCH /submission/:id/grade`) + return-for-revision flow (`status='returned'`). Kalau guru salah grade, hapus + siswa resubmit untuk MVP.
- Verify: handler tests (happy w/ late penalty, happy w/o late, version conflict 409, already_graded 409, ownership 403, validation nilai out of range).
- Commit: `feat(submission): grade endpoint w/ penalty calc (Task 4.C.4)`

#### 4.D Submission Frontend Siswa

**Task 4.D.1 — Tab "Tugas" di siswa bab detail (list tugas published) + halaman submit** ✅ DONE (commits `9f8e9d0` + `fe50c66`)
- Files baru:
  - `frontend/lib/submission-api.ts` — typed client (getMySubmission, submitTugas multipart, getMySubmissionAttachmentURL).
  - `frontend/components/siswa/SiswaTugasCard.tsx` — list card (judul + deadline + status badge: belum-submit/submitted/graded + nilai kalau graded + late badge).
  - `frontend/components/siswa/SiswaTugasList.tsx` — list section di tab Tugas, useQuery `listTugas` w/ status=published filter.
  - `frontend/app/(authed)/siswa/kelas/detail/tugas/page.tsx` — submit page `?id=:kelasID&tid=:tugasID`. Pre-fill kalau udah pernah submit (catatan + attachment list w/ download). Banner "Late submission akan kena penalty xx%" kalau lewat deadline + izinkan_late=true. Disable submit kalau lewat deadline + izinkan_late=false. Form: textarea catatan + multi-file picker (max 5 × 20MB, mime hint). Submit progress + invalidate on success.
- Files diubah:
  - `frontend/app/(authed)/siswa/kelas/detail/bab/page.tsx` — tab Tugas placeholder diganti `<SiswaTugasList kelasID babID />`.
  - `frontend/app/(authed)/siswa/kelas/detail/page.tsx` — opt: tambah tab Tugas kelas-wide (bab_id=null) atau skip kalau prefer bab-scoped only MVP.
- Verify: server build + lint clean. Bundle impact OK.
- Commit: `feat(fe-submission): siswa tugas list + submit page w/ late banner (Task 4.D.1)`

**Task 4.D.2 — Riwayat submission siswa di dashboard** ✅ DONE (commits `5d160b6` + `9d5eda2` + `6f49e14`)
- BE: `backend/internal/submission/{repo.go,mine.go,handler.go}` + route wire di `cmd/server/main.go`.
  - `Repo.ListBySiswaWithTugas(siswaID, limit)` — JOIN submission + tugas snapshot dalam single roundtrip (cegah N+1 di FE).
  - `Service.ListMine(siswaID, limit)` — mapping ke flat `MySubmissionItem` DTO untuk FE.
  - Endpoint: `GET /api/v1/siswa/submissions?limit=` (RoleGuard siswa, default 100, cap 500).
- FE: `frontend/lib/submission-api.ts` (`MySubmissionItem` types + `listMySubmissions`), `frontend/app/(authed)/siswa/tugas/page.tsx` (NEW, 380 LOC).
  - Page features: 3 stat cards (total/menunggu/graded), status filter tabs (all/submitted/graded), group by kelas dengan link buka kelas detail, per-row card menampilkan judul tugas + status badge + late badge + nilai (kalau graded, dengan penalty preview) + feedback + deadline.
  - Deep link ke `/siswa/kelas/detail/tugas?id=&tid=` untuk buka tugas.
- Sidebar: `frontend/app/(authed)/siswa/layout.tsx` — tambah nav item "Tugas saya" (ClipboardList icon) di antara Dashboard dan Gabung Kelas.
- Verify: server `go vet` + `go build` PASS, `npx tsc --noEmit` PASS, `npm run build` PASS (22 static routes, /siswa/tugas 3.54 kB), HTTP 200 verified. Smoke E2E live: siswa1 has 1 graded submission ✅ render dengan nilai 87.5.
- Commits: `feat(submission): ListMine cross-kelas + GET /siswa/submissions (Task 4.D.2 BE)` + `fix(submission): add time import in repo.go` + `feat(fe-submission): /siswa/tugas page + sidebar nav (Task 4.D.2 FE)`

#### 4.E Submission Review Frontend Guru

**Task 4.E.1 — Halaman review submission per tugas (list + grading dialog)** ✅ DONE (commit `0775ead`)
- Files baru:
  - `frontend/lib/submission-guru-api.ts` — typed client (listTugasSubmissions, getSubmission, gradeSubmission, getSubmissionAttachmentURL).
  - `frontend/components/tugas/SubmissionReviewList.tsx` — list per tugas, status filter tabs (all/submitted/graded), badge LATE merah kalau is_late, sort by submitted_at DESC. Action button "Beri Nilai" buka dialog grading.
  - `frontend/components/tugas/GradeSubmissionDialog.tsx` — dialog grading, Zod schema `nilai_asli` 0-100 decimal-2 + `feedback` max 5000 + `version`. Display: catatan siswa + attachment list (download presigned). Penalty preview kalau is_late: "Nilai akan jadi `nilai * (1 - PenaltyPersen/100)`". Submit → POST `/submission/:id/grade` → invalidate list + close.
  - `frontend/app/(authed)/guru/kelas/detail/tugas/page.tsx` — review page `?id=:kelasID&tid=:tugasID`. Header: tugas info + tombol Edit/Hapus. Body: `<SubmissionReviewList />`.
- Files diubah: `TugasCard.tsx` (Task 4.B.1) tombol "Lihat Submission" → router.push ke review page.
- Verify: server build + lint clean.
- Commit: `feat(fe-submission): guru review + grading dialog (Task 4.E.1)`

**Task 4.E.2 — Pending counter (partial; activity feed defer Fase 7)** ✅ DONE (commits `a4f14a4` + `34aff41`)
- BE: `backend/internal/submission/pending.go` (NEW) + `backend/internal/kelas/repo.go` (`ListIDsByGuru`) + handler wire di `cmd/server/main.go`.
  - `kelas.Repo.ListIDsByGuru(guruID)` — Pluck kelas IDs (active+archived) untuk aggregate scope.
  - `submission.PendingCounter` — single SQL aggregate `COUNT(*) FROM submission s JOIN tugas t WHERE s.status='submitted' AND t.kelas_id IN (...)`. Guru: scope to owned kelas; admin: all kelas.
  - Endpoint: `GET /api/v1/guru/pending-counts` → `{ungraded_submissions: int}`. Forward-compat shape — `pending_review_ulangan_bab/harian` akan tambah di Fase 5/7.
- FE: `frontend/lib/guru-api.ts` (NEW, `getPendingCounts` typed client) + `frontend/app/(authed)/guru/{layout,page}.tsx`.
  - Layout sidebar: nav item Dashboard kasih `badgeKey: 'ungraded'` → render rose pill dengan count saat >0. Polling `useQuery` refetchInterval 30s + staleTime 15s. Mobile compact nav juga punya badge.
  - Dashboard `/guru` page: tambah stat card "Tugas perlu dinilai" (ClipboardCheck icon, rose accent kalau >0) dengan refetch 30s.
- Defer Fase 7: GET `/guru/feed` cursor pagination + event ledger (locked #39 + #55) — itu scope Fase 7 Activity Feed. 4.E.2 sini hanya counter.
- Verify: server `go vet` + `go build` PASS, FE `npx tsc --noEmit` PASS, `npm run build` PASS. Smoke E2E: 0 → siswa2 submit → counter 1 → grade akan turun 0.
- Commits: `feat(submission): pending-counts endpoint for guru sidebar (Task 4.E.2 BE)` + `feat(fe-submission): pending counter badge guru sidebar + dashboard card (Task 4.E.2 FE)`

---

### Current Next Step (Section 18)

**Fase 3 ✅ CLOSED 17/17.** Live deploy verified 2026-05-21 (commit `aca38e4`).

**Fase 4 plan ✅ DECOMPOSED 14 task** — locked #70-#74. Roadmap v0.9.0.
- 4.A Tugas BE: 3/4 ✅ DONE (4.A.1 migration `b6a2cf9`, 4.A.2 CRUD `dc7d237`, 4.A.3 attachment `55fb86a`); 4.A.4 duplicate ⏳ deferable
- 4.B FE Guru: 2/2 ✅ DONE (4.B.1 + 4.B.2 merged commit `c4acf54` — kelas-wide + bab page wired bersamaan)
- 4.C Submission BE: 0/4 pending
- 4.D FE Siswa: 0/2 pending
- 4.E Review FE Guru: 0/2 pending

Cumulative sesi: 6 commit shipped + dual-pushed (origin GitHub + server bare). Server tree synced ke `c4acf54`, DB version 8, lms-api binary live PID 2229459 — BE tugas endpoints (4.A.1-3) + FE guru tab Tugas (4.B.1-2) live bersamaan dengan 1× restart. Smoke: healthz=200, tugas+attachments endpoints return 401 (auth-required, bukan 404 routes-not-found).

**Eksekusi berikutnya: pilih satu**
- **gas Task 4.C.1 (Submission BE migration + model + repo)** — masuk ke meat sub-fase Submission. Migration `000009_submission.up.sql` (single-row + version bump per locked #70), GORM struct + repo CRUD. Estimasi 60-90 menit.
- **gas Task 4.A.4 (Tugas duplicate)** — defer-bisa, mirror pattern bab duplicate. R2 CopyObject untuk attachment. Estimasi 30-45 menit.
- **smoke E2E tugas dari browser** — login guru, bikin tugas, upload PDF/DOCX, edit, archive, hapus. Verify 4.A+4.B integration sebelum lanjut Submission. Estimasi 15-20 menit.
- **stop dulu** — Fase 4 progress 5/14 (36%); 4.A 75% + 4.B 100%. 6 commit + dual-pushed + live.

Rekomendasi: **smoke E2E dulu** sebentar (lu test sendiri dari browser), confirm bug-free, baru gas 4.C.1. Submission BE adalah bagian terpadat Fase 4 — kalau 4.A+4.B ada bug yang ketauan saat smoke, lebih murah fix sekarang sebelum submission FK ke tugas.

> Catatan FE Tugas (4.B): TugasComposer auto-open TugasEditDialog setelah create (callback `onCreated`) supaya guru bisa langsung upload attachment di flow yang sama. Status mutation invalidate ALL filter variants karena status flip pindah bucket. shadcn `Checkbox` belum di-scaffold; pakai native `<input type="checkbox" className="size-4 rounded border-input">` mirror BabListSection pattern. Binary path systemd: `/home/ubuntu/lms/backend/bin/lms-api` (pakai `go build -o` ke path itu, NOT `/usr/local/bin/lms-api`).
