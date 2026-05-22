/**
 * Attempt API client — siswa-side latihan + ulangan flow (Task 5.G.1+5.G.2).
 *
 * Backend contracts:
 *   POST  /siswa/bab/:id/latihan/start       → start atau resume latihan
 *   POST  /siswa/bab/:id/ulangan/start       → start atau resume ulangan
 *   GET   /siswa/hasil-soal-bab/:id/items    → live attempt items (Task 5.G.1 BE)
 *   POST  /siswa/hasil-soal-bab/:id/answer   → upsert jawaban (latihan: immediate is_benar)
 *   POST  /siswa/hasil-soal-bab/:id/finish   → close latihan (status=selesai, no nilai)
 *   POST  /siswa/hasil-soal-bab/:id/submit   → close ulangan + auto-grade
 *   GET   /siswa/hasil-soal-bab/:id/review   → review jawaban (gated #81)
 *   GET   /siswa/bab/:id/hasil               → list hasil sendiri di bab
 *   GET   /siswa/bab/:id/ulangan-setting     → siswa lobby trimmed view
 *
 * Locked decisions:
 *   - #76 anti-cheat: items endpoint TIDAK return jawaban_benar.
 *     Latihan attempt aktif dapat surface is_benar (immediate feedback).
 *     Ulangan attempt aktif dapat is_benar=null sampai submit (delayed grade).
 *   - #62 image presign TTL 15m: refresh items query kalau perlu.
 *   - #81 review gating: ulangan review baru muncul setelah submit + setting.
 */

import { ApiError, api } from '@/lib/api';

// ---------- Types ----------

export type SoalMode = 'latihan' | 'ulangan' | 'keduanya';
export type SoalJawaban = 'a' | 'b' | 'c' | 'd' | 'e';
export type HasilMode = 'latihan' | 'ulangan';
export type HasilStatus = 'berlangsung' | 'selesai' | 'dibatalkan';
export type AttemptImageSlot = 'pertanyaan' | 'a' | 'b' | 'c' | 'd' | 'e';

export interface AttemptItemImage {
  slot: AttemptImageSlot;
  url: string;
  expires_at?: string;
}

export interface AttemptItem {
  soal_id: string;
  pertanyaan: string;
  opsi_a: string;
  opsi_b: string;
  opsi_c: string;
  opsi_d: string;
  opsi_e: string;
  mode: SoalMode;
  poin: number;
  urutan: number;
  jawaban_siswa?: SoalJawaban | null;
  /** Latihan only: surfaced setelah answer. Ulangan stays null sampai submit. */
  is_benar?: boolean | null;
  images?: AttemptItemImage[];
}

export interface AttemptItemsResult {
  hasil_id: string;
  bab_id: string;
  mode: HasilMode;
  status: HasilStatus;
  attempt_no: number;
  mulai_at: string;
  deadline_at?: string | null;
  total: number;
  items: AttemptItem[];
}

export interface StartLatihanResult {
  hasil_id: string;
  soal_ids: string[];
  total: number;
  mulai_at: string;
  resume: boolean;
  /** Pre-fill jawaban map (soal_id → letter) untuk resume. */
  jawaban?: Record<string, SoalJawaban>;
}

export interface AnswerResult {
  /** Latihan: immediate; Ulangan: false until submit. */
  is_benar: boolean;
  /** Latihan: surfaced. Ulangan: empty string sampai submit. */
  jawaban_benar: SoalJawaban | '';
  poin_dapat: number;
  jawaban_tersimpan: SoalJawaban;
}

export interface FinishResult {
  hasil_id: string;
  total: number;
  benar: number;
  salah: number;
  skip: number;
  status: HasilStatus;
}

// ---------- Functions ----------

export async function startLatihan(
  babID: string,
): Promise<{ hasil: StartLatihanResult }> {
  return api<{ hasil: StartLatihanResult }>(`/siswa/bab/${babID}/latihan/start`, {
    method: 'POST',
  });
}

export async function getAttemptItems(
  hasilID: string,
): Promise<{ attempt: AttemptItemsResult }> {
  return api<{ attempt: AttemptItemsResult }>(
    `/siswa/hasil-soal-bab/${hasilID}/items`,
  );
}

export interface AnswerInput {
  soal_id: string;
  jawaban: SoalJawaban;
}

export async function postAnswer(
  hasilID: string,
  input: AnswerInput,
): Promise<{ answer: AnswerResult }> {
  return api<{ answer: AnswerResult }>(`/siswa/hasil-soal-bab/${hasilID}/answer`, {
    method: 'POST',
    body: input,
  });
}

export async function finishLatihan(
  hasilID: string,
): Promise<{ summary: FinishResult }> {
  return api<{ summary: FinishResult }>(
    `/siswa/hasil-soal-bab/${hasilID}/finish`,
    { method: 'POST' },
  );
}

// ---------- Friendly error mapper ----------

export type AttemptAction = 'start' | 'items' | 'answer' | 'finish' | 'submit';

export function friendlyAttemptError(err: ApiError, action: AttemptAction): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID tidak valid.';
    case 'invalid_body':
    case 'invalid_input':
      return err.message || 'Input tidak valid.';
    case 'invalid_jawaban':
      return 'Jawaban harus salah satu dari a, b, c, d, atau e.';
    case 'invalid_soal_id':
      return 'ID soal tidak valid.';
    case 'soal_not_in_pool':
      return 'Soal ini tidak termasuk dalam attempt yang sedang berlangsung.';
    case 'forbidden':
      return 'Lu tidak punya akses ke attempt ini.';
    case 'not_found':
      return 'Data tidak ditemukan.';
    case 'latihan_pool_empty':
      return 'Belum ada soal latihan di bab ini. Tunggu guru menambahkan soal.';
    case 'ulangan_pool_empty':
      return 'Belum ada soal ulangan di bab ini.';
    case 'hasil_mode_invalid':
      return 'Attempt ini bukan mode yang sesuai dengan aksi.';
    case 'hasil_already_finished':
      return 'Attempt sudah selesai.';
    case 'hasil_cancelled':
      return 'Attempt ini sudah dibatalkan oleh guru. Mulai attempt baru.';
    case 'hasil_not_active':
      return 'Attempt sudah selesai atau dibatalkan. Buka halaman review.';
    case 'timer_expired':
      return 'Waktu ulangan sudah habis. Refresh untuk melihat hasil.';
    case 'bab_archived':
      return 'Bab sudah diarsipkan.';
    case 'kelas_archived':
      return 'Kelas sudah diarsipkan.';
    default:
      switch (action) {
        case 'start':
          return err.message || 'Gagal memulai attempt.';
        case 'items':
          return err.message || 'Gagal memuat soal.';
        case 'answer':
          return err.message || 'Gagal menyimpan jawaban.';
        case 'finish':
          return err.message || 'Gagal menyelesaikan attempt.';
        case 'submit':
          return err.message || 'Gagal submit ulangan.';
        default:
          return err.message;
      }
  }
}
