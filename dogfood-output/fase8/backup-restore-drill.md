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

### 2026-05-23 10:38 WIB

- Backup file: `lms_manual_2026-05-23_033715.sql.gz`
- Backup size: `180K`
- Restore DB: `lms_restore_drill_20260523033818`
- `gunzip -t`: `PASS`
- Restore: `PASS`
- `users_count`: `7`
- `kelas_count`: `28`
- Cleanup/drop DB: `PASS`
- Notes: First drill attempt created the manual backup successfully but `lms` DB role could not create databases; restore drill rerun with `sudo -u postgres createdb/dropdb -p 5435 -O lms` against disposable DB only. `sudo -u postgres` printed harmless cwd permission warnings because postgres cannot read `/home/ubuntu/lms`.

### 2026-05-25 13:13 WIB

- Backup file: `lms_manual_2026-05-25_061327.sql.gz`
- Backup size: `232482`
- Restore DB: `lms_restore_drill_20260525061328`
- `gunzip -t`: `PASS`
- Restore: `PASS`
- `users_count`: `22`
- `kelas_count`: `36`
- Cleanup/drop DB: `PASS`
- Notes: Disposable restore drill only; live DB untouched. `sudo -u postgres` printed a harmless cwd permission warning once because postgres cannot read the caller cwd.

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
