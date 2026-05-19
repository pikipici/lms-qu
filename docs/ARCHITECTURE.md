# LMS Architecture

> Snapshot Fase 0. Update tiap fase nambah komponen.
> Reference: `.kiro/steering/lms-roadmap.md` v0.7.2.

## Bird's-eye view

```
┌────────────────────────────────────────────────────────────┐
│                       rdpkhorur (Ubuntu, WIB)              │
│                                                            │
│   ┌──────────────────────────────────────────────────┐     │
│   │  systemd: lms-api.service (Type=simple)          │     │
│   │                                                  │     │
│   │  ┌─────────────────────────────────────────┐     │     │
│   │  │  lms-api (Go binary, port 8200)         │     │     │
│   │  │                                         │     │     │
│   │  │  Fiber router                           │     │     │
│   │  │   ├─ /api/v1/healthz   (liveness)       │     │     │
│   │  │   ├─ /api/v1/readyz    (DB+storage)     │     │     │
│   │  │   ├─ /api/v1/auth/*    (Fase 1)         │     │     │
│   │  │   ├─ /api/v1/admin/*   (Fase 1+)        │     │     │
│   │  │   ├─ /api/v1/kelas/*   (Fase 2+)        │     │     │
│   │  │   └─ Static FE (`frontend/out/`) + SPA  │     │     │
│   │  │       fallback                          │     │     │
│   │  └─────────────────────────────────────────┘     │     │
│   └──────────────────────────────────────────────────┘     │
│                                                            │
│   PostgreSQL 15 (localhost) ─── DATABASE_URL               │
│   Local disk: ./storage/uploads/<kategori>/<uuid>.<ext>    │
│   Backups:    /home/ubuntu/backups/lms_*.sql.gz            │
└────────────────────────────────────────────────────────────┘
              ▲
              │ ssh -L 8200:127.0.0.1:8200 rdpkhorur
              │
       Laptop browser → http://localhost:8200
```

Tidak ada Nginx (#9). Single binary serve API + static FE. JWT di localStorage,
same-origin → no CSRF risk klasik (#5 open decision still tracked).

## Process model

| Komponen | Process | Run-as | Notes |
|---|---|---|---|
| API + static | systemd `lms-api.service` | `ubuntu` | Restart=always, ExecStartPost cek `/readyz` |
| DB | systemd `postgresql.service` | `postgres` | Lokal, listen 127.0.0.1 |
| Backup | cron daily | `ubuntu` | `pg_dump | gzip > backups/` |
| Cleanup | cron daily (Fase 8) | `ubuntu` | Orphan files, expired tokens, retention |
| Migrations | manual (`migrate up`) | `ubuntu` | Triggered di `deploy.sh` step 4 |

## Auth flow (Fase 1 target)

```
Client                         lms-api                       PostgreSQL
  │                              │                              │
  │ POST /auth/login             │                              │
  │ {email,password}             │                              │
  ├─────────────────────────────►│  bcrypt verify               │
  │                              │  rate-limit check (5/15m)    │
  │                              ├─────────────────────────────►│
  │                              │  insert RefreshToken (jti)   │
  │                              ├─────────────────────────────►│
  │  {access(15m), refresh(7d),  │                              │
  │   user, must_change_password}│                              │
  │◄─────────────────────────────┤                              │
  │                              │                              │
  │ Authorization: Bearer ACCESS │                              │
  ├─────────────────────────────►│  JWT verify (HS256)          │
  │                              │  middleware order:           │
  │                              │   ratelimit → request-id →   │
  │                              │   auth → role-guard →        │
  │                              │   enrollment-guard           │
  │                              │                              │
  │  ... 401 expired ...         │                              │
  │ POST /auth/refresh           │                              │
  ├─────────────────────────────►│  rotate jti (revoke old +    │
  │                              │  issue new)                  │
  │                              ├─────────────────────────────►│
  │  {access, refresh}           │                              │
  │◄─────────────────────────────┤                              │
```

Reuse detection (#42): kalau old jti dipake setelah revoked → revoke
seluruh chain user + audit log `reuse_detected`.

## Storage convention (#58)

```
storage/uploads/
├── tugas/      <uuid>.pdf  <uuid>.docx  <uuid>.zip ...
├── soal/       <uuid>.jpg  <uuid>.png  <uuid>.webp ...
├── materi/     <uuid>.pdf  <uuid>.md ...
├── submission/ <uuid>.pdf ...
└── import/     <uuid>.csv  (auto-cleanup 1h)
```

Original filename disimpan di kolom DB terpisah; on-disk hanya UUID untuk
sanitization + cleanup-friendly.

## Concurrency model

| Skenario | Pattern | Locked |
|---|---|---|
| Submit ulangan (race) | `SELECT FOR UPDATE` + cek status di transaction, idempotent | #43 |
| Edit konten (Bab/Soal/Kelas) | Optimistic version (`version` column), 409 on mismatch | #56 |
| Timer expire job | `pg_advisory_xact_lock(hasil_id)` | #43 |
| 1 attempt aktif per ulangan | Partial unique index `WHERE deleted_at IS NULL` | #11 risk |

## Observability

- **Logs**: structured slog (text di dev, JSON di prod) ke stdout → journald.
- **Request-ID**: `X-Request-ID` di setiap req/res, propagate ke slog.
- **Error response**: `{ error, code, request_id }`.
- **Health**: `/healthz` (liveness) + `/readyz` (DB + storage writable).
- **Metrics**: deferred ke v0.8+ (kalau perlu Prometheus).

## Phase progression

Fase yang aktif sekarang highlighted; lihat `lms-roadmap.md` Section 10 untuk
breakdown detail per fase.

```
[x] 0  Setup            ← sekarang
[ ] 1  Auth & Admin
[ ] 2  Kelas + Bulk Import
[ ] 3  Bab + Materi + Pengumuman
[ ] 4  Tugas + Late + Resubmit
[ ] 5  Soal Bab (Latihan + Ulangan + Resume + Remedial + Random + Review)
[ ] 6  Ulangan Harian (mirror Fase 5)
[ ] 7  Rekap Nilai + Activity Feed + Pending Counters
[ ] 8  Polish + E2E + Cleanup tasks + Coverage gate
```

Notifikasi (v0.8) dibedah sebelum Fase 4.
