# Fase 8 Expanded E2E Hardening Backlog

> Date: 2026-05-23
> Status: Backlog for v0.14/v0.15 hardening
> Current MVP gate: `scripts/fase8-smoke.sh` PASS on deployed commit `be73d2a`

## Context

The go-live Playwright gate is intentionally limited to `frontend/e2e/login-smoke.spec.ts` because it is stable against the deployed app and covers the most important auth readiness paths. Expanded flow specs are kept as `.draft.ts` so TypeScript still checks them, but Playwright does not execute them by default.

## Current Draft Specs

| Draft | Purpose | Current State |
|---|---|---|
| `frontend/e2e/core-flows.draft.ts` | Guru/siswa/admin core journeys | Typechecks, not execution-ready |
| `frontend/e2e/force-change-password.draft.ts` | First-login forced password change flow | Typechecks, needs route/API mocks aligned |
| `frontend/e2e/guru-login.draft.ts` | Guru login/dashboard assertions | Typechecks, needs selectors aligned |

## Known Failure Classes

1. Selector drift: draft specs still expect labels/text that do not always match the current UI.
2. Route contract drift: some assertions assume routes or redirects from older UI flows.
3. Mock coverage gaps: login mocks may redirect into protected pages that immediately call unmocked APIs.
4. Data shape drift: API mocks need to match current typed client response envelopes.
5. Parallel/rate-limit risk: login-heavy tests should stay sequential unless per-test isolation is added.

## Promotion Criteria

A draft can be renamed back to `.spec.ts` only after all checks pass locally and on `rdpkhorur`:

```bash
cd frontend
npm run typecheck
npx playwright test --list
E2E_BASE_URL=http://127.0.0.1:8200 npx playwright test <candidate>.spec.ts
```

For remote deployed validation:

```bash
cd /home/ubuntu/lms/frontend
E2E_BASE_URL=http://127.0.0.1:8200 npx playwright test <candidate>.spec.ts
```

## Recommended Order

1. `guru-login.draft.ts` — smallest scope; align selectors and dashboard checks first.
2. `force-change-password.draft.ts` — important auth hardening; mock `/auth/change-password` and `/auth/me` exactly.
3. `core-flows.draft.ts` auth-only slices — split big file into smaller specs before promoting.
4. Guru CRUD journey — mock kelas/bab/materi endpoints, then move to real API smoke later.
5. Siswa task/ulangan journeys — promote only after stable seeded fixtures or full API mocks exist.
6. Admin import/create-user journey — add after CSV/import UI selectors settle.

## Non-Blocker Decision

Expanded E2E is not an MVP go-live blocker. The current blocker remains the final smoke script:

```bash
E2E_BASE_URL=http://127.0.0.1:8200 bash scripts/fase8-smoke.sh
```

Last verified remote result: `[fase8-smoke] PASS` on commit `be73d2a`.
