# Release Readiness - Fase 8 Operational Baseline

> Date: 2026-05-23
> Commit: `3c2d038` (`test: increase tugas coverage`)
> Deployment: `rdpkhorur:/home/ubuntu/lms`
> Status: Ready for optional tag/release decision

## What Is Closed

- Final smoke script added: `scripts/fase8-smoke.sh`.
- Backend static export routing fixed so extensionless routes like `/login` serve `login.html` before falling back to landing `index.html`.
- Playwright default execution made sequential to avoid login rate-limit flakiness on remote smoke runs.
- Stable go-live E2E gate isolated in `frontend/e2e/login-smoke.spec.ts`.
- Expanded E2E specs moved to `.draft.ts` so they stay typechecked but do not block Playwright discovery.
- Auth repository PostgreSQL integration gate added and validated with `AUTH_REPO_TEST_DSN` sourced from remote env without printing secrets.
- Remote deploy path hardened and validated through `deploy/deploy.sh --remote`.
- User-level systemd monitoring timer installed and validated on `rdpkhorur`.
- Generated root `public/` deploy output is ignored so remote git status stays clean.

## Remote Verification

Command:

```bash
cd /home/ubuntu/lms
E2E_BASE_URL=http://127.0.0.1:8200 bash scripts/fase8-smoke.sh
```

Result on latest validated operational baseline:

- Latest local baseline `3c2d038` includes follow-up coverage growth through tugas helpers/services. Remote smoke baseline remains `07429cd` until the next deploy validation.
- `/api/v1/healthz` PASS.
- `/api/v1/readyz` PASS.
- `/login` exported form check PASS.
- `npm run typecheck` PASS.
- Playwright discovery PASS: 6 tests.
- `guru-login.spec.ts` PASS: 3/3.
- `scripts/fase8-smoke.sh` PASS: login smoke 3/3.
- `scripts/fase8-monitoring-check.sh` PASS.
- Monitoring user timer active; last service result `success`, exit status `0`.

## Coverage And Deploy Evidence

- Auth repository integration coverage: `76.5%` when `AUTH_REPO_TEST_DSN` is set.
- Auth regular/default coverage remains about `59.6%` because integration tests skip without DSN.
- Total backend regular coverage is now `29.7%` after low-risk coverage growth commits.
- Standard remote deploy path validated at `203d8b8`; subsequent docs/monitoring/gitignore/coverage commits do not change runtime behavior.
- Evidence files:
  - `dogfood-output/fase8/auth-repo-integration-validation.md`
  - `dogfood-output/fase8/deploy-script-hardening-203d8b8.md`
  - `dogfood-output/fase8/monitoring-user-timer-validation.md`
  - `dogfood-output/fase8/coverage-monitoring-hardening.md`

## Deferred Hardening

- Strict total backend coverage target remains deferred to v0.14/v0.15, though auth clears the 70% target in integration mode and total regular coverage has grown to `29.7%`.
- Expanded E2E flows remain drafts; hardening order is documented in `dogfood-output/fase8/expanded-e2e-hardening-backlog.md`.
- External alerting is still missing; the current user timer logs failures but does not notify.
- Optional privileged persistence remains open: `loginctl enable-linger ubuntu` or system-level timer install.

## Suggested Next Decision

Either deploy/validate `3c2d038` and tag it as the production-readiness baseline, or continue without a tag and push backend regular coverage over the next milestone (`30%+`).
