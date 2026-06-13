# Bank Soal Multi-Tags Plan

Status: Implemented locally, pending deploy
Owner context: Fase 8 polish / production-readiness
Last updated: 2026-06-13

## Goal

Add real multi-tags for Bank Soal so guru can classify, search, reuse, and compose ujian questions faster with flexible labels such as `hots`, `remedial`, `grafik`, `analisis-data`, `mudah`, or `kelas-8`.

The current `mapel`, `tingkat`, and `topik` fields stay supported as structured legacy tags. The final UX should feel like tags/chips, not a rigid three-field taxonomy.

## Current State

Implemented today:

- Backend `bank_soal` table has `mapel`, `tingkat`, and `topik` fields only.
- Backend list endpoint supports:
  - `GET /api/v1/bank-soal?mapel=...`
  - `GET /api/v1/bank-soal?tingkat=...`
  - `GET /api/v1/bank-soal?topik=...`
- Ujian random source config uses `mapel/tingkat/topik` filter.
- Bulk create supports default `mapel/tingkat/topik` in request body.
- FE bridge UI already started:
  - `frontend/components/banksoal/BankSoalList.tsx` shows tag-like filter chips for `mapel`, `tingkat`, and `topik`.
  - `frontend/components/ujian/UjianSourceConfigPanel.tsx` uses tag-like chips for manual/random source filtering.

Implemented locally after this plan:

- `tags TEXT[]` migration exists with GIN index.
- Bank Soal API model/create/update/list supports `tags: string[]`.
- Bank Soal list supports `?tag=` and `?tags=` OR-overlap filtering.
- Ujian random source preview/start supports `filter.tags`.
- FE create/edit includes multi-tag chip editor.
- FE Bank Soal list and Ujian source config include free-tag filters.

Still not implemented:

- Dedicated tag suggestions endpoint.
- Bulk import request/per-row tags.

## Design Direction

Use `tags TEXT[]` on `bank_soal` for the first real multi-tag implementation.

Reasoning:

- Simpler than a normalized `bank_soal_tag` table.
- Good enough for per-guru private Bank Soal.
- PostgreSQL supports array overlap/contains queries and GIN index.
- Migration to normalized tags remains possible later if we need tag ownership, aliases, descriptions, usage count, or admin taxonomy.

Compatibility rule:

- `mapel`, `tingkat`, and `topik` remain in DB/API for existing data and existing Ujian source configs.
- New `tags` are additive.
- UI can show structured fields and free tags together, but should visually separate them enough to avoid confusion.

## Proposed Data Model

Migration candidate: `backend/migrations/000025_bank_soal_tags.up.sql`

```sql
ALTER TABLE bank_soal
ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_bank_soal_tags_gin
ON bank_soal USING gin (tags)
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_bank_soal_owner_tags
ON bank_soal USING btree (owner_guru_id)
WHERE deleted_at IS NULL;
```

Down migration:

```sql
DROP INDEX IF EXISTS idx_bank_soal_owner_tags;
DROP INDEX IF EXISTS idx_bank_soal_tags_gin;
ALTER TABLE bank_soal DROP COLUMN IF EXISTS tags;
```

Backfill option:

- Do not automatically copy `mapel/tingkat/topik` into `tags` at migration time.
- Keep them separate initially to avoid duplicate/confusing tags like `Informatika` appearing both as structured `mapel` and free tag.
- FE may offer a one-click suggestion to add structured fields as tags later, but not automatic for MVP.

## Tag Normalization Rules

Backend should normalize tags before save:

- Trim spaces.
- Lowercase or preserve case? Recommendation: preserve display case but compare/search case-insensitively is harder with arrays. For MVP, normalize to lowercase kebab-ish labels.
- Collapse internal whitespace to `-` or single space. Recommendation: kebab-case for stability.
- Remove empty tags.
- Deduplicate.
- Max tags per soal: 20.
- Max tag length: 40 chars.
- Allowed chars: letters, numbers, spaces, dash, underscore. Reject dangerous punctuation.

Recommended examples:

- Input `Analisis Data` -> stored `analisis-data`
- Input `HOTS` -> stored `hots`
- Input `kelas 8` -> stored `kelas-8`

Error codes candidate:

- `too_many_tags`
- `tag_too_long`
- `invalid_tag`

## Backend Implementation Plan

### Step BE-1: Model + Migration

Files likely touched:

- `backend/migrations/000025_bank_soal_tags.up.sql`
- `backend/migrations/000025_bank_soal_tags.down.sql`
- `backend/internal/banksoal/model.go`

Change:

- Add `Tags []string` to `BankSoal` using GORM/Postgres array type.
- Confirm serialization returns `tags: []` not `null`.

Implementation note:

- Existing code uses GORM. Use `pq.StringArray` or GORM-compatible serializer after checking existing dependencies.

### Step BE-2: Create/Update/List API

Files likely touched:

- `backend/internal/banksoal/handler.go`
- `backend/internal/banksoal/service.go`
- `backend/internal/banksoal/repo.go`

Change:

- Add `tags?: string[]` to create request.
- Add `tags?: string[]` to update request.
- Add `tags` to response via model JSON.
- Add list filters:
  - `?tag=hots` for single tag.
  - `?tags=hots,remedial` for OR overlap. Recommendation for MVP: OR overlap.
  - Later option: `tags_mode=all` for AND contains.

Repo query candidate:

```sql
tags && ?::text[]
```

or GORM equivalent.

### Step BE-3: Suggestions Endpoint

Optional but recommended for UX.

Endpoint candidate:

- `GET /api/v1/bank-soal/tags`

Response:

```json
{
  "tags": ["analisis-data", "hots", "remedial"],
  "structured": {
    "mapel": ["Informatika"],
    "tingkat": ["Kelas 8"],
    "topik": ["Grafik"]
  }
}
```

This avoids fetching only first 200 soal to derive filter options.

### Step BE-4: Bulk Import

Files likely touched:

- `backend/internal/banksoal/bulk.go`
- `backend/internal/banksoal/bulk_handler.go`

Change:

- Add optional request-level `tags?: string[]`.
- Add optional per-row `tags` column if we extend pipe format.
- Recommendation: phase this carefully:
  - MVP multi-tags bulk: request-level default tags only.
  - Later: per-row extra column `tags` with comma-separated values.

### Step BE-5: Ujian Source Config

Files likely touched:

- `backend/internal/ujian/model.go`
- `backend/internal/ujian/service.go`
- `backend/internal/ujian/start.go`
- `backend/internal/ujian/handler.go`
- `backend/internal/ujian/items.go`

Change:

- Extend random source config filter:

```json
{
  "mode": "random",
  "filter": {
    "mapel": "Informatika",
    "tingkat": "Kelas 8",
    "topik": "Grafik",
    "tags": ["hots", "analisis-data"]
  },
  "jumlah_soal": 10
}
```

Compatibility:

- Existing configs without `tags` must continue working.
- Manual source mode does not need schema change, but manual picker can filter by tags client-side/server-side.

## Frontend Implementation Plan

### Step FE-1: API Types

Files likely touched:

- `frontend/lib/banksoal-api.ts`
- `frontend/lib/ujian-api.ts`

Change:

- Add `tags: string[]` to `BankSoal`.
- Add `tags?: string[]` to create/update/bulk/list options.
- Add `tags?: string[]` to Ujian source filter type.

### Step FE-2: Reusable Tag Components

Create candidate:

- `frontend/components/banksoal/BankSoalTagInput.tsx`
- or reusable `frontend/components/shared/TagInput.tsx` if generic enough.

Needed behavior:

- Type tag, press Enter/comma to add.
- Backspace removes last empty tag.
- Paste comma-separated tags.
- Show max tags and validation error.
- Suggest existing tags.
- Mobile friendly.

### Step FE-3: Bank Soal Create/Edit

Files likely touched:

- `frontend/components/banksoal/BankSoalEditDialog.tsx`

Change:

- Add `Tags` chip input near `mapel/tingkat/topik`.
- Copy should explain:
  - `Mapel/Tingkat/Topik` = structured filters.
  - `Tags` = bebas, multi-label.

### Step FE-4: Bank Soal List Filters

Files likely touched:

- `frontend/components/banksoal/BankSoalList.tsx`

Change:

- Add multi-select free tag chips.
- Preserve current `mapel/tingkat/topik` chips.
- Show active filters in one line:
  - `Mapel: Informatika`, `Tingkat: Kelas 8`, `Tag: hots`, `Tag: remedial`.
- Display free tags on each soal row as neutral badges.

### Step FE-5: Ujian Source Picker

Files likely touched:

- `frontend/components/ujian/UjianSourceConfigPanel.tsx`

Change:

- Manual mode: filter visible Bank Soal by free tags.
- Random mode: include selected free tags in `filter.tags`.
- Preview should include tag filter so guru sees real pool size.

### Step FE-6: Bulk Paste Dialog

Files likely touched:

- `frontend/components/banksoal/BankSoalBulkPasteDialog.tsx`

Change:

- Add default tags chip input applied to all rows.
- Later: document per-row `tags` column once backend supports it.

## Rollout Stages

### Stage 0: Bridge UI (Done)

- Use existing `mapel/tingkat/topik` as tag-like chips.
- No schema/API change.
- Local validation done: `npm run typecheck` and `npm run build` pass.

### Stage 1: Backend Multi-Tags MVP (Done locally)

- Migration adds `tags TEXT[]`.
- API create/update/list supports tags.
- Ujian preview/start random pools support tags.
- Local validation: `go test ./internal/banksoal ./internal/ujian` passes.

### Stage 2: FE Multi-Tags MVP (Mostly done locally)

- Add tag input to create/edit.
- Add tag display/filter to Bank Soal list.
- Add tag filter to Ujian source config.
- Typecheck/build pass.
- Bulk default tags remain pending.

### Stage 3: Ujian Integration (Done locally)

- Add tag filter to random source config and preview/start pool.
- Add tag filter to manual picker.
- Existing configs without tags remain compatible because `tags` is optional.

### Stage 4: Polish + Migration UX

- Optional tag suggestions endpoint.
- Optional convert/copy structured fields into free tags.
- Optional usage count and cleanup unused tags.

## Testing Checklist

Backend:

- `go test ./internal/banksoal`
- `go test ./internal/ujian`
- `go test ./...`
- Migration up/down on disposable DB or remote staging flow.
- API smoke:
  - Create soal with tags.
  - Update tags.
  - List by one tag.
  - List by multiple tags.
  - Empty tags returns `[]`.
  - Invalid tag rejected.

Frontend:

- `npm run typecheck`
- `npm run build`
- Manual browser smoke:
  - Create Bank Soal with multiple tags.
  - Edit tags.
  - Filter Bank Soal by tag.
  - Bulk default tags apply.
  - Ujian manual picker filter by tag.
  - Ujian random preview respects tags.

Deploy:

- Push origin + server.
- Remote deploy via existing script:

```bash
ssh rdpkhorur 'cd /home/ubuntu/lms && set -a; . ./.env; set +a; bash deploy/deploy.sh --remote'
```

- Verify `/api/v1/readyz`.
- Verify schema version.

## Risks

- GORM array type may need dependency/import choice (`pq.StringArray` vs serializer).
- Query semantics must be clear: OR overlap vs AND contains.
- Existing random source configs must not break.
- Existing FE code may assume only `mapel/tingkat/topik` tags.
- Bulk format changes can confuse users if not documented carefully.

## Decisions To Lock Before Implementation

1. Store tags as lowercase kebab-case or preserve user casing?
   - Recommendation: lowercase kebab-case.
2. Multi-tag filter semantics: any tag matches or all tags required?
   - Recommendation MVP: any/OR, later add `tags_mode=all`.
3. Should `mapel/tingkat/topik` auto-backfill into `tags`?
   - Recommendation: no automatic backfill.
4. Maximum tags per soal?
   - Recommendation: 20.
5. Maximum tag length?
   - Recommendation: 40 chars.
6. Bulk import per-row tags now or later?
   - Recommendation: request-level default tags first, per-row later.

## Progress Log

- 2026-06-13: Plan created. Current implementation remains bridge UI only, no real multi-tags yet.
- 2026-06-13: Implemented real multi-tags locally: backend `tags TEXT[]`, Bank Soal API tags, Ujian random tag filter, FE tag editor/filter. Validation passed: `go test ./internal/banksoal ./internal/ujian`, `npm run typecheck`, `npm run build`, and targeted `git diff --check`. Bulk import tags and suggestions endpoint still pending.
