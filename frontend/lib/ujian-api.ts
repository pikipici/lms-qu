/**
 * Ujian (Ulangan Harian) API client — types + functions untuk
 * /api/v1/{kelas/:id/ujian, ujian/:id, hasil-ujian/:id} (Fase 6 Task 6.F.2).
 *
 * Backend contracts (Task 6.B+6.C+6.D+6.E):
 *   POST   /kelas/:id/ujian                   → create
 *   GET    /kelas/:id/ujian                   → list per kelas
 *   GET    /ujian/:id                         → detail
 *   PATCH  /ujian/:id                         → partial update + version
 *   DELETE /ujian/:id                         → hard delete + version
 *   POST   /ujian/:id/duplicate               → clone (status reset draft)
 *   POST   /ujian/:id/source/preview          → preview pool size + sample
 *   GET    /ujian/:id/hasil-rekap             → guru rekap (per-siswa)
 *   POST   /hasil-ujian/:id/cancel            → guru/admin soft-cancel
 *
 * Locked decisions referenced:
 *   - #56 optimistic concurrency (PATCH+DELETE wajib version)
 *   - #84 BankSoal pribadi cross-kelas (source draws from caller bank)
 *   - #85 source mode discriminated jsonb (manual ids vs random filter)
 *   - #86 random pool deterministic seed (snapshot saat siswa start)
 *   - #87 timer expire cron 30s + advisory lock auto-grade
 */

import { ApiError, api } from '@/lib/api';

// ---------- Types ----------

export type UjianStatus = 'draft' | 'published' | 'archived';

export type UjianSourceMode = 'manual' | 'random';

export interface UjianRandomFilter {
  mapel?: string;
  tingkat?: string;
  topik?: string;
}

/**
 * Discriminated source config (locked #85). Stored di backend sebagai
 * jsonb; FE kirim raw object di body PATCH/POST.
 */
export type UjianSourceConfig =
  | { mode: 'manual'; soal_ids: string[] }
  | {
      mode: 'random';
      filter?: UjianRandomFilter;
      jumlah_soal: number;
    };

export interface Ujian {
  id: string;
  kelas_id: string;
  guru_id: string;
  judul: string;
  deskripsi: string;
  durasi_menit: number;
  waktu_mulai?: string | null;
  waktu_selesai?: string | null;
  source_config_json: UjianSourceConfig | Record<string, never>;
  izinkan_review_setelah_submit: boolean;
  waktu_buka_review?: string | null;
  batas_attempt: number;
  attempt_unlimited: boolean;
  bobot: number;
  status: UjianStatus;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface UjianListResponse {
  items: Ujian[];
  total: number;
  limit: number;
  offset: number;
}

export interface CreateUjianInput {
  judul: string;
  deskripsi?: string;
  durasi_menit: number;
  waktu_mulai?: string | null;
  waktu_selesai?: string | null;
  izinkan_review_setelah_submit?: boolean;
  waktu_buka_review?: string | null;
  batas_attempt?: number;
  attempt_unlimited?: boolean;
  bobot?: number;
  status?: UjianStatus;
  source?: UjianSourceConfig;
}

export interface UpdateUjianInput {
  version: number;
  judul?: string;
  deskripsi?: string;
  durasi_menit?: number;
  waktu_mulai?: string | null;
  waktu_selesai?: string | null;
  izinkan_review_setelah_submit?: boolean;
  waktu_buka_review?: string | null;
  batas_attempt?: number;
  attempt_unlimited?: boolean;
  bobot?: number;
  status?: UjianStatus;
  source?: UjianSourceConfig;
}

export interface UjianSourcePreview {
  mode: UjianSourceMode;
  pool_size: number;
  jumlah_soal: number;
  soal_ids?: string[];
}

// ---------- Hasil rekap (Task 6.E.1) ----------

export interface SiswaRekap {
  siswa_id: string;
  siswa_name: string;
  siswa_email: string;
  attempt_count: number;
  cancelled_count: number;
  nilai_terbaik?: number | null;
  nilai_terakhir?: number | null;
  status_terakhir?: string;
  hasil_terakhir_id?: string | null;
  mulai_terakhir_at?: string | null;
}

export interface RekapHasilUjian {
  ujian_id: string;
  total: number;
  rata_rata?: number | null;
  items: SiswaRekap[];
}

export interface CancelUjianHasilResult {
  hasil_id: string;
  ujian_id: string;
  siswa_id: string;
  status: string;
  attempt_no: number;
  cancelled_at: string;
}

// ---------- Functions ----------

export async function listUjianByKelas(
  kelasID: string,
  opts: { status?: UjianStatus; limit?: number; offset?: number } = {},
): Promise<UjianListResponse> {
  const q = new URLSearchParams();
  if (opts.status) q.set('status', opts.status);
  if (typeof opts.limit === 'number' && opts.limit > 0) {
    q.set('limit', String(opts.limit));
  }
  if (typeof opts.offset === 'number' && opts.offset > 0) {
    q.set('offset', String(opts.offset));
  }
  const qs = q.toString();
  return api<UjianListResponse>(
    `/kelas/${kelasID}/ujian${qs ? `?${qs}` : ''}`,
  );
}

export async function getUjian(id: string): Promise<{ ujian: Ujian }> {
  return api<{ ujian: Ujian }>(`/ujian/${id}`);
}

export async function createUjian(
  kelasID: string,
  input: CreateUjianInput,
): Promise<{ ujian: Ujian }> {
  return api<{ ujian: Ujian }>(`/kelas/${kelasID}/ujian`, {
    method: 'POST',
    body: input,
  });
}

export async function updateUjian(
  id: string,
  input: UpdateUjianInput,
): Promise<{ ujian: Ujian }> {
  return api<{ ujian: Ujian }>(`/ujian/${id}`, {
    method: 'PATCH',
    body: input,
  });
}

export async function deleteUjian(
  id: string,
  version: number,
): Promise<{ ujian_id: string; deleted: boolean }> {
  return api<{ ujian_id: string; deleted: boolean }>(`/ujian/${id}`, {
    method: 'DELETE',
    body: { version },
  });
}

export async function forceDeleteUjianTesting(id: string): Promise<{
  ujian_id: string;
  deleted: boolean;
  hasil_deleted: number;
  jawaban_deleted: number;
}> {
  return api<{
    ujian_id: string;
    deleted: boolean;
    hasil_deleted: number;
    jawaban_deleted: number;
  }>(`/ujian/${id}/force-delete-testing`, {
    method: 'POST',
    body: {},
  });
}

export async function duplicateUjian(
  id: string,
  judul?: string,
): Promise<{ ujian: Ujian }> {
  return api<{ ujian: Ujian }>(`/ujian/${id}/duplicate`, {
    method: 'POST',
    body: judul ? { judul } : {},
  });
}

export async function previewUjianSource(
  id: string,
  source: UjianSourceConfig,
): Promise<{ preview: UjianSourcePreview }> {
  return api<{ preview: UjianSourcePreview }>(
    `/ujian/${id}/source/preview`,
    {
      method: 'POST',
      body: { source },
    },
  );
}

export async function getRekapHasilUjian(
  ujianID: string,
): Promise<{ rekap: RekapHasilUjian }> {
  return api<{ rekap: RekapHasilUjian }>(`/ujian/${ujianID}/hasil-rekap`);
}

export async function cancelUjianHasil(
  hasilID: string,
): Promise<{ hasil: CancelUjianHasilResult }> {
  return api<{ hasil: CancelUjianHasilResult }>(
    `/hasil-ujian/${hasilID}/cancel`,
    { method: 'POST', body: {} },
  );
}

// ---------- Friendly error mapper ----------

export type UjianAction =
  | 'create'
  | 'update'
  | 'delete'
  | 'force_delete_testing'
  | 'duplicate'
  | 'preview'
  | 'cancel'
  | 'rekap';

export function friendlyUjianError(
  err: ApiError,
  action: UjianAction,
): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID ujian tidak valid.';
    case 'invalid_body':
    case 'invalid_input':
      return (
        err.message || 'Input tidak valid. Periksa kembali data yang kamu kirim.'
      );
    case 'invalid_version':
      return 'Versi ujian tidak valid. Refresh dulu.';
    case 'invalid_status':
      return 'Status harus draft, published, atau archived.';
    case 'invalid_source':
      return err.message || 'Konfigurasi sumber soal tidak valid.';
    case 'source_missing':
      return 'Sumber soal wajib dipilih (manual atau random).';
    case 'source_pool_empty':
      return 'Filter random tidak menghasilkan soal. Coba longgarkan tag mapel/tingkat/topik.';
    case 'soal_not_in_bank':
      return 'Salah satu soal yang dipilih tidak ada di Bank Soal milik kamu.';
    case 'version_conflict':
      return 'Ujian baru saja di-update. Form sudah di-refresh — silakan ulangi perubahan kamu.';
    case 'forbidden':
      return 'Kamu tidak punya akses ke ujian ini.';
    case 'not_found':
      return 'Ujian tidak ditemukan (mungkin sudah dihapus).';
    case 'attempts_exist':
      return 'Ujian ini sudah dipakai siswa; tidak bisa dihapus. Archive saja.';
    case 'testing_cleanup_disabled':
      return 'Cleanup testing dimatikan di environment ini.';
    case 'active_attempts_block':
      return 'Ada siswa yang sedang mengerjakan ujian ini. Cancel attempt mereka dulu sebelum ubah timing/sumber soal.';
    case 'kelas_archived':
      return 'Kelas sudah diarsipkan; ujian tidak bisa diubah.';
    case 'review_locked':
      return 'Review belum dibuka oleh guru.';
    case 'review_disabled':
      return 'Review attempt ini dimatikan oleh guru.';
    case 'hasil_already_cancelled':
      return 'Attempt ini sudah dibatalkan sebelumnya.';
    case 'hasil_not_finished':
      return 'Attempt belum selesai; review/rekap belum tersedia.';
    default:
      switch (action) {
        case 'create':
          return err.message || 'Gagal membuat ujian.';
        case 'update':
          return err.message || 'Gagal menyimpan ujian.';
        case 'delete':
          return err.message || 'Gagal menghapus ujian.';
        case 'force_delete_testing':
          return err.message || 'Gagal menghapus data testing ujian.';
        case 'duplicate':
          return err.message || 'Gagal menduplikasi ujian.';
        case 'preview':
          return err.message || 'Gagal memuat preview sumber soal.';
        case 'cancel':
          return err.message || 'Gagal membatalkan attempt.';
        case 'rekap':
          return err.message || 'Gagal memuat rekap hasil.';
        default:
          return err.message;
      }
  }
}
