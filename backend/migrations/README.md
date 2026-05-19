# Backend Migrations

Versioned SQL migrations applied via [`golang-migrate/migrate`](https://github.com/golang-migrate/migrate)
(locked decision #35).

## Naming convention

```
000NNN_<short_name>.up.sql
000NNN_<short_name>.down.sql
```

NNN is monotonic and zero-padded (000001, 000002, ...). Each migration MUST
have a matching down file.

## Run on rdpkhorur

```bash
cd /home/ubuntu/lms/backend
migrate -path ./migrations -database "$DATABASE_URL" up
# rollback latest
migrate -path ./migrations -database "$DATABASE_URL" down 1
```

## Dev shortcut

In dev, set `AUTOMIGRATE=true` in `.env` and the server will use GORM
AutoMigrate at boot — but **only** for fast iteration. Production must use
`migrate up` so schema changes are explicit and reviewable.

## Adding a migration

1. Pick the next sequential number.
2. Write `up.sql` (idempotent where reasonable: `CREATE TABLE IF NOT EXISTS`,
   `ADD COLUMN IF NOT EXISTS`).
3. Write `down.sql` that fully reverses `up.sql`.
4. Test in staging: `migrate up` then `migrate down 1` then `migrate up` again.
5. Commit both files together.
