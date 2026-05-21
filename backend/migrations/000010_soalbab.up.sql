-- LMS — Phase 5 schema bootstrap (Fase 5): SoalBab + UlanganBabSetting +
-- HasilSoalBab + JawabanBab + EventBab + SoalAssignment.
--
-- Soal Bab covers two flows:
--   Latihan      — formative practice, no nilai persist, immediate is_benar feedback.
--   Ulangan Bab  — graded attempt(s) with timer, attempt limit, optional review gating.
--
-- SoalBab fields (locked #77/#78):
--   - 5 fixed opsi (a..e) + 1 jawaban kunci ('a'..'e').
--   - 6 image slot (pertanyaan + a..e), each via R2 object key (locked #62, #78).
--   - mode enum drives flow eligibility: latihan-only, ulangan-only, atau keduanya.
--   - poin smallint default 1, range 1-100.
--
-- UlanganBabSetting (1:1 dengan Bab):
--   - jumlah_soal: berapa soal di-snapshot per attempt (must ≤ pool size mode IN ('ulangan','keduanya')).
--   - durasi_menit: timer countdown.
--   - batas_attempt: berapa kali siswa boleh attempt (default 1).
--   - izinkan_review_setelah_submit + waktu_buka_review (locked #81).
--
-- HasilSoalBab — single attempt instance:
--   - mode = 'latihan' atau 'ulangan'.
--   - status = 'berlangsung' | 'selesai' | 'dibatalkan'.
--   - soal_ids_json (jsonb) frozen pool snapshot (locked #79 deterministic seed sha256).
--   - deadline_at (ulangan only); cron 30s sweep auto-grade kalau lewat (locked #80).
--   - nilai_total + jawaban_benar_count diisi saat submit (ulangan) atau finish manual (latihan: keep null).
--   - attempt_no untuk audit remedial chain.
--
-- JawabanBab — siswa answers per (hasil, soal):
--   - UNIQUE (hasil_id, soal_id) supaya UPSERT autosave aman dan re-answer overwrite.
--   - is_benar + poin_dapat di-grade saat answer (latihan) atau saat submit (ulangan).
--
-- EventBab — anti-cheat audit per attempt (soal_view, answer_save, submit, timer_expire).
--
-- SoalAssignment — audit copy soal antar bab (Fase 5+ guru convenience).
--
-- Reuses set_updated_at() trigger function from 000002.

CREATE TABLE IF NOT EXISTS soal_bab (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bab_id UUID NOT NULL REFERENCES bab(id) ON DELETE CASCADE,
    kelas_id UUID NOT NULL REFERENCES kelas(id) ON DELETE RESTRICT,
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
    mode TEXT NOT NULL DEFAULT 'keduanya' CHECK (mode IN ('latihan', 'ulangan', 'keduanya')),
    urutan INTEGER NOT NULL DEFAULT 0,
    version INTEGER NOT NULL DEFAULT 1,
    created_by_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE soal_bab IS 'Soal multiple-choice 5 opsi per bab. Mode latihan/ulangan/keduanya drive flow eligibility (locked #76). Inline 6 image slot (pertanyaan + a..e) ke R2 prefix soalbab/<uuid> (locked #78). Optimistic concurrency via version (locked #56). Kelas_id denormal untuk query cepat without join via bab.';

CREATE INDEX IF NOT EXISTS idx_soal_bab_bab_mode
    ON soal_bab USING btree (bab_id, mode);

CREATE INDEX IF NOT EXISTS idx_soal_bab_bab_urutan
    ON soal_bab USING btree (bab_id, urutan);

DROP TRIGGER IF EXISTS soal_bab_set_updated_at ON soal_bab;
CREATE TRIGGER soal_bab_set_updated_at
    BEFORE UPDATE ON soal_bab
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS ulangan_bab_setting (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bab_id UUID NOT NULL UNIQUE REFERENCES bab(id) ON DELETE CASCADE,
    jumlah_soal SMALLINT NOT NULL DEFAULT 10 CHECK (jumlah_soal BETWEEN 1 AND 200),
    durasi_menit SMALLINT NOT NULL DEFAULT 30 CHECK (durasi_menit BETWEEN 1 AND 300),
    batas_attempt SMALLINT NOT NULL DEFAULT 1 CHECK (batas_attempt BETWEEN 1 AND 10),
    izinkan_review_setelah_submit BOOLEAN NOT NULL DEFAULT true,
    waktu_buka_review TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE ulangan_bab_setting IS 'Setting Ulangan Bab per Bab (1:1 via UNIQUE bab_id). Jumlah_soal harus ≤ count(soal mode IN ulangan/keduanya). Review gating (locked #81): izinkan_review_setelah_submit + waktu_buka_review. Optimistic concurrency via version (locked #56).';

DROP TRIGGER IF EXISTS ulangan_bab_setting_set_updated_at ON ulangan_bab_setting;
CREATE TRIGGER ulangan_bab_setting_set_updated_at
    BEFORE UPDATE ON ulangan_bab_setting
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS hasil_soal_bab (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bab_id UUID NOT NULL REFERENCES bab(id) ON DELETE RESTRICT,
    siswa_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    mode TEXT NOT NULL CHECK (mode IN ('latihan', 'ulangan')),
    status TEXT NOT NULL DEFAULT 'berlangsung' CHECK (status IN ('berlangsung', 'selesai', 'dibatalkan')),
    soal_ids_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    mulai_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deadline_at TIMESTAMPTZ,
    selesai_at TIMESTAMPTZ,
    nilai_total NUMERIC(6, 2),
    jawaban_benar_count SMALLINT,
    jawaban_total SMALLINT,
    attempt_no SMALLINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE hasil_soal_bab IS 'Single attempt instance dari siswa untuk Latihan atau Ulangan Bab. soal_ids_json snapshot deterministic seed sha256(mulai_unix_micro||siswa_id||bab_id) (locked #79) — frozen per attempt supaya resume aman + anti-cheat refresh-shuffle. Deadline_at hanya untuk ulangan; cron 30s sweep auto-grade lewat advisory lock (locked #80). Latihan: nilai_total+jawaban_benar_count NULL (formative). Ulangan: diisi saat submit/cron auto-grade. Status dibatalkan = remedial reset by guru (locked #76).';

CREATE INDEX IF NOT EXISTS idx_hasil_soal_bab_bab_siswa_mode_status
    ON hasil_soal_bab USING btree (bab_id, siswa_id, mode, status);

-- Cron sweep target: ulangan berlangsung yang lewat deadline. Partial index hemat space.
CREATE INDEX IF NOT EXISTS idx_hasil_soal_bab_active_deadline
    ON hasil_soal_bab USING btree (deadline_at)
    WHERE status = 'berlangsung' AND mode = 'ulangan';

DROP TRIGGER IF EXISTS hasil_soal_bab_set_updated_at ON hasil_soal_bab;
CREATE TRIGGER hasil_soal_bab_set_updated_at
    BEFORE UPDATE ON hasil_soal_bab
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS jawaban_bab (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hasil_id UUID NOT NULL REFERENCES hasil_soal_bab(id) ON DELETE CASCADE,
    soal_id UUID NOT NULL REFERENCES soal_bab(id) ON DELETE RESTRICT,
    jawaban TEXT,
    is_benar BOOLEAN,
    poin_dapat SMALLINT NOT NULL DEFAULT 0,
    answered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (hasil_id, soal_id)
);

COMMENT ON TABLE jawaban_bab IS 'Jawaban siswa per (hasil, soal). UNIQUE (hasil_id, soal_id) supaya UPSERT autosave aman + re-answer overwrite. Latihan: is_benar+poin_dapat diisi saat answer (immediate feedback). Ulangan: is_benar+poin_dapat NULL/0 sampai submit, lalu di-grade batch dalam tx (locked #80).';

CREATE INDEX IF NOT EXISTS idx_jawaban_bab_hasil
    ON jawaban_bab USING btree (hasil_id);

CREATE TABLE IF NOT EXISTS event_bab (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hasil_id UUID NOT NULL REFERENCES hasil_soal_bab(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    meta JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE event_bab IS 'Anti-cheat audit ledger per HasilSoalBab attempt. Action enum: soal_view, answer_save, submit, timer_expire, resume. Meta jsonb opaque (locked #55-style ledger pattern). Index (hasil_id, created_at) untuk timeline replay.';

CREATE INDEX IF NOT EXISTS idx_event_bab_hasil_created
    ON event_bab USING btree (hasil_id, created_at);

CREATE TABLE IF NOT EXISTS soal_assignment (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_bab_id UUID NOT NULL REFERENCES bab(id) ON DELETE RESTRICT,
    target_bab_id UUID NOT NULL REFERENCES bab(id) ON DELETE RESTRICT,
    copied_count INTEGER NOT NULL DEFAULT 0,
    created_by_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_bab_id, target_bab_id)
);

COMMENT ON TABLE soal_assignment IS 'Audit trail kalau guru copy soal antar bab. UNIQUE (source, target) supaya idempotent — re-copy same pair akan replace count saja. Out-of-scope MVP untuk endpoint; defer ke Fase 5+ kalau guru request.';

CREATE INDEX IF NOT EXISTS idx_soal_assignment_target
    ON soal_assignment USING btree (target_bab_id);

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000010_soalbab')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
