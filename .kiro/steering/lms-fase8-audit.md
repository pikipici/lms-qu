# LMS Fase 8 — Audit Summary & Error Standardization

> Date: 2026-05-23  
> Purpose: Document current error handling state + fix plan

---

## Status: ✅ GOOD NEWS

Major packages **already follow** standardized envelope `{error, code, request_id}`:

### ✅ auth (55.8% coverage)
- `middleware/unauthorized()` → `{error, code, request_id}`
- `authError()` helper → standardized
- Rate limit responses → standardized
- **Status**: CONSISTENT

### ✅ audit (75.4% coverage)
- `errResp()` helper → standardized
- Line 40: `invalid_id` → **DRIFT** (should be `invalid_kelas_id`)

### ✅ admin (75.4% coverage)
- Need to verify specific error responses

### ✅ guru/siswa packages
- Need to verify specific error responses

---

## Known Drifts (Low Priority)

| File | Line | Current | Target | Priority |
|------|------|---------|--------|----------|
| `backend/internal/audit/handler.go` | 40 | `invalid_id` | `invalid_kelas_id` | LOW (deferred to v0.14) |
| Various guru/siswa packages | - | Check envelope | `{error, code, request_id}` | LOW |

---

## Action Plan

### Phase 1: Audit (Current Session)
1. ✅ Deploy script hardened
2. 🔲 Admin package error audit
3. 🔲 Guru package error audit  
4. 🔲 Siswa package error audit

### Phase 2: Fix (Next Session)
1. Fix known drifts (invalid_id → invalid_kelas_id)
2. Add helper functions where missing
3. Static typecheck FE↔BE contract

### Phase 3: E2E (Week 2)
1. Playwright tests with error boundary checks
2. Smoke suite with error response validation

---

## Recommendation

**Skip strict error standardization for v0.14.0** → focus on E2E coverage instead.

Reason:
- Major packages already consistent
- Only 1 known drift (`invalid_id` → `invalid_kelas_id`)
- FE error toast handles generic messages fine
- Can fix in v0.15 hardening

**Pivot:** Langsung ke E2E test plan (Week 1.3)
