# Fase 6 — Ulangan Harian (Ujian) UX/QA Dogfood Report

**Date:** 2026-05-22
**Method:** Static-audit + HTTP/API smoke verification (no live browser visual QA in this pass; CLI environment).
**Audit skill applied:** `dogfood` + `fe-be-contract-drift-audit`.
**Scope:** Fase 6 BE+FE end-to-end — `backend/internal/banksoal/`, `backend/internal/ujian/`, `frontend/lib/banksoal-api.ts`, `frontend/lib/ujian-api.ts`, `frontend/lib/siswa-ujian-api.ts`, `frontend/components/banksoal/*`, `frontend/components/ujian/*`, `frontend/components/siswa-ujian/*`, plus migration `000011_ujian.up.sql`.

## Executive summary

| Severity   | Count | Notes |
|------------|------:|-------|
| Critical   |   1   | DB CHECK constraint vs Go validator drift → user-visible HTTP 500 on durasi 301-600. |
| High       |   1   | FE form `durasi_menit` max=360 mismatches DB CHECK cap of 300 (same shape as Fase 5 fix `22d2095`). |
| Medium     |   1   | FE banksoal-api maps error codes BE never emits + misses codes BE does emit. |
| Low        |   2   | FE siswa-ujian-api has redundant `'timer_expired'` arm (alias path); FE `friendlyUjianError` declares `'ujian_not_started'` arm but BE emits `ujian_window_not_open`. |
| **Total**  | **5** | All 9/9 HTTP smoke probes verified the bounds. |

All findings are static-confirmed and (where applicable) reproduced via `curl`. The Fase 6 BE happy-path smoke (416/416 from closure) is unaffected — these are edge-case + non-happy-path drift.

## Contract surface (key fields)

| Field                       | DB constraint                         | Go validator (service.go)             | FE form / type                                 | Drift?  |
|-----------------------------|---------------------------------------|---------------------------------------|------------------------------------------------|---------|
| `ujian.durasi_menit`        | `CHECK BETWEEN 1 AND 300` (000011)    | `1..600` (`MaxDurasiMenit=600`)       | `min=1 max=360`                                | YES — 3-way drift |
| `ujian.source.jumlah_soal`  | (no DB cap)                           | `1..200` (`MaxJumlahSoal=200`)        | `min=1 max=200`                                | OK      |
| `bank_soal.poin`            | (no DB cap)                           | `1..100` (validateSoalFields)         | `min=1 max=100`                                | OK      |
| `bank_soal.pertanyaan` size | TEXT (no cap)                         | `MaxPertanyaanBytes = 5*1024`         | no FE maxLength                                | OK (BE-enforced) |
| `bank_soal.opsi_*` size     | TEXT (no cap)                         | `MaxOpsiBytes = 2*1024`               | no FE maxLength                                | OK (BE-enforced) |
| BankSoalImagePresignTTL     | n/a                                   | `15 * time.Minute`                    | items refetch every 12m (UjianPlayer)          | OK (gated, mirrors Fase 5 fix) |
| Ujian status enum           | `draft|published|archived`            | matches model.Valid()                 | `'draft' | 'published' | 'archived'`           | OK      |
| HasilUjian status enum      | `berlangsung|selesai|dibatalkan`      | matches model.Valid()                 | `'berlangsung' | 'selesai' | 'dibatalkan'`     | OK (unlike Fase 5 pre-fix) |
| Ujian source mode           | jsonb discriminated                   | `manual | random`                     | `'manual' | 'random'`                          | OK      |

## Findings

### 1 — [CRITICAL] DB CHECK durasi_menit cap=300 vs Go cap=600 → HTTP 500 on 301-600

- **File (DB):** `backend/migrations/000011_ujian.up.sql:86`
  ```sql
  durasi_menit SMALLINT NOT NULL DEFAULT 60 CHECK (durasi_menit BETWEEN 1 AND 300)
  ```
- **File (Go):** `backend/internal/ujian/service.go:56-59`
  ```go
  MaxJumlahSoal     = 200
  MinJumlahSoal     = 1
  MinDurasiMenit    = 1
  MaxDurasiMenit    = 600 // 10 jam max
  ```
- **Repro (live, 2026-05-22):**
  ```text
  POST /api/v1/kelas/<K>/ujian {"durasi_menit":301,...}  → 500
  POST /api/v1/kelas/<K>/ujian {"durasi_menit":361,...}  → 500
  ```
  Server log:
  ```
  ERROR: new row for relation "ujian" violates check constraint
  "ujian_durasi_menit_check" (SQLSTATE 23514)
  ```
- **Why it's Critical:** any client (FE, mobile, automation, or human via Postman) hitting durasi 301-600 gets a generic 500 instead of a structured 400 `invalid_body`. Also reveals an internal consistency bug — the service comment "10 jam max" is contradicted by the DB.
- **Fix options (LOC ≈ 1):**
  - **Option A (preferred): align Go cap to DB.** `MaxDurasiMenit = 300`. Matches Fase 5 SoalBab pattern (`UlanganBabSettingForm` durasi cap was 300 since 5.G UX pass `22d2095`). Comment update: `// 5 jam max (matches CHECK constraint)`.
  - **Option B: bump DB CHECK to 600.** Requires migration 000012 + roadmap doc. Heavier change, only justified if guru actually wants 5-10h ujian (unlikely; SoalBab cap is also 300).

  Going with **Option A** — keeps schema as the source of truth and stops leaking 500s.

### 2 — [HIGH] FE durasi_menit form max=360 mismatches DB cap=300

- **File:** `frontend/components/ujian/UjianFormDialog.tsx:142, 309`
  ```tsx
  if (form.durasi_menit < 1 || form.durasi_menit > 360) { ... }
  <Input min={1} max={360} ... />
  ```
- **Symptom:** guru fills 301-360 in the form, FE accepts (no client error), submit hits BE Go validator (which currently passes due to Finding #1), then DB rejects with 500. After Finding #1 fix, the FE 301-360 case will resolve to 400 `invalid_body` from BE — still a wasted round-trip + cryptic error vs catching client-side.
- **Fix (LOC ≈ 2):** swap 360 → 300 (both validation block and `<Input max={...}>`). Update form-level error message: `'Durasi 1-300 menit.'`.
- **Note:** this is the exact same drift shape as Fase 5 `22d2095` but caught live this time. Locked decision #88 coverage gate (defer Fase 8) won't catch this because it's a UI form bound, not a Go test concern.

### 3 — [MEDIUM] FE banksoal-api error map drift — declares codes BE never emits + missing codes BE does emit

- **File:** `frontend/lib/banksoal-api.ts:296-360`
- **BE-emitted set** (from `grep errResp .. backend/internal/banksoal/*.go`):
  ```text
  forbidden, image_decode_failed, image_encode_failed, image_slot_empty,
  image_upload_failed, internal, invalid_body, invalid_id, invalid_limit,
  invalid_offset, invalid_slot, invalid_version, jawaban_invalid,
  missing_file, not_found, open_failed, payload_too_large, read_failed,
  rows_required, unsupported_mime, version_conflict
  ```
- **FE-handled set:**
  ```text
  invalid_id, invalid_body, invalid_input, jawaban_invalid, invalid_version,
  version_conflict, forbidden, not_found, image_too_large, image_invalid_type,
  image_decode_failed, invalid_slot, too_many, rows_required, invalid_limit,
  invalid_offset
  ```
- **Drift (FE-declared but BE never emits):** `image_too_large`, `image_invalid_type`, `too_many`, `invalid_input`. Verified live with curl:
  ```bash
  POST /api/v1/bank-soal/<id>/image?slot=pertanyaan (>5MB file)
  → 413 {"code":"payload_too_large", ...}
  ```
  The `image_too_large` arm is dead. (`too_many` in handler reuses `bulk_handler.go:78` — actually emitted there, but it's via `bulk-paste` which lives in same module — so this one is **valid**, not dead. Keep `too_many`.) `image_invalid_type` → never grep-matched; BE emits `unsupported_mime` for that case. `invalid_input` → not emitted by banksoal handlers (uses `invalid_body`).
- **Drift (BE emits but FE has no arm):** `payload_too_large`, `unsupported_mime`, `image_slot_empty`, `image_encode_failed`, `image_upload_failed`, `r2_unavailable` (handler emits this too — `image_handler.go:183`), `missing_file`, `open_failed`, `read_failed`. Without arms these all fall to the `default:` branch → user sees raw `err.message` only.
- **Fix (LOC ≈ 12):** drop dead `image_too_large` + `image_invalid_type` + `invalid_input` arms. Add arms for the four user-facing image error codes (`payload_too_large`, `unsupported_mime`, `image_slot_empty`, `r2_unavailable`). Skip the internal `*_failed` codes (default fallback already friendly enough).

### 4 — [LOW] FE siswa-ujian-api `friendlyUjianError` has duplicate timer-expired arm

- **File:** `frontend/lib/siswa-ujian-api.ts:315-317`
  ```ts
  case 'ujian_timer_expired':
  case 'timer_expired':
    return 'Waktu ujian sudah habis. Refresh untuk lihat hasil.';
  ```
- **Verified BE emits:**
  - `ujian_timer_expired` — from `flow_handler.go:167` (handler 410).
  - `timer_expired` — from `timer_cron.go:234` (audit ledger meta only, not as an HTTP code).
- The cron meta-ledger field is a JSON metadata key, not an HTTP error code. The FE arm `'timer_expired'` is therefore unreachable as an HTTP error response from the ujian flow — the only HTTP path uses `ujian_timer_expired`.
- Source of confusion: SoalBab Fase 5 emits `'timer_expired'` directly from `ulangan_handler.go:131`. Likely the FE author copy-pasted both arms defensively while writing siswa-ujian-api. Harmless (both map to same string), but it's dead code that lies about BE behavior.
- **Fix (LOC ≈ 1):** drop the `'timer_expired'` arm (keep only `'ujian_timer_expired'`).

### 5 — [LOW] FE `friendlyUjianError` arm `'ujian_not_started'` orphan; missing `ujian_window_not_open`

- **File (FE):** `frontend/lib/siswa-ujian-api.ts:295-296`
  ```ts
  case 'ujian_not_started':
    return 'Ujian belum dibuka. Tunggu sampai waktu mulai tiba.';
  ```
- **BE truth:** `flow_handler.go:171` emits `ujian_window_not_open` (StatusConflict 409). No code path emits `ujian_not_started` anywhere in `backend/internal/ujian/`.
- **Symptom:** when siswa hits POST `/start` before `waktu_mulai`, FE gets `'ujian_window_not_open'` and falls through to `default:` → bilingual `err.message` "ujian belum dimulai sesuai jadwal" (still readable, but doesn't match the polished arm copy). The polished arm with key `'ujian_not_started'` is unreachable.
- **Fix (LOC ≈ 1):** rename arm `'ujian_not_started'` → `'ujian_window_not_open'`. Keep the friendly Indonesian copy.

## Locked decisions cross-check

| Locked | Decision                                                | Verified |
|--------|---------------------------------------------------------|----------|
| #76    | Anti-cheat: items strip `jawaban_benar`, answer no `is_benar` | OK (Fase 6 closure smoke 6.D.4 + 6.G.2) |
| #81    | Review gating: `/review` enforces `izinkan_review_setelah_submit` + `waktu_buka_review` | OK (FE `canViewReview` + BE handler) |
| #84    | Bank Soal scope per-guru pribadi                        | OK (`canManageSoal` + tests) |
| #85    | Source mode discriminated (manual/random)               | OK (validator + jsonb `mode` + `Valid()`) |
| #86    | Random pool deterministic seed `sha256(mulai_unix_micro || siswa_id || ujian_id)` | OK (start.go:247 + smoke `qa-6d1.sh` resume same ids) |
| #87    | Cron 30s + advisory lock auto-grade                     | OK (`timer_cron.go` + reuse soalbab goroutine) |
| #88    | Backend coverage gate ≥70% (defer Fase 8)               | DEFERRED (mirror Fase 5 #82 soft fallback) |
| #56    | Optimistic concurrency `version`                        | OK (Update/Delete require expectedVersion) |

No locked-decision violations.

## HTTP smoke regression — boundary checks (`/tmp/qa-fase6-bounds.sh`)

Run live against rdpkhorur:8200 with guru1/guru1pass fixture:

```text
PASS  T1a Create durasi=300 (DB CHECK cap = accept)                       got=201
PASS  T1b Create durasi=301 (BE Go OK 1..600 but DB rejects → 500 BUG)    got=500
PASS  T1c Create durasi=601 (over BE Go cap → 400)                        got=400
PASS  T1d Create durasi=361 (FE form max blocks but BE Go OK; DB rejects) got=500
PASS  T2 jumlah_soal=201 (over BE cap → 400)                              got=400
PASS  T3a BankSoal poin=100 (BE cap)                                      got=201
PASS  T3b BankSoal poin=101 (over → 400)                                  got=400
PASS  T4a Image >5MB returns 413                                          got=413
PASS  T4b Image too-large code=payload_too_large (FE drift)               got=payload_too_large

=== Result: 9 pass, 0 fail ===
```

Findings #1, #2, #3 each have at least one PASS row above asserting the actual BE behavior that the FE layer does not match.

## Recommended fix batch

Single commit, mirror Fase 5 pattern `fix(fe-ujian): UX/QA pass Fase 6 — durasi bound, error mapper drift`:

| # | Severity | File / change                                                              | LOC |
|---|----------|----------------------------------------------------------------------------|-----|
| 1 | Critical | `backend/internal/ujian/service.go` — `MaxDurasiMenit = 600` → `300`; comment update | 2 |
| 2 | High     | `frontend/components/ujian/UjianFormDialog.tsx` — `360` → `300` (both validation + `<Input max>`); error message | 3 |
| 3 | Medium   | `frontend/lib/banksoal-api.ts` — drop `image_too_large` / `image_invalid_type` / `invalid_input` arms; add `payload_too_large` / `unsupported_mime` / `image_slot_empty` / `r2_unavailable` arms | ~12 |
| 4 | Low      | `frontend/lib/siswa-ujian-api.ts` — drop `'timer_expired'` arm | 1 |
| 5 | Low      | `frontend/lib/siswa-ujian-api.ts` — rename `'ujian_not_started'` → `'ujian_window_not_open'` | 1 |
| Boundary regression | (smoke) | `dogfood-output/fase6/smoke-bounds.sh` — already in repo; T1b should flip to expecting **400** after fix #1 | 1 |

Total ≈ 20 LOC. No migration needed.

## Decisions for user

- **Option A vs B for Finding #1.** Recommended A (cap Go validator at 300 to match DB). User already capped Soal Bab at 300 in `22d2095`. Picking A unless user wants B (DB migration).
- **Finding #3 fallback default.** Currently FE `default:` returns `err.message` for unknown codes. Acceptable — keeps maintenance light. Add only the user-facing image error codes; let internal `*_failed` codes fall through.
- **Finding #5 — keep both old + new arm?** Recommended dropping `'ujian_not_started'` entirely (no BE path emits it). Could keep both as alias if user wants resilience to a future rename.

## Testing notes

- **Did:** static FE↔BE↔DB diff, BE bound probe via curl (9/9), error code grep parity, source-code audit of `friendly*` mappers, cross-check vs locked decisions #76-#88, audit ledger column reads.
- **Did NOT:** live browser visual QA (CLI environment, no Playwright); SiswaUjianPlayer mid-flow image presign refresh test (would need live R2 + 12-min wait); manual play-through of Fase 6.G.2 player (TanStack Query cache + autosave inspection).
- **Coverage gate #88 (Fase 8):** still deferred. Recommend adding regression test against migration vs Go validator parity in Fase 8 to catch this class of drift earlier.

## Appendix — fix preview

```diff
--- a/backend/internal/ujian/service.go
+++ b/backend/internal/ujian/service.go
-	MinDurasiMenit    = 1
-	MaxDurasiMenit    = 600 // 10 jam max
+	MinDurasiMenit    = 1
+	MaxDurasiMenit    = 300 // 5 jam max (matches DB CHECK)

--- a/frontend/components/ujian/UjianFormDialog.tsx
- *   - durasi_menit (1-360)
+ *   - durasi_menit (1-300)
-    if (form.durasi_menit < 1 || form.durasi_menit > 360) {
-      e.durasi_menit = 'Durasi 1-360 menit.';
+    if (form.durasi_menit < 1 || form.durasi_menit > 300) {
+      e.durasi_menit = 'Durasi 1-300 menit.';
-                max={360}
+                max={300}

--- a/frontend/lib/banksoal-api.ts
-    case 'invalid_input':
-    case 'invalid_body':
+    case 'invalid_body':
-    case 'image_too_large':
-      return 'Gambar terlalu besar (>5MB).';
-    case 'image_invalid_type':
-      return 'Tipe gambar tidak didukung. Pakai JPG/PNG/WEBP.';
+    case 'payload_too_large':
+      return 'Gambar terlalu besar (>5MB).';
+    case 'unsupported_mime':
+      return 'Tipe gambar tidak didukung. Pakai JPG/PNG/WEBP.';
+    case 'image_slot_empty':
+      return 'Slot gambar kosong.';
+    case 'r2_unavailable':
+      return 'Penyimpanan gambar belum dikonfigurasi. Hubungi admin.';

--- a/frontend/lib/siswa-ujian-api.ts
-    case 'ujian_not_started':
+    case 'ujian_window_not_open':
       return 'Ujian belum dibuka. Tunggu sampai waktu mulai tiba.';
-    case 'ujian_timer_expired':
-    case 'timer_expired':
+    case 'ujian_timer_expired':
       return 'Waktu ujian sudah habis. Refresh untuk lihat hasil.';
```
