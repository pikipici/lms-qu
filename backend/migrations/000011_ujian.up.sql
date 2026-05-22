-- LMS — Phase 6 schema bootstrap (Fase 6): BankSoal + Ujian + UjianSoal +
-- HasilUjian + JawabanUjian + EventUjian.
--
-- Ujian (Ulangan Harian, locked decisions #83-#88) covers cross-bab graded
-- assessment with two source modes (locked #85):
--   manual — guru pick soal_ids[] explicit dari Bank Soal sendiri.
--   random — filter (mapel?,tingkat?,topik?) + jumlah_soal apply ke Bank Soal.
--
-- BankSoal (locked #84) — per-guru pribadi pool soal lintas-bab. Tag fields
-- mapel + tingkat + topik free-form text. Tidak ada FK ke Bab; sebaliknya
-- Ujian (per-kelas) draw dari pool ini. Image slot mirror SoalBab (#78):
--   pertanyaan + 5 opsi a..e → R2 prefix soal-bank/<uuid>.<ext>.
--
-- Ujian — instance per-kelas, owned by guru kelas. SourceConfigJSON jsonb
-- discriminated:
--   {mode:"manual", soal_ids:[uuid, ...]}
--   {mode:"random", filter:{mapel?,tingkat?,topik?}, jumlah_soal:N}
-- UjianSoal junction caches manual-mode soal_ids dengan urutan untuk FK
-- safety + cascade. Random mode tidak populate UjianSoal — siswa start
-- snapshot via deterministic seed (locked #86) langsung dari pool filter.
--
-- HasilUjian — single attempt instance. Partial unique (ujian_id, siswa_id)
-- WHERE deleted_at IS NULL supaya remedial reset (soft delete via deleted_at)
-- tetap bisa bikin attempt baru. Status enum mirror SoalBab (#76):
-- berlangsung|selesai|dibatalkan. SoalIDsJSON frozen snapshot (locked #86).
-- Cron 30s sweep auto-grade (locked #87 reuse goroutine soalbab/timer_cron).
--
-- JawabanUjian — per-soal answer mirror JawabanBab. UNIQUE (hasil_id, soal_id)
-- → UPSERT autosave aman. Ulangan delayed grade: is_benar NULL + poin_dapat 0
-- sampai submit, lalu grade batch dalam tx.
--
-- EventUjian — anti-cheat audit ledger mirror EventBab.
--
-- Reuses set_updated_at() trigger function from 000002.

CREATE TABLE IF NOT EXISTS bank_soal (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_guru_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    mapel TEXT NOT NULL DEFAULT '',
    tingkat TEXT NOT NULL DEFAULT '',
    topik TEXT NOT NULL DEFAULT '',
    pertanyaan TEXT NOT NULL DEFAULT '',
    pertanyaan_object_key TEXT,
    opsi_a TEXT NOT NULL DEFAULT '',
    opsi_a_object_key TEXT,
    opsi_b TEXT NOT NULL DEFAULT '',
    opsi_b_object_key TEXT,
    opsi_c TEXT NOT NULL DEFAULT '',
    opsi_c_object_key TEXT,
    opsi_d TEXT NOT NULL DEFAULT '',
    opsi_d_object_key TEXT,
    opsi_e TEXT NOT NULL DEFAULT '',
    opsi_e_object_key TEXT,
    jawaban TEXT NOT NULL CHECK (jawaban IN ('a', 'b', 'c', 'd', 'e')),
    poin SMALLINT NOT NULL DEFAULT 1 CHECK (poin BETWEEN 1 AND 100),
    version INTEGER NOT NULL DEFAULT 1,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE bank_soal IS 'Bank Soal per-guru pribadi (locked #84). Soal lintas-bab dengan tag mapel/tingkat/topik free-form. 5 opsi a..e + 1 jawaban kunci + 6 image slot R2 prefix soal-bank/<uuid> (locked #78). Ownership: WHERE owner_guru_id = current_user. Soft delete via deleted_at supaya HasilUjian referensi tetap valid kalau guru hapus soal setelah ada attempt.';

-- Random filter + ownership query path
CREATE INDEX IF NOT EXISTS idx_bank_soal_owner_filter
    ON bank_soal USING btree (owner_guru_id, mapel, tingkat)
    WHERE deleted_at IS NULL;

-- List pagination by owner
CREATE INDEX IF NOT EXISTS idx_bank_soal_owner_created
    ON bank_soal USING btree (owner_guru_id, created_at DESC)
    WHERE deleted_at IS NULL;

DROP TRIGGER IF EXISTS bank_soal_set_updated_at ON bank_soal;
CREATE TRIGGER bank_soal_set_updated_at
    BEFORE UPDATE ON bank_soal
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS ujian (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kelas_id UUID NOT NULL REFERENCES kelas(id) ON DELETE CASCADE,
    guru_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    judul TEXT NOT NULL DEFAULT '',
    deskripsi TEXT NOT NULL DEFAULT '',
    durasi_menit SMALLINT NOT NULL DEFAULT 60 CHECK (durasi_menit BETWEEN 1 AND 300),
    waktu_mulai TIMESTAMPTZ,
    waktu_selesai TIMESTAMPTZ,
    source_config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    izinkan_review_setelah_submit BOOLEAN NOT NULL DEFAULT true,
    waktu_buka_review TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE ujian IS 'Ujian (Ulangan Harian) instance per-kelas. SourceConfigJSON discriminated (locked #85): manual = {mode:"manual",soal_ids:[]}, random = {mode:"random",filter:{...},jumlah_soal:N}. Pool guru_id = kelas.guru_id (denormal). Status draft → published → archived mirror Bab.Status (locked Fase 3 pattern). Review gating mirror SoalBab (locked #81). Optimistic concurrency via version (locked #56). Window waktu_mulai/waktu_selesai optional — null = always open / no end.';

CREATE INDEX IF NOT EXISTS idx_ujian_kelas_status
    ON ujian USING btree (kelas_id, status);

CREATE INDEX IF NOT EXISTS idx_ujian_guru
    ON ujian USING btree (guru_id);

DROP TRIGGER IF EXISTS ujian_set_updated_at ON ujian;
CREATE TRIGGER ujian_set_updated_at
    BEFORE UPDATE ON ujian
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS ujian_soal (
    ujian_id UUID NOT NULL REFERENCES ujian(id) ON DELETE CASCADE,
    soal_id UUID NOT NULL REFERENCES bank_soal(id) ON DELETE RESTRICT,
    urutan SMALLINT NOT NULL DEFAULT 0,
    PRIMARY KEY (ujian_id, soal_id)
);

COMMENT ON TABLE ujian_soal IS 'Junction Ujian × BankSoal untuk source mode=manual (locked #85). Cache soal_ids + urutan supaya FK safety + cascade saat Ujian dihapus + simple ordered fetch. Random mode TIDAK populate junction — siswa start snapshot langsung pakai deterministic seed (locked #86) di atas filter pool. Edit Ujian dengan attempt aktif → 409 ujian_active_attempts (handler-level guard).';

CREATE INDEX IF NOT EXISTS idx_ujian_soal_ujian_urutan
    ON ujian_soal USING btree (ujian_id, urutan);

CREATE TABLE IF NOT EXISTS hasil_ujian (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ujian_id UUID NOT NULL REFERENCES ujian(id) ON DELETE RESTRICT,
    siswa_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'berlangsung' CHECK (status IN ('berlangsung', 'selesai', 'dibatalkan')),
    soal_ids_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    mulai_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deadline_at TIMESTAMPTZ,
    selesai_at TIMESTAMPTZ,
    nilai_total NUMERIC(6, 2),
    jawaban_benar_count SMALLINT,
    jawaban_total SMALLINT,
    attempt_no SMALLINT NOT NULL DEFAULT 1,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE hasil_ujian IS 'Single attempt Ujian per (ujian, siswa). Partial unique (ujian_id, siswa_id) WHERE deleted_at IS NULL → remedial reset (locked #45 mirror) soft-delete attempt lama, baru attempt boleh lewat. SoalIDsJSON snapshot deterministic seed sha256(mulai_unix_micro||siswa||ujian) (locked #86) — frozen per attempt anti refresh-shuffle. DeadlineAt = mulai_at + durasi_menit; cron 30s sweep auto-grade (locked #87, reuse goroutine SoalBab timer_cron). NilaiTotal+JawabanBenarCount diisi saat submit/cron auto-grade.';

CREATE UNIQUE INDEX IF NOT EXISTS idx_hasil_ujian_unique_active
    ON hasil_ujian USING btree (ujian_id, siswa_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_hasil_ujian_siswa
    ON hasil_ujian USING btree (siswa_id, status)
    WHERE deleted_at IS NULL;

-- Cron sweep target: ulangan berlangsung yang lewat deadline. Partial index hemat space.
CREATE INDEX IF NOT EXISTS idx_hasil_ujian_active_deadline
    ON hasil_ujian USING btree (deadline_at)
    WHERE status = 'berlangsung' AND deleted_at IS NULL;

DROP TRIGGER IF EXISTS hasil_ujian_set_updated_at ON hasil_ujian;
CREATE TRIGGER hasil_ujian_set_updated_at
    BEFORE UPDATE ON hasil_ujian
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS jawaban_ujian (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hasil_id UUID NOT NULL REFERENCES hasil_ujian(id) ON DELETE CASCADE,
    soal_id UUID NOT NULL REFERENCES bank_soal(id) ON DELETE RESTRICT,
    jawaban TEXT,
    is_benar BOOLEAN,
    poin_dapat SMALLINT NOT NULL DEFAULT 0,
    answered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (hasil_id, soal_id)
);

COMMENT ON TABLE jawaban_ujian IS 'Jawaban siswa per (hasil, soal). UNIQUE (hasil_id, soal_id) supaya UPSERT autosave aman + re-answer overwrite. Ulangan: is_benar NULL + poin_dapat 0 sampai submit, lalu grade batch dalam tx (locked #87). Mirror jawaban_bab pattern.';

CREATE INDEX IF NOT EXISTS idx_jawaban_ujian_hasil
    ON jawaban_ujian USING btree (hasil_id);

CREATE TABLE IF NOT EXISTS event_ujian (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hasil_id UUID NOT NULL REFERENCES hasil_ujian(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE event_ujian IS 'Anti-cheat audit ledger per HasilUjian attempt. Action enum: soal_view, answer_save, submit, timer_expire, resume, cancel. Meta jsonb opaque (locked #55-style ledger pattern). Mirror event_bab.';

CREATE INDEX IF NOT EXISTS idx_event_ujian_hasil_created
    ON event_ujian USING btree (hasil_id, created_at);

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000011_ujian')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
