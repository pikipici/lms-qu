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
