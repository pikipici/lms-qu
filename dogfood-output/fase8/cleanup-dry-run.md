# Fase 8 Cleanup Dry-Run Log

Use this before enabling any destructive cleanup. Do not paste credentials or full `DATABASE_URL` values here.

## Checklist

- Same-day backup exists.
- Latest backup restore drill has passed, or destructive cleanup remains blocked.
- Dry-run command only counts/lists candidates; it does not delete DB rows or R2 objects.
- Candidate counts are reviewed before any code/env flag enables destructive mode.
- Smoke checks pass after the dry-run.

## Entries

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
