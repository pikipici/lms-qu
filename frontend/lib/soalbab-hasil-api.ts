/**
 * Hasil + Rekap + Cancel API client — Fase 5 Task 5.F.2 (FE Guru).
 *
 * Backend contracts (Task 5.E.1):
 *   GET  /bab/:id/hasil-rekap          → guru/admin only. Per-siswa aggregate.
 *   POST /hasil-soal-bab/:id/cancel    → guru/admin only. Soft-cancel ulangan attempt.
 *                                        Idempotent. Mode=latihan → 400 cancel_latihan.
 *
 *   GET  /siswa/hasil-soal-bab/:id/review  → siswa-only review (tidak dipakai di FE Guru).
 *   GET  /siswa/bab/:id/hasil              → siswa list hasil sendiri (tidak dipakai di FE Guru).
 *
 * Locked decisions:
 *   - #76: Cancel hanya untuk mode=ulangan, attempt yang dibatalkan tidak count
 *     attempt_no berikutnya. Backend handle increment.
 *   - Sort rekap: nilai_terbaik DESC nulls last, lalu siswa_name ASC.
 *   - Rata-rata kelas dihitung dari nilai_terbaik per siswa (skip nil).
 */

import { ApiError, api } from '@/lib/api';
import type { HasilStatus } from '@/lib/soalbab-attempt-api';

// ---------- Types ----------

export type { HasilStatus };

export interface SiswaRekap {
  siswa_id: string;
  siswa_name: string;
  siswa_email: string;
  rombel_id?: string | null;
  rombel_nama?: string;
  attempt_count: number; // excluding dibatalkan
  cancelled_count: number;
  nilai_terbaik?: number | null;
  nilai_terakhir?: number | null;
  status_terakhir?: string | null;
  mulai_terakhir_at?: string | null;
  hasil_terakhir_id?: string | null;
}

export interface RekapResult {
  bab_id: string;
  total: number;
  rata_rata?: number | null;
  items: SiswaRekap[];
}

export interface CancelResult {
  hasil_id: string;
  bab_id: string;
  siswa_id: string;
  status: HasilStatus;
  attempt_no: number;
  cancelled_at: string;
}

// ---------- Functions ----------

export async function getHasilRekap(babID: string): Promise<RekapResult> {
  return api<RekapResult>(`/bab/${babID}/hasil-rekap`);
}

export async function cancelHasilAttempt(
  hasilID: string,
): Promise<{ hasil: CancelResult }> {
  return api<{ hasil: CancelResult }>(`/hasil-soal-bab/${hasilID}/cancel`, {
    method: 'POST',
  });
}

// ---------- Friendly error mapper ----------

export function friendlyHasilError(err: ApiError): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID tidak valid.';
    case 'forbidden':
      return 'Kamu tidak punya akses untuk operasi ini.';
    case 'not_found':
      return 'Data hasil tidak ditemukan.';
    case 'cancel_latihan':
      return 'Latihan tidak punya nilai persist; tidak ada attempt yang perlu di-reset.';
    case 'already_cancelled':
      return 'Attempt ini sudah dibatalkan sebelumnya.';
    case 'bab_archived':
      return 'Bab sudah diarsipkan.';
    case 'kelas_archived':
      return 'Kelas sudah diarsipkan.';
    default:
      return err.message || 'Operasi gagal. Coba lagi.';
  }
}
