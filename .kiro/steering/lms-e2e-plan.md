# LMS — E2E Test Plan (Priority 1)

> Date: 2026-05-23  
> Purpose: Playwright + Go smoke suite for v0.14.0 production release  
> Scope: 10 critical user journeys (must pass for go-live)

---

## 1. Test Infrastructure

### Setup (deploy/deploy.sh already has)
```bash
# Local
bash deploy/deploy.sh

# Remote
ssh rdpkhorur "cd /home/ubuntu/lms && bash deploy/deploy.sh --remote"
```

### Test Commands
```bash
# Local typecheck
npm run typecheck

# Local E2E (uses localhost:8200)
E2E_BASE_URL=http://localhost:8200 npm run e2e

# Remote E2E
cd /home/ubuntu/lms
E2E_BASE_URL=http://127.0.0.1:8200 npm run e2e
```

---

## 2. Test Flows (10 Priority 1)

### 1. Auth: Login → Dashboard Redirect (Role-Based)
**User**: Guru  
**Steps**:
1. Navigate to `/login`
2. Enter valid guru credentials
3. Expect redirect to `/guru` or `/guru/dashboard`
4. Dashboard loads without force-change-password redirect

**Playwright test**: `tests/e2e/auth/guru-login.spec.ts`

---

### 2. Auth: Force-Change-Password Redirect
**User**: Siswa (first login)  
**Steps**:
1. Admin creates user with `MustChangePassword=true`
2. User navigates to `/login`
3. Enters credentials
4. Expect redirect to `/me/security` with success message
5. User changes password
6. Expect redirect back to dashboard

**Playwright test**: `tests/e2e/auth/force-change-password.spec.ts`

---

### 3. Guru: Create Kelas → Add Bab → Upload Materi
**User**: Guru  
**Steps**:
1. Login as guru
2. Navigate to `/guru/kelas`
3. Click "Bikin kelas"
4. Fill form (name, description, invite code)
5. Submit → expect 201 + redirect to kelas detail
6. Add bab (draft → published)
7. Upload materi (PDF, max 20MB)
8. Expect presigned URL generation

**Playwright test**: `tests/e2e/guru/kelas-bab-materi.spec.ts`

---

### 4. Guru: Create Tugas → Siswa Submit → Guru Grade
**User**: Guru + Siswa  
**Steps**:
1. (Guru) Create tugas with deadline
2. (Siswa) Join kelas, navigate to `/siswa/tugas`
3. (Siswa) View tugas detail, submit file
4. (Guru) Navigate to `/guru/tugas`, see submission pending
5. (Guru) Grade submission (score 85)
6. (Siswa) View nilai, expect 85

**Playwright test**: `tests/e2e/guru-siswa/tugas-flow.spec.ts`

---

### 5. Guru: Create Ulangan → Siswa Take → Review Result
**User**: Guru + Siswa  
**Steps**:
1. (Guru) Create ulangan bab (10 soal, 30 menit)
2. (Guru) Publish ulangan
3. (Siswa) Navigate to `/siswa/kelas/detail/ujian`
4. (Siswa) Take ulangan (timer countdown, autosave 600ms)
5. (Siswa) Submit before expire
6. (Siswa) Review result (if enabled)
7. (Guru) View hasil di rekap

**Playwright test**: `tests/e2e/guru-siswa/ulangan-flow.spec.ts`

---

### 6. Siswa: Join Kelas → View Materi → Submit Tugas
**User**: Siswa  
**Steps**:
1. Admin invites siswa via kode
2. Siswa navigates to `/siswa/kelas/join`
3. Enter invite code → join kelas
4. View materi list (published bab only)
5. View tugas list
6. Submit tugas via file upload

**Playwright test**: `tests/e2e/siswa/join-tugas.spec.ts`

---

### 7. Siswa: Kerjain Ulangan (Timer + Autosave + Submit)
**User**: Siswa  
**Steps**:
1. Navigate to ulangan lobby
2. Enter fullscreen
3. Timer countdown visible (30:00 → 00:00)
4. Answer questions (A-E)
5. Autosave every 600ms
6. Submit before expire
7. Result page loads with score

**Playwright test**: `tests/e2e/siswa/ujian-timer.spec.ts`

---

### 8. Admin: Bulk Import Siswa via CSV
**User**: Admin  
**Steps**:
1. Navigate to `/admin/pengguna`
2. Click "Import CSV"
3. Upload CSV (name, email, role=siswa, kelas_id)
4. Preview rows (valid/invalid count)
5. Confirm import
6. Verify users created in DB

**Playwright test**: `tests/e2e/admin/bulk-import.spec.ts`

---

### 9. Admin: Create User + Force Password Change
**User**: Admin  
**Steps**:
1. Navigate to `/admin/users`
2. Click "Tambah pengguna"
3. Fill form (email, name, password)
4. Set `MustChangePassword=true`
5. Submit
6. Verify user created with flag

**Playwright test**: `tests/e2e/admin/create-user.spec.ts`

---

### 10. Cross-Role: Logout → Login as Different Role
**User**: Guru → Siswa  
**Steps**:
1. Login as guru
2. Navigate to `/guru`
3. Logout
4. Login as siswa (different credentials)
5. Expect redirect to `/siswa`
6. Verify role-based routing

**Playwright test**: `tests/e2e/auth/cross-role.spec.ts`

---

## 3. Test Data Setup

### Critical: Test Data Isolation

**Option A: Dedicated test DB (RECOMMENDED)**
- Create separate `lms_e2e_test` database
- Point `E2E_DATABASE_URL` to test DB
- Seed fresh data before each test run
- **Prevents flakiness from concurrent tests**

**Option B: In-memory fixtures (CURRENT PLAN)**
- Use Go test DB mock for backend assertions
- Playwright only validates UI flow
- Acceptance criteria: all tests pass sequentially on clean slate

### Seed Script (Option B)
```bash
cd /home/ubuntu/lms/backend
ADMIN_EMAIL=admin@sekolah.id \
ADMIN_PASSWORD='Test123!' \
./bin/seed-admin
```

### Playwright Fixtures
```typescript
// tests/fixtures/seed.ts
const setup = async ({ page, context }) => {
  // Create admin user
  await page.goto('/admin/users');
  // Create guru user
  // Create siswa user
  // Create kelas, bab, materi
};
```

---

## 4. Smoke Test Suite (Shell-Based)

```bash
#!/bin/bash
# tests/smoke.sh

set -euo pipefail

BASE_URL="${E2E_BASE_URL:-http://127.0.0.1:8200}"

echo "[smoke] Checking healthz..."
curl -fsS "$BASE_URL/api/v1/healthz" || exit 1

echo "[smoke] Checking readyz..."
curl -fsS "$BASE_URL/api/v1/readyz" || exit 1

echo "[smoke] Running 3 auth smokes..."
E2E_BASE_URL="$BASE_URL" npx playwright test tests/e2e/auth/smoke.spec.ts

echo "[smoke] All smoke tests passed!"
```

---

## 5. Known Gaps (Not in Scope for v0.14)

- [ ] Duplicate bab/kelas flow
- [ ] Reset hasil attempt
- [ ] Review jawaban setelah submit
- [ ] Admin audit log view
- [ ] Cross-bab ulangan flow
- [ ] CSV import validation edge cases

---

## 6. Definition of Done

- [ ] All 10 Priority 1 flows pass locally
- [ ] All 10 Priority 1 flows pass on rdpkhorur
- [ ] Smoke suite passes after deploy
- [ ] No flaky tests (deterministic data, no time assertions)
- [ ] E2E results in `dogfood-output/fase8/e2e-report.md`

---

## 7. Next Step

Implement Playwright tests one by one, starting with:
1. `tests/e2e/auth/guru-login.spec.ts` (Flow #1)
2. `tests/e2e/auth/force-change-password.spec.ts` (Flow #2)
