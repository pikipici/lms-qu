# Fase 8 Go-Live E2E Smoke

## Decision

For MVP go-live, the Playwright gate is the stable auth smoke file only:

```bash
cd /home/ubuntu/lms/frontend
E2E_BASE_URL=http://127.0.0.1:8200 npx playwright test login-smoke.spec.ts
```

## Why

The auth smoke validates the highest-risk production entry points against the deployed Go binary + static export:

- Login form validation works.
- Admin login stores session and routes to `/admin`.
- Temporary-password users route to `/me/security`.

Expanded core-flow specs are useful but not yet stable enough to block go-live because they still need current UI selectors and destination-page API mocks.

## Test Layout

- `frontend/e2e/login-smoke.spec.ts` — go-live smoke gate.
- `frontend/e2e/*.draft.ts` — expanded E2E drafts kept under typecheck, excluded from Playwright discovery because they do not use `.spec.ts`.

## Required Pre-Checks

```bash
cd /home/ubuntu/lms/frontend
npm run typecheck
npx playwright test --list
```

Expected current discovery: `3` tests in `login-smoke.spec.ts`.

## Follow-Up

Before promoting expanded flows back to `.spec.ts`:

1. Add shared login helper.
2. Mock required dashboard/protected API calls per role.
3. Replace brittle text selectors with stable labels or test ids where needed.
4. Re-enable one flow at a time and verify on `rdpkhorur`.
