/**
 * Siswa-side Ujian API client (Task 6.G.1 lobby + 6.G.2 player/review).
 *
 * Backend contracts dipakai di lobby (6.G.1):
 *   GET  /kelas/:id/ujian                       → list per kelas (BE
 *                                                  service-level role-branch
 *                                                  siswa: published-only).
 *   GET  /siswa/kelas/:id/ujian/hasil           → siswa hasil aggregate
 *                                                  (cross-ujian dalam kelas).
 *
 * Backend contracts dipakai di player+review (6.G.2):
 *   POST /siswa/ujian/:id/start                 → start atau resume attempt.
 *   GET  /siswa/hasil-ujian/:id/items           → live items (anti-cheat #76:
 *                                                  no jawaban_benar; presigned
 *                                                  image slots TTL 15m).
 *   POST /siswa/hasil-ujian/:id/answer          → autosave jawaban (delayed
 *                                                  grade; ulangan ga return
 *                                                  feedback per jawaban).
 *   POST /siswa/hasil-ujian/:id/submit          → submit + auto-grade tx
 *                                                  (idempotent; locked #87
 *                                                  pg_advisory_xact_lock).
 *   GET  /siswa/hasil-ujian/:id/review          → review payload (gated #81
 *                                                  izinkan_review_setelah_submit
 *                                                  + waktu_buka_review).
 *
 * Locked decisions referenced:
 *   - #76 anti-cheat ulangan: items strip jawaban_benar; answer endpoint
 *     ga return feedback per jawaban; siswa baru tahu nilai post-submit.
 *   - #81 review gating: izinkan_review_setelah_submit + waktu_buka_review.
 *   - #86 deterministic seed: resume bawa pool sama, refresh tidak
 *     shuffle ulang.
 *   - #87 timer expire cron 30s + advisory lock submit/cron mutex.
 */

import { ApiError, api } from '@/lib/api';
import type { Ujian, UjianListResponse } from '@/lib/ujian-api';

// ---------- Types ----------

export type SiswaUjianHasilStatus = 'berlangsung' | 'selesai' | 'dibatalkan';

/** Mirror backend HasilSummary (ujian/hasil.go). */
export interface UjianHasilSummary {
  hasil_id: string;
  ujian_id: string;
  status: SiswaUjianHasilStatus;
  attempt_no: number;
  nilai_total?: number | null;
  jawaban_benar_count?: number | null;
  jawaban_total?: number | null;
  mulai_at: string;
  deadline_at?: string | null;
  selesai_at?: string | null;
}

/** Mirror backend SiswaHasilListResult (ujian/hasil.go). */
export interface SiswaUjianHasilListResult {
  kelas_id: string;
  nilai_terbaik?: number | null;
  nilai_terakhir?: number | null;
  attempt_count: number; // hasil status='selesai' only (locked #76)
  items: UjianHasilSummary[];
}

// ---------- Player+Review (6.G.2 prep) ----------

export type UjianSoalJawaban = 'a' | 'b' | 'c' | 'd' | 'e';

/** ImageSlot mirrors banksoal item slot dengan presigned URL TTL 15m. */
export interface UjianSoalImageSlot {
  slot: 'pertanyaan' | 'opsi_a' | 'opsi_b' | 'opsi_c' | 'opsi_d' | 'opsi_e';
  url: string;
}

/** Live attempt item — anti-cheat: no jawaban_benar (locked #76). */
export interface UjianAttemptItem {
  soal_id: string;
  urutan: number;
  pertanyaan: string;
  opsi_a: string;
  opsi_b: string;
  opsi_c: string;
  opsi_d: string;
  opsi_e: string;
  poin: number;
  images?: UjianSoalImageSlot[];
  jawaban_siswa?: UjianSoalJawaban | null;
  // is_benar deliberately absent untuk ulangan (delayed grade #76).
}

/**
 * Live items payload — flat shape from BE Items handler (no envelope).
 * Mirror backend ItemsResult struct (ujian/items.go).
 */
export interface UjianAttemptItemsResult {
  hasil_id: string;
  ujian_id: string;
  status: SiswaUjianHasilStatus;
  attempt_no: number;
  mulai_at: string;
  deadline_at?: string | null;
  total: number;
  items: UjianAttemptItem[];
}

/**
 * Start payload — fields mirror backend StartResult.
 * Note: NO ujian_id wrap; soal_ids[] full pool snapshot.
 */
export interface UjianStartResult {
  hasil_id: string;
  ujian_id: string;
  soal_ids: string[];
  total: number;
  mulai_at: string;
  deadline_at: string;
  durasi_detik: number;
  attempt_no: number;
  resume: boolean;
}

export interface UjianAnswerInput {
  soal_id: string;
  jawaban: UjianSoalJawaban;
}

export interface UjianAnswerResult {
  ok: boolean;
  // ulangan: TIDAK return is_benar / jawaban_benar (locked #76)
}

/**
 * Submit payload — fields mirror backend SubmitResult.
 * Wrapped sebagai {"summary": ...} di response.
 */
export interface UjianSubmitResult {
  hasil_id: string;
  nilai_total: number;
  jawaban_benar_count: number;
  jawaban_total: number;
  selesai_at: string;
  dapat_review_at?: string | null;
  izinkan_review: boolean;
  already_submitted: boolean;
}

export interface UjianReviewItem {
  soal_id: string;
  pertanyaan: string;
  opsi_a: string;
  opsi_b: string;
  opsi_c: string;
  opsi_d: string;
  opsi_e: string;
  jawaban_benar: UjianSoalJawaban;
  jawaban_siswa?: UjianSoalJawaban | null;
  is_benar?: boolean | null;
  poin_dapat: number;
  poin_maksimal: number;
  urutan: number;
}

export interface UjianReviewResult {
  hasil_id: string;
  ujian_id: string;
  status: SiswaUjianHasilStatus;
  attempt_no: number;
  nilai_total?: number | null;
  jawaban_benar_count?: number | null;
  jawaban_total?: number | null;
  mulai_at: string;
  selesai_at?: string | null;
  items: UjianReviewItem[];
}

// ---------- Functions ----------

/**
 * List ujian per kelas dari sisi siswa. BE service-level role-branch
 * (callerRole=siswa) auto-filter status=published only — siswa tidak
 * lihat draft/archived. Endpoint: /siswa/kelas/:id/ujian (Task 6.G.1
 * route, separate dari guru kelasGroup yang admin/guru-only).
 */
export async function listSiswaUjianByKelas(
  kelasID: string,
  opts: { limit?: number; offset?: number } = {},
): Promise<UjianListResponse> {
  const q = new URLSearchParams();
  if (typeof opts.limit === 'number' && opts.limit > 0) {
    q.set('limit', String(opts.limit));
  }
  if (typeof opts.offset === 'number' && opts.offset > 0) {
    q.set('offset', String(opts.offset));
  }
  const qs = q.toString();
  return api<UjianListResponse>(
    `/siswa/kelas/${kelasID}/ujian${qs ? `?${qs}` : ''}`,
  );
}

/**
 * List hasil ujian milik caller di kelas (cross-ujian aggregate). Buat
 * lobby/history. attempt_count hanya count status='selesai' (locked #76:
 * dibatalkan tidak count).
 */
export async function listSiswaUjianHasil(
  kelasID: string,
): Promise<{ hasil: SiswaUjianHasilListResult }> {
  return api<{ hasil: SiswaUjianHasilListResult }>(
    `/siswa/kelas/${kelasID}/ujian/hasil`,
  );
}

// 6.G.2 functions (player + review) — sudah typed, tinggal dipakai di
// stage berikutnya. Cuma siswa-side; guru/admin pakai ujian-api.ts.

export async function startSiswaUjian(
  ujianID: string,
): Promise<{ hasil: UjianStartResult }> {
  return api<{ hasil: UjianStartResult }>(`/siswa/ujian/${ujianID}/start`, {
    method: 'POST',
    body: {},
  });
}

/**
 * BE handler returns flat ItemsResult (no envelope) — distinct dari Start
 * yang wrapped {"hasil": ...}. Mirror SoalBab pattern.
 */
export async function getSiswaUjianItems(
  hasilID: string,
): Promise<UjianAttemptItemsResult> {
  return api<UjianAttemptItemsResult>(
    `/siswa/hasil-ujian/${hasilID}/items`,
  );
}

export async function postSiswaUjianAnswer(
  hasilID: string,
  input: UjianAnswerInput,
): Promise<UjianAnswerResult> {
  return api<UjianAnswerResult>(`/siswa/hasil-ujian/${hasilID}/answer`, {
    method: 'POST',
    body: input,
  });
}

/** BE wraps as {"summary": SubmitResult}. */
export async function submitSiswaUjian(
  hasilID: string,
): Promise<{ summary: UjianSubmitResult }> {
  return api<{ summary: UjianSubmitResult }>(
    `/siswa/hasil-ujian/${hasilID}/submit`,
    { method: 'POST', body: {} },
  );
}

export async function getSiswaUjianReview(
  hasilID: string,
): Promise<{ review: UjianReviewResult }> {
  return api<{ review: UjianReviewResult }>(
    `/siswa/hasil-ujian/${hasilID}/review`,
  );
}

// ---------- Friendly error mapper ----------

export type SiswaUjianAction =
  | 'list'
  | 'hasil'
  | 'start'
  | 'items'
  | 'answer'
  | 'submit'
  | 'review';

export function friendlySiswaUjianError(
  err: ApiError,
  action: SiswaUjianAction,
): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID ujian tidak valid.';
    case 'invalid_input':
    case 'invalid_body':
      return err.message || 'Input tidak valid.';
    case 'forbidden':
      return 'Lu tidak punya akses ke ujian ini.';
    case 'not_found':
      return 'Ujian atau attempt tidak ditemukan.';
    case 'kelas_archived':
      return 'Kelas sudah diarsipkan; ujian tidak bisa dimulai.';
    case 'ujian_not_published':
      return 'Ujian belum dipublish guru.';
    case 'ujian_not_started':
      return 'Ujian belum dibuka. Tunggu sampai waktu mulai tiba.';
    case 'ujian_window_closed':
      return 'Jendela waktu ujian sudah lewat.';
    case 'ujian_archived':
      return 'Ujian sudah diarsipkan; tidak bisa dimulai lagi.';
    case 'source_pool_empty':
      return 'Pool soal kosong. Hubungi guru untuk fix konfigurasi sumber soal.';
    case 'soal_not_in_pool':
      return 'Soal yang lu jawab tidak ada di pool attempt ini.';
    case 'hasil_not_owned':
      return 'Attempt ini bukan milik lu.';
    case 'hasil_not_active':
      return 'Attempt sudah selesai atau dibatalkan.';
    case 'hasil_not_finished':
      return 'Attempt belum selesai; review belum tersedia.';
    case 'hasil_already_finished':
      return 'Attempt sudah selesai.';
    case 'hasil_already_cancelled':
      return 'Attempt sudah dibatalkan.';
    case 'ujian_timer_expired':
    case 'timer_expired':
      return 'Waktu ujian sudah habis. Refresh untuk lihat hasil.';
    case 'submit_after_grace':
      return 'Submit ditolak karena lewat batas waktu. Cron akan auto-grade attempt ini.';
    case 'review_locked':
      return 'Pembahasan belum dibuka guru.';
    case 'review_disabled':
      return 'Guru tidak mengaktifkan pembahasan untuk ujian ini.';
    case 'rate_limited':
      return 'Terlalu banyak request. Coba lagi sebentar.';
    default:
      switch (action) {
        case 'list':
          return err.message || 'Gagal memuat daftar ujian.';
        case 'hasil':
          return err.message || 'Gagal memuat riwayat ujian.';
        case 'start':
          return err.message || 'Gagal memulai ujian.';
        case 'items':
          return err.message || 'Gagal memuat soal.';
        case 'answer':
          return err.message || 'Gagal menyimpan jawaban.';
        case 'submit':
          return err.message || 'Gagal submit ujian.';
        case 'review':
          return err.message || 'Gagal memuat pembahasan.';
        default:
          return err.message;
      }
  }
}

// Re-export Ujian buat downstream consumers convenience.
export type { Ujian };
