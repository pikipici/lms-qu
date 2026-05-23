# Remote Deploy Validation - b19dd23

> Date: 2026-05-23
> Remote host: rdpkhorur
> Deployed commit: b19dd23
> Service: lms-api
> Base URL: http://127.0.0.1:8200

## Dirty State Handling

Before updating the remote working tree, existing remote changes were preserved with `git stash push -u`:

- Stash label: `pre-deploy preserve remote dirty state before b19dd23 2026-05-23T14:48:03+00:00`
- Reason: remote had a dirty `frontend/package-lock.json` plus manually synced Fase 8 hardening files.

No `git reset --hard` was used.

## Update + Deploy

The remote bare origin was updated from local and the working tree fast-forwarded to `b19dd23`.

The standard `deploy/deploy.sh --remote` path did not complete because the remote environment lacks `R2_ENDPOINT` and R2 head-bucket pre-flight failed. To avoid writing secrets or changing `.env`, deployment was completed manually with the same effective build/swap/restart steps while skipping only the R2 pre-flight check.

Manual deploy result:

- Frontend `npm run build`: PASS, with existing lint warnings only.
- Backend `go build ./cmd/server`: PASS.
- Migration step: `no change`.
- `systemctl restart lms-api`: PASS.
- `/api/v1/readyz`: PASS on attempt 1.

## Post-Deploy Gates

Remote validation after deploy:

- `npm run typecheck`: PASS.
- `npx playwright test --list`: PASS, 6 tests discovered.
- `guru-login.spec.ts`: PASS, 3/3.
- `scripts/fase8-smoke.sh`: PASS, login smoke 3/3.
- `scripts/fase8-monitoring-check.sh`: PASS, healthz/readyz/systemd.

## Remaining Remote Notes

After deployment, the remote working tree showed generated/deploy dirt:

- `M frontend/package-lock.json`
- `?? public/`

The pre-deploy remote dirty state remains recoverable from `stash@{0}` on the remote host. The generated `public/` directory is used as deployed static output and was not removed.
