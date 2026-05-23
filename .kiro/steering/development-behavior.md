# LMS — Development Behavior & State

> Tracking development state, conventions, and agreements per session.
> Auto-synced with roadmap decisions.

---

## Current Session (2026-05-23)

### Phase: Fase 8 — Polish + E2E / Production Readiness
**Status:** Week 1/3 In Progress

### Session Goals

1. **Deploy script hardening**
   - Pre-flight checks (DB, R2, env vars, port, disk)
   - Build to /tmp, verify executable
   - Backup old binary before replace
   - Post-deploy readyz health check
   - Auto-rollback on failure

2. **Error standardization**
   - Contract: `{error, code, request_id}` for all packages
   - Audit: auth (55.8%), guru, siswa, admin (75.4%)
   - Fix drift: invalid_id → invalid_kelas_id consistency
   - Envelope consistency across all error responses

3. **E2E test plan**
   - 10-15 Priority 1 critical flows
   - Playwright + Go smoke suite
   - Local + remote (rdpkhorur) verification

4. **State tracking**
   - Update roadmap.md decisions
   - Document known gaps for v0.14

### Conventions Enforced

- **Local machine**: pure coding + git only, no runtime deps
- **Remote server (rdpkhorur)**: all build/test/run via SSH
- **Deploy pattern**: follow fb-bot (single binary, systemd, no Nginx)
- **Error handling**: standardized envelope with request_id propagation
- **Testing**: E2E Priority 1 flows, coverage gate deferred to v0.14
- **Branching**: main → push origin → deploy

---

## Previous Session State (v0.13.0 — Fase 7)

- **Closed**: Rekap Nilai + Activity Feed + Pending Counters + Guru Audit
- **Release**: v0.13.0 tag `7abb804`
- **Smoke**: 96/96 green
- **Coverage gate**: deferred (#88 → v0.14)
- **Known drifts**: error key naming (`invalid_id`) deferred to v0.14

---

## Next Session Hook

Review `dogfood-output/fase8/` for:
- coverage-gate.md (MVP scope decision)
- cleanup-dry-run.md (verified safe)
- backup-restore-drill.md (manual, tested)
- e2e-playwright.md (existing skeleton)

All changes via `.kiro/steering/lms-fase8-plan.md` as living doc.
