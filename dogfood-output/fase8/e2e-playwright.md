# Fase 8 Playwright E2E Log

## 2026-05-23 10:00 WIB

- Commit under test: `2d3cba5`
- Target: `rdpkhorur`, `E2E_BASE_URL=http://127.0.0.1:8200`
- Browser install: `npx playwright install chromium` completed on remote.
- Command: `cd /home/ubuntu/lms/frontend && E2E_BASE_URL=http://127.0.0.1:8200 npm run e2e`
- Result: `PASS`
- Tests:
  - `login smoke › shows validation errors before submitting` PASS
  - `login smoke › stores admin session and routes to admin dashboard` PASS
- Notes: Static export serves `/login` via root fallback HTML, so the spec navigates from `/` through the `Masuk` link before interacting with the login form.

## 2026-05-23 10:30 WIB

- Commit under test: `2d6071a`
- Target: `rdpkhorur`, `E2E_BASE_URL=http://127.0.0.1:8200`
- Deploy command: `cd /home/ubuntu/lms && git fetch origin main && git reset --hard origin/main && set -a; . ./.env; set +a; bash deploy/deploy.sh --remote`
- Deploy result: `PASS` (`readyz OK`, migrations `no change`)
- E2E command: `cd /home/ubuntu/lms/frontend && E2E_BASE_URL=http://127.0.0.1:8200 npm run e2e`
- E2E result: `PASS` (`3 passed`)
- Tests:
  - `login smoke › shows validation errors before submitting` PASS
  - `login smoke › stores admin session and routes to admin dashboard` PASS
  - `login smoke › forces temporary-password users to the security page` PASS
- Notes: Adds regression coverage for the locked force-change-password guard: `must_change_password=true` login must redirect to `/me/security` and render the required-change warning/form.
