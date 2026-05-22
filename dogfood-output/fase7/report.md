# Fase 7 UX/QA Audit Report

> Method: static FE↔BE contract drift audit (browser unavailable di local
> Windows; HTTP smoke covered di `/tmp/qa-7{a..e}.sh` 7.A 16/16 + 7.B 17/17
> + 7.C 16/16 + 7.D 11/11 + 7.E 20/20 hijau). Live curl probe via SSH
> rdpkhorur untuk validasi numeric edge case.
>
> Scope: Fase 7 Task A-E (siswa nilai + guru rekap + activity feed +
> pending counters consolidated + audit log). Browser visual QA TIDAK
> covered — defer ke real dogfood saat user lihat live.

## Executive summary

Total findings: **5** (0 Critical, 2 High, 2 Medium, 1 Low).

Severity counts:
- Critical: 0 (no SQLSTATE 500, no auth bypass, no broken happy path)
- High: 2 (silent invalid input fallback di feed endpoint)
- Medium: 2 (error key naming convention drift)
- Low: 1 (TanStack refetch-in-background config drift)

Highlights:
- 3-layer numeric audit clean — no DB CHECK violation di Fase 7 endpoints
  (feed limit clamp [1,50] di service-side, audit limit clamp [1,100] di
  handler+service, pending no numeric input)
- Enum/shape audit clean — `feed.EventKind` (3 values) + `audit.AllowedActions`
  (44 entries) + `nilai.Rekap*` shape semua match BE↔FE
- Smoke pass tapi static audit reveal silent-input class yang test current
  ga catch (limit=abc)

## Contract surface table

| Feature | Endpoint | Numeric bounds | Enum/keys | Error code shape |
|---|---|---|---|---|
| 7.A siswa nilai | GET /siswa/nilai, /siswa/kelas/:id/nilai | none | `Rekap*` shape match | `invalid_id`/`not_found`/`forbidden`/`internal` |
| 7.B guru rekap | GET /kelas/:id/rekap?format=json\|csv | none (kelas-scoped 10K cap di code) | `Rekap*` shape match, format enum 2 vals | `invalid_id`/`not_found`/`forbidden`/`internal` |
| 7.C activity feed | GET /guru/feed?cursor=&limit= | limit clamp [1,50] di service | `EventKind` 3 vals match FE | `forbidden`/`invalid_cursor`/`internal` |
| 7.D pending | GET /guru/pending-counts | none | shape 3 fields match FE | `forbidden`/`internal` |
| 7.E audit | GET /guru/kelas/:id/audit?action=&limit=&offset=, /guru/audit-actions | limit clamp [1,100] handler+service, offset≥0 | 44 actions match FE labels | `invalid_kelas_id`/`kelas_not_found`/`invalid_action`/`invalid_limit`/`invalid_offset`/`forbidden`/`unauthorized`/`internal_error` |

## Findings

### Finding 1 — Feed endpoint silently swallows invalid `limit` param [Severity: High | Category: Functional]

- File: `backend/internal/feed/handler.go:37-42`
- BE behavior:
  ```go
  limit := 0
  if v := strings.TrimSpace(c.Query("limit")); v != "" {
      if n, perr := strconv.Atoi(v); perr == nil {
          limit = n  // parse error silently dropped → limit stays 0 → service applies default 20
      }
  }
  ```
  - `?limit=abc` → HTTP 200 with default 20 events
  - `?limit=-5` → HTTP 200 with 20 events (negative also dropped via `if limit <= 0` clamp)
  - `?limit=0` → HTTP 200 with 20 events (default applied)
- Probe evidence (rdpkhorur live):
  ```
  GET /guru/feed?limit=abc → HTTP 200 (expected 400 invalid_limit)
  GET /guru/feed?limit=-5  → HTTP 200 (expected 400)
  ```
- Impact: FE sending malformed limit gets silent default behavior — bug
  detection hard. Inconsistent dgn audit endpoint same-feature which
  returns 400 for `?limit=abc` and `?limit=0`/`?limit=-1`.
- Fix size: ~6 LOC — match audit handler pattern: parse fail → `errResp
  StatusBadRequest "invalid_limit"`, also reject `limit < 1`.

### Finding 2 — Feed endpoint accepts negative `limit` without 400 [Severity: High | Category: Functional]

- File: `backend/internal/feed/handler.go:37-42` + `backend/internal/feed/service.go:237-241`
- BE behavior: handler trusts negative int through, service applies
  `if limit <= 0 → defaultLimit`. No 400. Compare audit:
  ```go
  // audit/handler.go:54
  if err != nil || n < 1 {
      return errResp(c, fiber.StatusBadRequest, "invalid limit", "invalid_limit")
  }
  ```
- Impact: same semantic class as Finding 1 — hidden default fallback
  without user feedback.
- Fix size: bundled with Finding 1 (single patch). Recommended pattern:
  reject `< 1` di handler, return 400 dgn `invalid_limit`.

### Finding 3 — Inconsistent error code naming convention across Fase 7 endpoints [Severity: Medium | Category: API contract]

- Files:
  - `backend/internal/nilai/handler.go` — emits `invalid_id`, `not_found`, `forbidden`, `internal`
  - `backend/internal/audit/handler.go` — emits `invalid_kelas_id`, `kelas_not_found`, `forbidden`, `internal_error`
  - `backend/internal/submission/handler.go:303` (PendingHandler.Count) — emits `internal`
  - `backend/internal/feed/handler.go` — emits `invalid_cursor`, `forbidden`, `internal`
- BE evidence:
  ```
  GET /kelas/not-uuid/rekap     → {"code":"invalid_id"}
  GET /kelas/not-uuid/audit     → {"error":"invalid_kelas_id"}   (different shape too — audit lacks request_id key in error envelope, see below)
  ```
- FE consume: all FE error handlers (`audit-api.ts`, `nilai-api.ts`,
  `feed-api.ts`, `guru-api.ts`) currently only check `err.status` (HTTP
  code), not `err.code`/`err.error`. So drift TIDAK break behavior.
- Impact: log-grep / future error mapper FE arms / observability
  inconsistent. Pattern mismatch makes it hard to write generic error
  toast.
- Fix size: ~10 LOC across 2 files. Recommendation: standardize to
  audit-style specific keys (`invalid_kelas_id`, `kelas_not_found`,
  `internal_error`) since they're more grep-able. Backward compat: low
  risk because no FE consumer reads them.

### Finding 4 — Error response envelope shape drift between handlers [Severity: Medium | Category: API contract]

- Files:
  - `backend/internal/audit/handler.go:26-30` — emits `{error, message}`
  - `backend/internal/feed/handler.go:58-63` — emits `{error, code, request_id}` (includes request_id from middleware)
  - `backend/internal/nilai/handler.go` (probed) — emits `{code, error, request_id}`
- Probe evidence:
  ```
  GET /kelas/not-uuid/audit       → {"error":"invalid_kelas_id","message":"invalid kelas id"}
  GET /kelas/not-uuid/rekap       → {"code":"invalid_id","error":"invalid kelas id","request_id":"..."}
  GET /guru/feed?cursor=garbage   → {"error":"...","code":"invalid_cursor","request_id":"..."}
  ```
- Impact: audit response missing `request_id` (debugging harder — guru
  nge-report bug ga bisa kasih request_id) AND key naming `error` vs
  `code` mixed up. Shape drift = FE generic error parser break.
- Fix size: ~5 LOC — align audit `errResp` to use
  `{code, error, request_id}` matching project default helper. Inline
  the request_id from `RequestIDFromFiber(c)` like other handlers.

### Finding 5 — Pending counter polling: dashboard query missing `refetchIntervalInBackground: false` [Severity: Low | Category: UX]

- File: `frontend/app/(authed)/guru/page.tsx:44-49`
- FE code:
  ```tsx
  const pendingQ = useQuery({
    queryKey: ['guru', 'pending-counts'],
    queryFn: getPendingCounts,
    staleTime: 15_000,
    refetchInterval: 30_000,
    // ← missing refetchIntervalInBackground: false
  });
  ```
- Compare layout.tsx:71-77 (deduped query with same queryKey) — has
  `refetchIntervalInBackground: false`. TanStack ngambil config FROM the
  first registered query for that key, jadi efek fungsional di-dedupe
  oleh layout config saat ini. Tapi kalau urutan mount berubah, idle tab
  jadi polling.
- Impact: defensive consistency. No user-visible bug today.
- Fix size: 1 LOC — add `refetchIntervalInBackground: false` di
  `guru/page.tsx`.

## Recommended fixes (ordered by severity)

**Quick wins (1-batch fix):**
1. Finding 1+2 — feed handler limit validation (~6 LOC). Match audit
   pattern: reject `parse fail || < 1` dgn 400. Will need smoke regen
   `/tmp/qa-7c.sh` to assert `limit=abc → 400` and `limit=-5 → 400`.
2. Finding 5 — guru/page.tsx 1 LOC defensive add.

**Cleanup batch:**
3. Finding 3 — error code naming standardization (~10 LOC across nilai,
   submission/pending). Standardize to specific keys
   (`invalid_kelas_id`/`kelas_not_found`/`internal_error`).
4. Finding 4 — error response envelope alignment (~5 LOC) — audit
   handler add `request_id` + use `code` instead of `error` for the key.

**Decisions needed:** none. All fixes mechanical.

## Locked decisions cross-check

- ✅ Locked #55 (activity feed cursor): opaque base64 `(at_unix_micro
  DESC, id DESC)`, polling pakai cursor=null untuk slice latest. Verified
  di feed/service.go:80 `defaultLimit=20, maxLimit=50`.
- ✅ Locked #59 (guru audit scope): hard scope `WHERE target_kelas_id=:id`,
  ownership guard. Verified di audit/service.go ListByKelas.
- ✅ Locked #90 (read-only at-query-time aggregator, no `nilai_*`
  tables): rekap.go pakai single-pass JOIN ke HasilSoalBab+Submission+
  HasilUjian. No new table.
- ✅ Locked #91 (FE routing): `/siswa/nilai`, `/siswa/kelas/[id]/nilai`,
  `/guru/kelas/[id]/rekap-nilai`, `/guru/kelas/[id]/audit` semua routed
  pakai static-export query-string `?id=` pattern.
- ✅ Locked #92 (activity feed source): 3 source UNION ALL —
  submission_baru/ulangan_selesai/siswa_join. Note: roadmap entry
  mentioned 6 event types (submission_submitted, submission_graded,
  siswa_joined, ulangan_bab_finished, ujian_finished, hasil_reset) tapi
  implementation MVP cuma 3 (submission/ulangan_selesai/siswa_join).
  Open: future enhancement?
- ✅ Locked #93 (pending counters consolidated): single endpoint, 3
  fields, polling 30s. Verified di submission/pending.go.
- ⚠️ Roadmap claim 6 event types vs implementation 3 — not a finding
  (MVP scope OK), but flag untuk 7.G release notes.

## Testing notes

- Static audit only. No browser visual QA performed (Windows local
  blocker). Real dogfood (UI flow, empty state, mobile) defer ke user
  saat live.
- Numeric 3-layer audit: no DB CHECK constraint relevant in Fase 7
  endpoints (no INSERT/UPDATE with numeric input — read-only +
  pagination params only).
- Live HTTP smoke previously: 7.A 16/16, 7.B 17/17, 7.C 16/16, 7.D 11/11,
  7.E 20/20 — all PASS. This audit is additive layer.
- Audit suggests adding regression case `T-FEED-LIMIT-INVALID`:
  ```bash
  curl ... "/guru/feed?limit=abc"  # expect 400
  curl ... "/guru/feed?limit=-1"   # expect 400
  curl ... "/guru/feed?limit=0"    # expect 400
  ```
  to match existing `T-AUDIT-LIMIT` smoke.
