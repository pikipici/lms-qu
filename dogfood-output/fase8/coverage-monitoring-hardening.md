# Fase 8 Coverage + Monitoring Hardening

> Date: 2026-05-23
> Status: Initial hardening scripts added

## Coverage Script

Use `scripts/fase8-coverage.sh` to generate a backend coverage profile and summary:

```bash
bash scripts/fase8-coverage.sh
```

Outputs:

- `dogfood-output/fase8/coverage.out`
- `dogfood-output/fase8/coverage-summary.txt`

Current policy stays pragmatic: coverage is measured and tracked, but strict 70% per-package gating is deferred to v0.14/v0.15. The highest-priority packages for test growth remain auth/admin/soalbab/ujian/nilai/feed.

## Monitoring Check

Use `scripts/fase8-monitoring-check.sh` on the deployed host:

```bash
cd /home/ubuntu/lms
BASE_URL=http://127.0.0.1:8200 SERVICE_NAME=lms-api bash scripts/fase8-monitoring-check.sh
```

Checks:

- `/api/v1/healthz` responds within timeout.
- `/api/v1/readyz` responds within timeout and verifies runtime dependencies.
- `systemctl is-active lms-api` is green when systemd is available.
- `systemctl is-enabled lms-api` warning is surfaced but does not fail the script.

## Recommended Monitoring Next Step

Add a cron/systemd timer or external uptime monitor to run the monitoring check every 1-5 minutes. Alert if either `/readyz` or systemd active check fails twice in a row.

## E2E Link

The new executable E2E hardening step is `frontend/e2e/guru-login.spec.ts`. It expands beyond the auth-only go-live smoke by mocking dashboard APIs and asserting guru dashboard counters/feed render correctly.
