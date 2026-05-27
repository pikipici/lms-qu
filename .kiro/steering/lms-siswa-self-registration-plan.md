# LMS — Siswa Self-Registration Implementation Plan

> Status: **Draft / pending decision**  
> Owner: User + Apis  
> Last updated: 2026-05-27  
> Parent: `lms-roadmap.md` / Fase 8 polish backlog

---

## 1. Goal

Menambahkan fitur pendaftaran mandiri untuk siswa, tanpa menghilangkan kontrol admin atas struktur sekolah dan kelas.

Ekspektasi produk:
- Admin tetap setup master data `Sekolah` dan `Kelas`.
- Siswa bisa membuat akun sendiri dari halaman publik.
- Siswa memilih `Sekolah` dan `Kelas` dari dropdown yang disediakan sistem.
- Admin bisa menentukan mode onboarding: langsung masuk kelas atau wajib approval dulu.

---

## 2. Product Decision

### 2.1 Form daftar siswa

Form publik pendaftaran siswa:

```text
Nama Lengkap
Username
Password
Konfirmasi Password
Sekolah
Kelas
```

Catatan:
- `Sekolah` wajib berasal dari data yang dibuat admin.
- `Kelas` wajib berasal dari sekolah terpilih.
- Siswa tidak boleh mengetik sekolah/kelas bebas.
- Siswa tidak boleh memilih role; role selalu `siswa`.

### 2.2 Mode onboarding

Admin dapat memilih mode pendaftaran siswa:

| Mode | Perilaku |
|---|---|
| `auto_approve` | Setelah daftar, akun siswa aktif dan langsung masuk kelas pilihan. |
| `approval_required` | Setelah daftar, akun siswa aktif terbatas dan request masuk kelas menunggu approval admin/guru. |

Rekomendasi default: `approval_required`.

---

## 3. Scope

### In scope

- Backend endpoint publik untuk daftar siswa.
- Backend endpoint publik untuk list sekolah aktif dan kelas aktif per sekolah.
- Setting admin untuk mode onboarding siswa.
- Data model untuk join request atau enrollment pending.
- Halaman register siswa di frontend.
- Admin/guru UI untuk melihat dan approve/reject permintaan gabung jika mode `approval_required`.
- Audit log untuk register dan approval lifecycle.
- E2E smoke untuk dua mode onboarding.

### Out of scope untuk MVP

- Email verification.
- Captcha / bot protection eksternal.
- Reset password mandiri via email.
- Parent/guardian account.
- Public school creation by siswa.
- Siswa memilih banyak kelas saat daftar.

---

## 4. Recommended Data Model

### 4.1 Setting onboarding

Letakkan setting di level sekolah agar fleksibel antar sekolah.

Opsi A — tambah kolom ke `sekolah`:

```sql
ALTER TABLE sekolah
ADD COLUMN siswa_registration_enabled BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN siswa_registration_mode TEXT NOT NULL DEFAULT 'approval_required';
```

Constraint:

```sql
CHECK (siswa_registration_mode IN ('auto_approve', 'approval_required'))
```

Rationale:
- Simple untuk MVP.
- Admin bisa aktifkan/nonaktifkan self-register per sekolah.
- Mode berlaku untuk semua kelas di sekolah itu.

Opsi B — level kelas:

```sql
ALTER TABLE kelas
ADD COLUMN siswa_registration_enabled BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN siswa_registration_mode TEXT NOT NULL DEFAULT 'approval_required';
```

Rationale:
- Lebih fleksibel, tapi UI/logic lebih kompleks.

Decision recommendation: mulai dari level sekolah dulu. Kalau nanti perlu, override per kelas bisa ditambahkan di fase berikutnya.

### 4.2 Join request

Rekomendasi pakai tabel baru agar enrollment resmi tetap bersih.

```sql
CREATE TABLE siswa_join_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  siswa_id UUID NOT NULL REFERENCES users(id),
  sekolah_id UUID NOT NULL REFERENCES sekolah(id),
  kelas_id UUID NOT NULL REFERENCES kelas(id),
  status TEXT NOT NULL DEFAULT 'pending',
  requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  decided_at TIMESTAMPTZ NULL,
  decided_by UUID NULL REFERENCES users(id),
  reject_reason TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (siswa_id, kelas_id),
  CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled'))
);
```

Behavior:
- `auto_approve`: create user + enrollment active + optional join request `approved` for history.
- `approval_required`: create user + join request `pending`; do not create active enrollment yet.
- On approve: create enrollment active and mark request `approved`.
- On reject: mark request `rejected`; no enrollment.

---

## 5. Backend Plan

### 5.1 Public endpoints

Add unauthenticated endpoints under `/api/v1/auth` or `/api/v1/public`.

```http
GET /api/v1/public/sekolah?registration_enabled=true
GET /api/v1/public/sekolah/:id/kelas
POST /api/v1/auth/register-siswa
```

`GET /public/sekolah` response:

```json
{
  "data": [
    {
      "id": "uuid",
      "nama": "SMK Contoh",
      "registration_mode": "approval_required"
    }
  ]
}
```

`GET /public/sekolah/:id/kelas` response:

```json
{
  "data": [
    {
      "id": "uuid",
      "nama": "X RPL 1"
    }
  ]
}
```

`POST /auth/register-siswa` request:

```json
{
  "nama": "Budi Santoso",
  "username": "budi01",
  "password": "password-minimum",
  "password_confirm": "password-minimum",
  "sekolah_id": "uuid",
  "kelas_id": "uuid"
}
```

Success response for `auto_approve`:

```json
{
  "status": "registered",
  "enrollment_status": "active",
  "message": "Pendaftaran berhasil. Kamu sudah masuk kelas."
}
```

Success response for `approval_required`:

```json
{
  "status": "registered",
  "enrollment_status": "pending",
  "message": "Pendaftaran berhasil. Menunggu persetujuan admin/guru."
}
```

### 5.2 Validation rules

- `nama`: required, 2-100 chars.
- `username`: required, unique, normalized lowercase, allowed chars `[a-z0-9._-]`, 3-32 chars.
- `password`: follow existing password policy.
- `password_confirm`: must match.
- `sekolah_id`: must exist and `siswa_registration_enabled=true`.
- `kelas_id`: must exist, active, and belongs to selected `sekolah_id`.
- Role is hardcoded to `siswa`.
- Force-change-password should be `false` for self-register users unless product decides otherwise.

### 5.3 Security controls

- Reuse global rate limit plus add stricter register limit, e.g. 5 registrations / 15 minutes per IP.
- Never expose inactive schools/classes in public endpoint.
- Never allow client-provided role, status, or admin flags.
- Return generic duplicate username error without leaking more user data.
- Audit all lifecycle actions.

### 5.4 Audit events

Add audit actions:

```text
siswa.self_register
siswa.join_request_created
siswa.join_request_approved
siswa.join_request_rejected
siswa.auto_enrolled
sekolah.registration_setting_updated
```

---

## 6. Admin/Guru UX Plan

### 6.1 Admin sekolah settings

Add controls in admin school detail/edit page:

```text
[ ] Izinkan siswa daftar sendiri
Mode pendaftaran:
  ( ) Langsung masuk kelas
  ( ) Perlu persetujuan admin/guru
```

Copy helper:
- Langsung masuk kelas: cocok untuk lingkungan internal/terkontrol.
- Perlu persetujuan: siswa bisa daftar, tapi akses kelas aktif setelah disetujui.

### 6.2 Request approval UI

Add page/card for pending join requests.

Admin view:
- Can see all pending requests across schools/classes.
- Can approve/reject.
- Filters: sekolah, kelas, status.

Guru view:
- Can see pending requests only for classes they own/teach.
- Can approve/reject for owned classes.

Table columns:

```text
Nama Siswa | Username | Sekolah | Kelas | Tanggal Daftar | Status | Aksi
```

Actions:
- Approve: creates enrollment active.
- Reject: optional reason.

### 6.3 Dashboard indicators

- Admin dashboard: pending join request count.
- Guru dashboard/sidebar: pending join request count for owned classes.

---

## 7. Siswa UX Plan

### 7.1 Public register page

Route candidate:

```text
/register
```

or, if keeping auth namespace:

```text
/auth/register-siswa
```

Recommended: `/register` with clear title `Daftar sebagai Siswa`.

Behavior:
- Load sekolah list on page load.
- Kelas dropdown disabled until sekolah selected.
- On sekolah change, fetch kelas list for that sekolah.
- Show registration mode hint if needed:
  - `Kamu akan langsung masuk kelas setelah daftar.`
  - `Kamu perlu menunggu persetujuan admin/guru setelah daftar.`

### 7.2 Post-register behavior

Option A — redirect to login:
- Simpler and safer.
- Show success message on login page.

Option B — auto-login:
- Smoother UX.
- Requires issuing token immediately after register.

Recommendation MVP: redirect to login first. Auto-login can be added later.

### 7.3 Pending state after login

If user has no active enrollment but has pending request:
- Show siswa dashboard empty state:

```text
Pendaftaran kelas sedang menunggu persetujuan.
Kelas: X RPL 1
Sekolah: SMK Contoh
```

If rejected:
- Show rejected state and allow choosing another class if registration still enabled.

---

## 8. Implementation Steps

### Step 1 — Backend foundation

- Add migration for sekolah registration settings.
- Add migration for `siswa_join_requests`.
- Add repository/service methods for public school/class list.
- Add register siswa service with transaction:
  - validate input
  - create user role siswa
  - branch by registration mode
  - create enrollment or join request
  - write audit log

### Step 2 — Admin setting UI

- Expose setting fields in admin sekolah API.
- Add setting controls in admin sekolah form/detail.
- Add audit event for setting changes.

### Step 3 — Approval workflow

- Add admin/guru endpoints:

```http
GET /api/v1/admin/siswa-join-requests?status=pending&sekolah_id=&kelas_id=
POST /api/v1/admin/siswa-join-requests/:id/approve
POST /api/v1/admin/siswa-join-requests/:id/reject
GET /api/v1/guru/siswa-join-requests?status=pending&kelas_id=
POST /api/v1/guru/siswa-join-requests/:id/approve
POST /api/v1/guru/siswa-join-requests/:id/reject
```

- Admin scope: all schools/classes.
- Guru scope: owned/assigned classes only.
- Approve must be idempotent-safe and transactional.

### Step 4 — Siswa register frontend

- Add public register page.
- Add public API client helpers.
- Add validation and friendly error handling.
- Add link from login page: `Belum punya akun siswa? Daftar di sini`.

### Step 5 — Pending/rejected siswa states

- Update siswa dashboard empty state to detect pending/rejected join request.
- Add API endpoint if needed:

```http
GET /api/v1/siswa/join-requests/mine
```

### Step 6 — Tests and smoke

Backend tests:
- Register disabled school => 400/403.
- Register invalid kelas for sekolah => 400.
- Duplicate username => 409.
- Auto approve creates enrollment.
- Approval required creates pending request only.
- Guru cannot approve request outside owned class.
- Approve creates enrollment exactly once.

Frontend checks:
- Typecheck.
- Register page dropdown flow.
- Admin setting form.
- Approval table actions.

E2E smoke:
- Admin enables `auto_approve` -> siswa registers -> login -> sees class.
- Admin enables `approval_required` -> siswa registers -> login -> pending -> guru approves -> siswa sees class.

---

## 9. API/Error Contract

Use standard error envelope:

```json
{
  "error": "Pesan error untuk user",
  "code": "machine_readable_code",
  "request_id": "uuid"
}
```

Suggested codes:

```text
registration_disabled
invalid_sekolah_id
invalid_kelas_id
kelas_not_in_sekolah
username_taken
password_mismatch
join_request_exists
join_request_not_found
join_request_not_pending
forbidden_kelas_scope
```

---

## 10. Rollout Plan

Recommended rollout:

1. Ship backend + admin setting with default `siswa_registration_enabled=false`.
2. Deploy safely; no behavior change for existing users.
3. Enable registration manually for one test school.
4. Smoke both modes.
5. Add link from login page after smoke passes.
6. Document admin guidance.

Backward compatibility:
- Existing admin-created siswa flow remains unchanged.
- Existing enrollment remains active.
- Public register hidden/disabled unless at least one school enables it.

---

## 11. Open Decisions

1. Setting level: sekolah only for MVP, or kelas-level override from day one?
2. Username only, or support email too?
3. Redirect to login after register, or auto-login?
4. Who can approve: admin only, or admin + assigned guru?
5. Should rejected siswa be able to submit a new request from dashboard?
6. Should admin be able to bulk approve pending requests?

---

## 12. Recommendation

Implement MVP with:
- Setting at `sekolah` level.
- Default `siswa_registration_enabled=false`.
- Default mode `approval_required`.
- Public form: nama, username, password, konfirmasi password, sekolah, kelas.
- Guru and admin can approve, scoped to their authority.
- Redirect to login after successful registration.

This keeps onboarding easy for siswa while preserving admin control over sekolah/kelas access.
