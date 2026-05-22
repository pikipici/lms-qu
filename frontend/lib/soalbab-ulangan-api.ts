/**
 * Ulangan Bab API client — siswa flow (Task 5.G.2).
 *
 * Backend contracts:
 *   GET   /siswa/bab/:id/ulangan-setting    → SiswaLobbyView (trimmed)
 *   GET   /siswa/bab/:id/hasil              → SiswaHasilListResult (history)
 *   POST  /siswa/bab/:id/ulangan/start      → start atau resume
 *   POST  /siswa/hasil-soal-bab/:id/answer  → upsert jawaban (ulangan: no feedback)
 *   POST  /siswa/hasil-soal-bab/:id/submit  → close + auto-grade (idempotent)
 *   GET   /siswa/hasil-soal-bab/:id/review  → review payload (gated #81)
 *   GET   /siswa/hasil-soal-bab/:id/items   → live items (already di soalbab-attempt-api)
 *
 * Locked decisions:
 *   - #76 anti-cheat: ulangan answer endpoint TIDAK return is_benar/jawaban_benar.
 *     Player tidak menampilkan feedback per jawaban; siswa baru tahu nilai post-submit.
 *   - #79 deterministic seed: resume bawa pool sama, refresh tidak shuffle ulang.
 *   - #80 cron auto-grade 30s tick + advisory lock submit/cron mutex.
 *   - #81 review gating: izinkan_review_setelah_submit + waktu_buka_review.
 *   - #76 cancelled tidak count terhadap batas_attempt.
 */

import { ApiError, api } from '@/lib/api';
import type { SoalJawaban, AnswerInput, HasilStatus, HasilMode } from '@/lib/soalbab-attempt-api';

// ---------- Types ----------

/** Trimmed siswa lobby payload — pool size NOT exposed. */
export interface SiswaLobbyView {
  bab_id: string;
  durasi_menit: number;
  batas_attempt: number;
  izinkan_review_setelah_submit: boolean;
  waktu_buka_review?: string | null;
  configured: boolean;
}

export interface HasilSummary {
  hasil_id: string;
  mode: HasilMode;
  status: HasilStatus;
  attempt_no: number;
  nilai_total?: number | null;
  jawaban_benar_count?: number | null;
  jawaban_total?: number | null;
  mulai_at: string;
  deadline_at?: string | null;
  selesai_at?: string | null;
}

export interface SiswaHasilListResult {
  bab_id: string;
  nilai_terbaik?: number | null;
  nilai_terakhir?: number | null;
  attempt_count: number; // ulangan-only counter (excluding dibatalkan)
  items: HasilSummary[];
}

export interface UlanganStartResult {
  hasil_id: string;
  soal_ids: string[];
  total: number;
  mulai_at: string;
  deadline_at: string;
  durasi_detik: number;
  attempt_no: number;
  batas_attempt: number;
  resume: boolean;
}

export interface UlanganSubmitResult {
  hasil_id: string;
  nilai_total: number;
  jawaban_benar_count: number;
  jawaban_total: number;
  selesai_at: string;
  dapat_review_at?: string | null;
  izinkan_review: boolean;
  already_submitted: boolean;
}

export interface ReviewItem {
  soal_id: string;
  pertanyaan: string;
  opsi_a: string;
  opsi_b: string;
  opsi_c: string;
  opsi_d: string;
  opsi_e: string;
  jawaban_benar: SoalJawaban;
  jawaban_siswa?: SoalJawaban | null;
  is_benar?: boolean | null;
  poin_dapat: number;
  poin_maksimal: number;
  urutan: number;
}

export interface ReviewResult {
  hasil_id: string;
  bab_id: string;
  mode: HasilMode;
  status: HasilStatus;
  attempt_no: number;
  nilai_total?: number | null;
  jawaban_benar_count?: number | null;
  jawaban_total?: number | null;
  mulai_at: string;
  selesai_at?: string | null;
  items: ReviewItem[];
}

// ---------- Functions ----------

export async function getSiswaUlanganLobby(
  babID: string,
): Promise<{ setting: SiswaLobbyView }> {
  return api<{ setting: SiswaLobbyView }>(`/siswa/bab/${babID}/ulangan-setting`);
}

export async function getSiswaHasilList(
  babID: string,
): Promise<{ hasil: SiswaHasilListResult }> {
  return api<{ hasil: SiswaHasilListResult }>(`/siswa/bab/${babID}/hasil`);
}

export async function startUlangan(
  babID: string,
): Promise<{ hasil: UlanganStartResult }> {
  return api<{ hasil: UlanganStartResult }>(
    `/siswa/bab/${babID}/ulangan/start`,
    { method: 'POST' },
  );
}

/**
 * Ulangan answer endpoint — server returns {ok: true} (locked #76,
 * no immediate feedback). Network/validation errors di-throw via ApiError.
 */
export async function postUlanganAnswer(
  hasilID: string,
  input: AnswerInput,
): Promise<{ ok: true }> {
  return api<{ ok: true }>(`/siswa/hasil-soal-bab/${hasilID}/answer`, {
    method: 'POST',
    body: input,
  });
}

export async function submitUlangan(
  hasilID: string,
): Promise<{ summary: UlanganSubmitResult }> {
  return api<{ summary: UlanganSubmitResult }>(
    `/siswa/hasil-soal-bab/${hasilID}/submit`,
    { method: 'POST' },
  );
}

export async function getReview(
  hasilID: string,
): Promise<{ review: ReviewResult }> {
  return api<{ review: ReviewResult }>(
    `/siswa/hasil-soal-bab/${hasilID}/review`,
  );
}

// ---------- Friendly error mapper ----------

export type UlanganAction =
  | 'lobby'
  | 'history'
  | 'start'
  | 'answer'
  | 'submit'
  | 'review';

export function friendlyUlanganError(err: ApiError, action: UlanganAction): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID tidak valid.';
    case 'invalid_body':
    case 'invalid_input':
      return err.message || 'Input tidak valid.';
    case 'invalid_jawaban':
      return 'Jawaban harus a, b, c, d, atau e.';
    case 'invalid_soal_id':
      return 'ID soal tidak valid.';
    case 'soal_not_in_pool':
      return 'Soal ini tidak termasuk dalam attempt yang sedang berlangsung.';
    case 'forbidden':
      return 'Lu tidak punya akses ke attempt ini.';
    case 'not_found':
      return 'Data tidak ditemukan.';
    case 'ulangan_setting_missing':
      return 'Guru belum mengaktifkan ulangan untuk bab ini. Tunggu dulu yaa.';
    case 'ulangan_pool_empty':
      return 'Belum ada soal mode ulangan di bab ini.';
    case 'ulangan_pool_insufficient':
      return 'Jumlah soal ulangan kurang dari setting. Minta guru tambah soal lagi.';
    case 'batas_attempt_exceeded':
      return 'Lu sudah mencapai batas attempt ulangan untuk bab ini.';
    case 'timer_expired':
      return 'Waktu ulangan sudah habis. Refresh untuk lihat hasilnya.';
    case 'submit_after_grace':
      return 'Waktu submit sudah lewat — sistem akan auto-grade attempt ini.';
    case 'already_submitted':
      return 'Ulangan sudah disubmit sebelumnya.';
    case 'hasil_already_finished':
      return 'Attempt sudah selesai.';
    case 'hasil_cancelled':
      return 'Attempt ini dibatalkan oleh guru. Mulai attempt baru.';
    case 'hasil_not_active':
      return 'Attempt sudah selesai atau dibatalkan. Buka halaman review.';
    case 'hasil_not_finished':
      return 'Attempt belum selesai; review baru muncul setelah submit.';
    case 'hasil_mode_invalid':
      return 'Attempt ini bukan mode yang sesuai.';
    case 'review_locked':
      return 'Review belum dibuka guru. Tunggu sampai waktu pembahasan.';
    case 'review_disabled':
      return 'Guru tidak mengaktifkan review untuk ulangan ini.';
    case 'bab_archived':
      return 'Bab sudah diarsipkan.';
    case 'kelas_archived':
      return 'Kelas sudah diarsipkan.';
    default:
      switch (action) {
        case 'lobby':
          return err.message || 'Gagal memuat info ulangan.';
        case 'history':
          return err.message || 'Gagal memuat riwayat attempt.';
        case 'start':
          return err.message || 'Gagal memulai ulangan.';
        case 'answer':
          return err.message || 'Gagal menyimpan jawaban.';
        case 'submit':
          return err.message || 'Gagal submit ulangan.';
        case 'review':
          return err.message || 'Gagal memuat review.';
        default:
          return err.message;
      }
  }
}
