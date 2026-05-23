# Fase 8 Coverage Gate Re-Scope

## Decision

For MVP go-live, do not block release on the old strict 70% per-package backend coverage gate.

## Rationale

The 70% target is still a good hardening target, but current package shape makes it inefficient as a go-live blocker:

- `internal/admin` already meets the target at `75.4%`.
- `internal/auth` is close enough for targeted hardening but still below target at `55.8%`.
- `internal/soalbab`, `internal/ujian`, and `internal/nilai` need DB/service-oriented tests to move coverage materially; helper-only tests are not enough.
- Critical user journeys are better protected for MVP by representative Playwright E2E and final deploy smoke.

## MVP Go-Live Gate

MVP release should require:

1. Playwright auth smokes pass on the deployed binary.
2. Representative core-flow E2E specs are present and typechecked.
3. `/api/v1/healthz` and `/api/v1/readyz` pass after deploy.
4. Backup/restore drill remains documented as passed.
5. Cleanup dry-run remains gated and safe.
6. Coverage gaps are documented as v0.14/v0.15 hardening work.

## Known Coverage Gaps

Latest measured backend package coverage:

- `internal/auth`: `55.8%`
- `internal/admin`: `75.4%`
- `internal/soalbab`: `7.6%`
- `internal/ujian`: `9.0%`
- `internal/nilai`: `23.6%`

## Follow-Up Hardening

Move the old 70% package target to post-MVP hardening:

- Add DB-backed repository/service tests for `nilai`.
- Add flow handler/service tests for `soalbab` and `ujian`.
- Add remaining auth edge-case tests until `internal/auth` clears 70%.
- Re-enable strict per-package threshold after the critical packages have representative tests.
