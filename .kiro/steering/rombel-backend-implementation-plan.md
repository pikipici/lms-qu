# Rombel Backend Implementation Plan

## Tujuan

Pisahkan `rombel` dari `kelas pembelajaran` secara teknis di database, backend API, dan flow registrasi siswa.

- `Rombel`: struktur resmi sekolah, contoh `VII-A`, `VII-B`, `VIII-A`.
- `Kelas pembelajaran`: course operasional guru, contoh `Matematika VII-A Semester 1`.

## Kondisi Saat Ini

Saat ini UI admin sudah diarahkan ke `Sekolah & Rombel`, tetapi implementasi sementara masih memakai tabel/API `kelas` lama untuk rombel.

Risiko kondisi sementara:

- Rombel admin bisa muncul sebagai kelas guru.
- `guru_id` masih wajib untuk data rombel karena memakai tabel `kelas`.
- `/register` masih memilih data dari konsep kelas lama.
- Semantik data campur antara struktur sekolah dan course pembelajaran.

## Target Arsitektur

### Tabel Baru: `rombels`

Kolom awal:

- `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`
- `sekolah_id UUID NOT NULL REFERENCES sekolah(id) ON DELETE RESTRICT`
- `nama TEXT NOT NULL`
- `deskripsi TEXT NOT NULL DEFAULT ''`
- `active BOOLEAN NOT NULL DEFAULT true`
- `version INTEGER NOT NULL DEFAULT 1`
- `archived_at TIMESTAMPTZ NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`

Constraint/index:

- Unique aktif per sekolah: `UNIQUE(sekolah_id, lower(nama)) WHERE archived_at IS NULL`
- Index `sekolah_id`
- Index active list: `(sekolah_id, archived_at)`

### Tabel Membership Siswa-Rombel

Nama: `rombel_memberships`

Kolom awal:

- `rombel_id UUID NOT NULL REFERENCES rombels(id) ON DELETE RESTRICT`
- `siswa_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT`
- `status TEXT NOT NULL DEFAULT 'active'`
- `joined_via TEXT NOT NULL DEFAULT 'self_registration'`
- `joined_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `removed_at TIMESTAMPTZ NULL`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`

Constraint/index:

- Primary key `(rombel_id, siswa_id)`
- `status IN ('active', 'removed')`
- `joined_via IN ('self_registration', 'admin')`
- Index siswa: `(siswa_id, status)`
- Index rombel: `(rombel_id, status)`

### Update Join Request

Saat ini request siswa memakai `kelas_id`. Ubah bertahap:

1. Tambah kolom nullable `rombel_id UUID REFERENCES rombels(id) ON DELETE RESTRICT`.
2. Untuk request baru dari `/register`, wajib pakai `rombel_id`.
3. `kelas_id` dibuat nullable untuk backward compatibility.
4. Setelah migrasi penuh, `kelas_id` bisa deprecated untuk self-registration.

## Backend API Baru

### Admin Rombel API

Base path: `/api/v1/admin/sekolah/:sekolah_id/rombels`

Endpoint:

- `GET /api/v1/admin/sekolah/:sekolah_id/rombels`
  - List rombel sekolah.
  - Query: `page`, `page_size`, `include_archived`.

- `POST /api/v1/admin/sekolah/:sekolah_id/rombels`
  - Body: `{ "nama": string, "deskripsi"?: string }`.
  - Role: admin.

- `PATCH /api/v1/admin/rombels/:id`
  - Body: `{ "version": number, "nama": string, "deskripsi"?: string }`.
  - Optimistic lock via `version`.

- `POST /api/v1/admin/rombels/:id/archive`
  - Soft archive.
  - Ditolak kalau sudah archived.

- `DELETE /api/v1/admin/rombels/:id`
  - Hard delete hanya kalau kosong.
  - Kosong berarti tidak ada active membership dan tidak ada join request pending/approved/rejected yang masih referensi rombel.
  - Kalau tidak kosong, return `409 rombel_not_empty`.

### Public Registration API

Endpoint existing public perlu diarahkan ke rombel:

- `GET /api/v1/public/sekolah`
  - Tetap list sekolah yang self-registration enabled.

- `GET /api/v1/public/sekolah/:id/rombels`
  - List rombel aktif untuk dropdown `/register`.
  - Hanya sekolah dengan `siswa_registration_enabled=true`.

- `POST /api/v1/public/register-siswa`
  - Body berubah dari `kelas_id` ke `rombel_id`.
  - Validasi rombel aktif dan sekolah membuka pendaftaran.
  - Mode `approval_required`: buat join request dengan `rombel_id`.
  - Mode `auto_approve`: buat user siswa + active `rombel_memberships`.

## Service Layer

Buat package baru:

- `backend/internal/rombel/model.go`
- `backend/internal/rombel/repo.go`
- `backend/internal/rombel/service.go`
- `backend/internal/rombel/handler.go`

Service responsibilities:

- Validasi nama tidak kosong.
- Validasi sekolah ada.
- Validasi unique nama rombel aktif per sekolah.
- Optimistic locking untuk update.
- Archive soft delete.
- Hard delete only-if-empty.
- Audit log untuk create/update/archive/delete.

Error sentinel yang disarankan:

- `ErrInvalidInput`
- `ErrNotFound`
- `ErrConflict`
- `ErrAlreadyArchived`
- `ErrNotEmpty`
- `ErrVersionConflict`

## Update Flow Approval Siswa

Handler approval join request perlu bisa memproses `rombel_id`.

Saat approve:

1. Jika request punya `rombel_id`, insert/upsert `rombel_memberships`.
2. Jangan insert ke `kelas_enrollments`.
3. Jika request legacy punya `kelas_id`, pakai flow lama untuk backward compatibility.

UI admin/guru request siswa harus menampilkan:

- `sekolah_nama`
- `rombel_nama`
- fallback legacy `kelas_nama` kalau request lama.

## Update Frontend Setelah Backend Siap

### Admin Sekolah

Ganti temporary calls:

- Dari `listKelas({ sekolahId })` ke `listRombels(sekolahId)`.
- Dari `createKelas(...)` ke `createRombel(sekolahId, ...)`.
- Dari `updateKelas(...)` ke `updateRombel(...)`.
- Dari `archiveKelas(...)` ke `archiveRombel(...)`.
- Tambahkan tombol `Hapus` hanya untuk rombel kosong.

### Register

Ganti dropdown kelas menjadi rombel:

1. User pilih sekolah.
2. Frontend load `/public/sekolah/:id/rombels`.
3. User pilih rombel.
4. Submit `rombel_id`.

### Guru Kelas

Tidak perlu pakai rombel dulu untuk fase awal. Guru tetap membuat kelas pembelajaran sendiri.

Fase berikutnya baru tambahkan relasi optional:

- `kelas_rombel_links(kelas_id, rombel_id)`
- Guru bisa attach kelas pembelajaran ke rombel tertentu.

## Migrasi Data Sementara

Karena data rombel sementara sekarang tersimpan di tabel `kelas`, migrasi bisa dilakukan manual/bertahap.

Opsi aman:

1. Buat tabel `rombels` kosong.
2. Admin input ulang rombel resmi dari halaman Sekolah & Rombel baru.
3. Jangan otomatis migrasi semua `kelas`, karena tabel `kelas` juga berisi kelas pembelajaran guru.

Opsi semi-otomatis jika dibutuhkan:

- Migrasi hanya record `kelas` yang dibuat dari UI admin sementara dan punya `sekolah_id`.
- Tetap perlu review manual karena sulit membedakan rombel vs course dari nama saja.

Rekomendasi: pakai opsi aman, input ulang rombel resmi.

## Urutan Implementasi

1. Tambah migration `rombels` dan `rombel_memberships`.
2. Tambah kolom `rombel_id` nullable di join request table.
3. Buat package backend `internal/rombel`.
4. Register route admin rombel.
5. Register route public rombel.
6. Update public register service agar submit `rombel_id`.
7. Update join request approve flow agar masuk `rombel_memberships`.
8. Update frontend API client `rombel-api.ts`.
9. Update `/admin/sekolah` section rombel agar pakai API baru.
10. Update `/register` agar pakai rombel API baru.
11. Smoke test admin create/edit/archive/delete-empty rombel.
12. Smoke test siswa self-registration approval-required.
13. Smoke test siswa self-registration auto-approve.
14. Deploy.

## Acceptance Criteria

- Admin bisa tambah/edit/archive/hapus-kosong rombel dari edit sekolah.
- Admin tidak perlu memilih guru saat membuat rombel.
- Rombel tidak muncul di daftar kelas pembelajaran guru.
- Guru tetap bisa membuat kelas pembelajaran dari panel guru.
- `/register` memilih sekolah + rombel.
- Approval siswa memasukkan siswa ke `rombel_memberships`, bukan `kelas_enrollments`.
- Existing kelas pembelajaran guru tetap aman.
- Existing join request legacy masih bisa diproses atau minimal tidak membuat backend error.

## Catatan Penting

Implementasi ini sebaiknya tidak menghapus tabel/API `kelas`, karena `kelas` tetap dipakai sebagai kelas pembelajaran guru. Yang dihapus nanti hanya temporary usage rombel via API `kelas` di frontend admin sekolah.
