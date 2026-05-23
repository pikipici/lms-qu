# Auth Repository Integration Validation

> Date: 2026-05-23
> Commit under validation: 3dc1a85
> Remote host: rdpkhorur
> DSN handling: sourced from remote environment and not printed; recorded as `[REDACTED]`.

## Scope

Validated the gated PostgreSQL auth repository integration tests from `backend/internal/auth/repo_integration_test.go` against the remote PostgreSQL database.

The tests create an isolated temporary schema, run the auth migrations, exercise repository methods, and drop the schema after the run.

## Commands

```bash
# File sync only; remote working tree was not reset because frontend/package-lock.json is dirty there.
scp backend/internal/auth/repo_integration_test.go rdpkhorur:/home/ubuntu/lms/backend/internal/auth/repo_integration_test.go

# Run integration tests without printing the DSN.
ssh rdpkhorur 'cd /home/ubuntu/lms/backend && set -a && . ../.env && set +a && AUTH_REPO_TEST_DSN="$DATABASE_URL" go test ./internal/auth -run TestRepoIntegration -count=1 -v'

# Measure auth package coverage with integration tests enabled.
ssh rdpkhorur 'cd /home/ubuntu/lms/backend && set -a && . ../.env && set +a && AUTH_REPO_TEST_DSN="$DATABASE_URL" go test ./internal/auth -coverprofile=/tmp/auth-repo-integration.out -count=1 >/tmp/auth-repo-integration.log && go tool cover -func=/tmp/auth-repo-integration.out | tail -n 1'
```

## Result

Integration tests passed:

- `TestRepoIntegration_UserLifecycleAndListFilters` PASS
- `TestRepoIntegration_RefreshTokenLifecycle` PASS
- `TestRepoIntegration_LoginAttemptsAndAuditFilters` PASS

Auth package coverage with repository integration enabled:

- `auth`: 76.5% statements

This clears the Fase 8 auth package target of 70% when PostgreSQL integration coverage is enabled.

## Notes

- Default local/CI `go test ./...` remains safe because tests skip when `AUTH_REPO_TEST_DSN` is unset.
- Remote repository was still at `be73d2a` with manually synced hardening files; no `git pull`, reset, or deploy was run to avoid touching the remote dirty `frontend/package-lock.json`.
- Next step for production parity is a normal remote deploy/update to the latest pushed commit after deciding how to handle the remote dirty frontend lockfile.
