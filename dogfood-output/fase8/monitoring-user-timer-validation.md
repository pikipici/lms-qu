# Fase 8 Monitoring User Timer Validation

> Date: 2026-05-23
> Remote host: rdpkhorur
> Service checked: lms-api
> Base URL: http://127.0.0.1:8200

## Scope

Automated `scripts/fase8-monitoring-check.sh` with a systemd user timer on the remote host.

A system-level timer was attempted first, but privileged `sudo` unit installation was denied by the execution environment, so the safe non-root path was used instead.

## Installed User Units

Remote files created under `/home/ubuntu/.config/systemd/user/`:

- `lms-fase8-monitoring.service`
- `lms-fase8-monitoring.timer`

Timer schedule:

- `OnBootSec=2min`
- `OnUnitActiveSec=5min`
- `RandomizedDelaySec=30s`
- `Persistent=true`

The repository now includes `scripts/install-fase8-monitoring-user-timer.sh` to reproduce this setup.

## Validation

Commands validated on remote:

```bash
systemctl --user is-enabled lms-fase8-monitoring.timer
systemctl --user is-active lms-fase8-monitoring.timer
systemctl --user show lms-fase8-monitoring.service -p Result -p ExecMainStatus --no-pager
systemctl --user list-timers lms-fase8-monitoring.timer --no-pager
journalctl --user -u lms-fase8-monitoring.service -n 30 --no-pager
```

Results:

- Timer enabled: `enabled`
- Timer active: `active`
- Last service result: `Result=success`, `ExecMainStatus=0`
- Journal output showed `healthz`, `readyz`, `systemd service: lms-api`, and `PASS`.

## Operational Notes

- This is a user timer. It runs while the `ubuntu` user manager is active. For boot-persistent operation without login sessions, enable lingering with `loginctl enable-linger ubuntu` from a privileged shell.
- A system timer remains preferable for production alerting if privileged installation is available.
- This timer logs to the user journal; it does not yet send external alerts.
