# Fase 8 Cleanup Dry-Run Log

Use this before enabling any destructive cleanup. Do not paste credentials or full `DATABASE_URL` values here.

## Checklist

- Same-day backup exists.
- Latest backup restore drill has passed, or destructive cleanup remains blocked.
- Dry-run command only counts/lists candidates; it does not delete DB rows or R2 objects.
- Candidate counts are reviewed before any code/env flag enables destructive mode.
- Smoke checks pass after the dry-run.

## Entries

### 2026-05-23 09:16 WIB

- Commit SHA: `8a0a475`
- Backup file checked: `NOT_CHECKED`
- Restore drill status: `NOT_RUN`
- `login_attempts_old`: `0`
- `refresh_tokens_expired_revoked`: `0`
- `hasil_soal_bab_deleted_old`: `unavailable: required column missing: deleted_at`
- `hasil_ujian_deleted_old`: `0`
- R2 orphan candidate sample reviewed: `N/A`
- Smoke after dry-run: `PASS` (`deploy/deploy.sh --remote` readyz OK before dry-run)
- Decision: `keep dry-run`
- Notes: `cleanup-dry-run --format=json` executed on rdpkhorur. No destructive cleanup enabled. Binary was built manually after deploy because first run could not find backend/bin/cleanup-dry-run; deploy script already includes the build step and should be rechecked on next deploy.

### YYYY-MM-DD HH:MM WIB

- Commit SHA: `<sha>`
- Backup file checked: `<filename>.sql.gz`
- Restore drill status: `PASS|FAIL|NOT_RUN`
- `login_attempts_old`: `<count>`
- `refresh_tokens_expired_revoked`: `<count>`
- `hasil_soal_bab_deleted_old`: `<count>`
- `hasil_ujian_deleted_old`: `<count>`
- R2 orphan candidate sample reviewed: `YES|NO|N/A`
- Smoke after dry-run: `PASS|FAIL|NOT_RUN`
- Decision: `keep dry-run|enable limited destructive mode|blocked`
- Notes: `<anything unusual, no secrets>`
