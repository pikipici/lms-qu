# LMS Fase 8 — Polish + E2E / Production Readiness Plan

> Status: **In Progress** (Week 1/3)  
> Owner: User + Apis  
> Last updated: 2026-05-23  
> Parent: `lms-roadmap.md` (v0.13.0 Fase 7 CLOSED)

---

## 1. Goal

Mengubah LMS dari feature-complete (Fase 0-7) menjadi production-ready MVP dengan:
- Critical user journey E2E test coverage (Playwright)
- Hardened deploy script (pre-flight, health check, rollback)
- Error standardization (FE↔BE contract consistency)
- Final deploy smoke suite

---

## 2. Non-Goals (Explicit)

- **NOT** strict 70% per-package coverage gate → deferred v0.14/v0.15 (see `dogfood-output/fase8/coverage-gate.md`)
- **NOT** new features (no new domains)
- **NOT** performance optimization (out of scope for v0.13.x)

---

## 3. Phase Breakdown

### Week 1 — Foundation (Current)
| # | Task | Scope | Status |
|---|------|-------|--------|
| W1.1 | **Deploy script hardening** | `deploy/deploy.sh` pre-flight + health check + rollback | TODO |
| W1.2 | **Error standardization** | auth / guru / siswa packages error keys + envelope | TODO |
| W1.3 | **E2E test plan** | Draft 10-15 critical flows + skeleton | TODO |
| W1.4 | **development-behavior.md** | Update state tracking + conventions | TODO |

### Week 2 — Execution
| # | Task | Scope | Status |
|---|------|-------|--------|
| W2.1 | **E2E implementation** | Playwright tests: auth, guru CRUD, siswa flow, admin | IN PROGRESS — go-live smoke isolated in `login-smoke.spec.ts`; expanded drafts kept as `.draft.ts` |
| W2.2 | **Coverage re-scope** | Define MVP critical paths, add tests only for those | DONE — strict 70% per-package gate deferred, see `dogfood-output/fase8/coverage-rescope.md` |
| W2.3 | **Smoke test suite** | Shell-based final smoke (healthz, readyz, login export, typecheck, E2E smoke) | DONE — `scripts/fase8-smoke.sh` |

### Week 3 — Production
| # | Task | Scope | Status |
|---|------|-------|--------|
| W3.1 | **v0.14.0 release** | Tag + release notes + deploy | TODO |
| W3.2 | **Monitoring** | systemd status check + basic alerting | TODO |
| W3.3 | **Documentation** | Known gaps documented for v0.15 | TODO |

---

## 4. Technical Details

### 4.1 Deploy Script Hardening (`deploy/deploy.sh`)

**Pre-flight checks (wajib pass sebelum mutate):**
- [ ] `DATABASE_URL` set dan reachable (psql ping 2s timeout)
- [ ] `R2_BUCKET` reachable (aws s3api head-bucket)
- [ ] `.env` loaded (semua required vars exist)
- [ ] Port 8200 free atau owned by current lms-api
- [ ] Disk space ≥ 500MB

**Build phase:**
- [ ] Build BE binary ke `/tmp/lms-api-new`
- [ ] Build FE static ke `/tmp/frontend-new`
- [ ] Verify binary executable (`test -x`)

**Deploy phase:**
- [ ] Stop systemd service
- [ ] Backup old binary: `cp bin/lms-api bin/lms-api.bak.$(date +%s)`
- [ ] Copy new binary + static
- [ ] Migrate up (idempotent)
- [ ] Restart systemd

**Post-deploy verification:**
- [ ] Wait max 30s, poll `/api/v1/readyz` → 200
- [ ] Health check DB + R2
- [ ] Rollback on failure: restore old binary + restart

### 4.2 Error Standardization

**Current drift (known):**
- `invalid_id` vs `invalid_kelas_id` (deferred from 7.F)
- Some packages still return `{error, message}` instead of `{error, code, request_id}`

**Target contract (all packages):**
```json
{
  "error": "human_readable_message",
  "code": "machine_key",
  "request_id": "uuid"
}
```

**Packages to audit & fix:**
1. `internal/auth` — 55.8% coverage, error keys
2. `internal/guru` — scope ownership errors
3. `internal/siswa` — enrollment/404 errors
4. `internal/admin` — 75.4% coverage, minor drift

### 4.3 E2E Critical Flows

**MVP gate update (2026-05-23):** strict 70% per-package coverage is deferred for v0.14/v0.15 hardening. For MVP go-live, require deployed auth smokes + representative Playwright core-flow coverage + final health/ready smoke. See `dogfood-output/fase8/coverage-rescope.md`.

**Priority 1 (Must have for v0.14.0):**
1. Auth: login → dashboard redirect (role-based) — Playwright covered
2. Auth: force-change-password redirect — Playwright covered
3. Guru: create kelas → add bab → upload materi — representative mocked E2E in progress
4. Guru: create tugas → siswa submit → guru grade — representative mocked E2E in progress
5. Guru: create ulangan → siswa take → review result — representative mocked E2E in progress
6. Siswa: join kelas → view materi → submit tugas — representative mocked E2E in progress
7. Siswa: kerjain ulangan (timer + autosave + submit) — representative mocked E2E in progress
8. Admin: bulk import siswa via CSV — planned
9. Admin: create user + force password change — representative mocked E2E covered
10. Cross-role: logout → login as different role — planned

**Priority 2 (Nice to have):**
11. Guru: duplicate bab / kelas
12. Guru: reset hasil attempt
13. Siswa: review jawaban setelah ulangan
14. Admin: audit log view

---

## 5. Risk Register

| Risk | Impact | Mitigation |
|------|--------|------------|
| FE↔BE contract drift | High | Error standardization + static typecheck |
| Deploy failure no rollback | High | Backup binary + auto-rollback on readyz fail |
| E2E flaky in CI | Medium | Deterministic test data, no time-dependent assertions |
| Coverage gate blocks release | Low | Explicitly deferred, documented |
| R2 credential leak in logs | Medium | Redact all secrets in deploy output |

---

## 6. Definition of Done

- [ ] `deploy/deploy.sh` has pre-flight + post-deploy health + rollback
- [ ] All auth/guru/siswa errors use standardized envelope
- [ ] E2E passes 10/10 Priority 1 flows locally + on rdpkhorur
- [ ] Smoke test suite passes after deploy
- [ ] v0.14.0 tagged and deployed
- [ ] Known gaps documented in release notes

---

## 7. Current Next Step

1. Update `development-behavior.md`
2. Harden `deploy/deploy.sh`
3. Audit error standardization
4. Draft E2E test plan

