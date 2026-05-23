# Deploy Script Hardening Validation - 203d8b8

> Date: 2026-05-23
> Remote host: rdpkhorur
> Deployed commit: 203d8b8
> Service: lms-api

## Changes Validated

- `deploy/deploy.sh` now derives `R2_ENDPOINT` from `R2_ACCOUNT_ID` when `R2_ENDPOINT` is unset.
- AWS CLI env is derived from existing R2 app env without writing secrets to disk.
- R2 `head-bucket` is warning-only by default because the remote provider/config rejects the pre-flight even though runtime storage config exists. Set `R2_PREFLIGHT_REQUIRED=true` to make it blocking.
- Frontend install changed from `npm install` to `npm ci`.
- Backend builds now run from `backend/` so Go sees `go.mod`.
- Frontend static copy now uses `frontend/out/.` so shell wildcard expansion is not broken by quotes.
- `frontend/package-lock.json` was synced from Linux after `npm install --package-lock-only` so remote `npm ci` succeeds.

## Remote Deploy Result

Standard deploy command succeeded:

```bash
ssh rdpkhorur 'cd /home/ubuntu/lms && bash deploy/deploy.sh --remote'
```

Key result:

- Pre-flight DB: PASS.
- R2 head-bucket: WARNING, non-blocking by design.
- Frontend `npm ci && npm run build`: PASS, existing lint warnings only.
- Backend build: PASS.
- Migration: `no change`.
- Service restart: PASS.
- `/api/v1/readyz`: PASS on attempt 1.

## Post-Deploy Gates

- `npm run typecheck`: PASS.
- Playwright discovery: PASS, 6 tests in 2 files.
- `guru-login.spec.ts`: PASS, 3/3.
- `scripts/fase8-smoke.sh`: PASS, login smoke 3/3.
- `scripts/fase8-monitoring-check.sh`: PASS.

## Remaining Remote Notes

- Remote working tree is clean except `?? public/`, the generated static output directory used by deployment.
- The prior dirty/generated states remain recoverable in remote `git stash` entries.
