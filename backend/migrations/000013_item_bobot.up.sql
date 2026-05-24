ALTER TABLE tugas ADD COLUMN IF NOT EXISTS bobot INTEGER NOT NULL DEFAULT 100;
ALTER TABLE ujian ADD COLUMN IF NOT EXISTS bobot INTEGER NOT NULL DEFAULT 100;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'tugas_bobot_non_negative'
  ) THEN
    ALTER TABLE tugas ADD CONSTRAINT tugas_bobot_non_negative CHECK (bobot >= 0);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'ujian_bobot_non_negative'
  ) THEN
    ALTER TABLE ujian ADD CONSTRAINT ujian_bobot_non_negative CHECK (bobot >= 0);
  END IF;
END $$;
