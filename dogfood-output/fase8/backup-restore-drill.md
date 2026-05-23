# Fase 8 Backup/Restore Drill Log

Use this log for every backup restore drill. Do not paste credentials or full `DATABASE_URL` values here.

## Checklist

- Backup file exists under `/home/ubuntu/lms-backups/daily` or `/home/ubuntu/lms-backups/monthly`.
- `gunzip -t <backup>` exits 0.
- Restore target is a disposable DB named `lms_restore_drill_<timestamp>`, not live `lms`.
- Restore command uses `psql -v ON_ERROR_STOP=1` and exits 0.
- Sanity queries run successfully for critical tables (`users`, `kelas`).
- Disposable DB is dropped after verification.

## Entries

### YYYY-MM-DD HH:MM WIB

- Backup file: `<filename>.sql.gz`
- Backup size: `<size>`
- Restore DB: `lms_restore_drill_<timestamp>`
- `gunzip -t`: `PASS|FAIL`
- Restore: `PASS|FAIL`
- `users_count`: `<count>`
- `kelas_count`: `<count>`
- Cleanup/drop DB: `PASS|FAIL`
- Notes: `<anything unusual, no secrets>`
