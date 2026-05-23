# Fase 8 Final Smoke

## Command

```bash
E2E_BASE_URL=http://127.0.0.1:8200 bash scripts/fase8-smoke.sh
```

## Checks

The script validates the deployed binary and static frontend together:

1. `/api/v1/healthz`
2. `/api/v1/readyz`
3. `/login` serves the exported login form (`nama@sekolah.id` present)
4. Frontend `npm run typecheck`
5. Playwright discovery (`npx playwright test --list`)
6. Playwright go-live auth smoke (`login-smoke.spec.ts`)

## Expected State

- Playwright discovery should list `3` tests in `login-smoke.spec.ts`.
- Expanded flow drafts stay as `.draft.ts` and remain excluded from smoke discovery.
