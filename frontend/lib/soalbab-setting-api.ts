/**
 * UlanganBabSetting API client — GET/PUT /api/v1/bab/:id/ulangan-setting
 * (Fase 5 Task 5.F.2).
 *
 * Backend contracts:
 *   GET /bab/:id/ulangan-setting → SettingView (guru/admin) atau
 *                                  SiswaLobbyView (siswa). FE Guru selalu
 *                                  pakai SettingView shape.
 *   PUT /bab/:id/ulangan-setting → upsert. Body:
 *     { jumlah_soal, durasi_menit, batas_attempt,
 *       izinkan_review_setelah_submit, waktu_buka_review?, version }
 *
 * Bounds (locked #74):
 *   jumlah_soal:   1-200, harus ≤ pool_size
 *   durasi_menit:  1-360
 *   batas_attempt: 1-10
 *   waktu_buka_review: optional RFC3339 atau null/empty (clear)
 *
 * Optimistic concurrency:
 *   Upsert pertama (belum pernah ada row) → version = 0
 *   Update existing row → version = current view.version
 *   Backend 409 version_conflict → caller refetch + retry.
 */

import { ApiError, api } from '@/lib/api';

// ---------- Types ----------

export interface SettingView {
  bab_id: string;
  jumlah_soal: number;
  durasi_menit: number;
  batas_attempt: number;
  attempt_unlimited: boolean;
  izinkan_review_setelah_submit: boolean;
  waktu_buka_review?: string | null;
  version: number;
  created_at?: string;
  updated_at?: string;
  pool_size: number;
  configured: boolean;
}

export interface UpsertSettingInput {
  jumlah_soal: number;
  durasi_menit: number;
  batas_attempt: number;
  attempt_unlimited: boolean;
  izinkan_review_setelah_submit: boolean;
  /** RFC3339 string. Empty/null clears the field. */
  waktu_buka_review?: string | null;
  /** 0 untuk first insert, ≥1 untuk update. */
  version: number;
}

// ---------- Functions ----------

export async function getUlanganSetting(babID: string): Promise<{ setting: SettingView }> {
  return api<{ setting: SettingView }>(`/bab/${babID}/ulangan-setting`);
}

export async function upsertUlanganSetting(
  babID: string,
  input: UpsertSettingInput,
): Promise<{ setting: SettingView }> {
  return api<{ setting: SettingView }>(`/bab/${babID}/ulangan-setting`, {
    method: 'PUT',
    body: input,
  });
}

// ---------- Friendly error mapper ----------

export function friendlySettingError(err: ApiError): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID bab tidak valid.';
    case 'invalid_body':
    case 'invalid_input':
      return err.message || 'Data setting tidak valid. Periksa kembali angka yang lu masukkan.';
    case 'invalid_waktu_buka_review':
      return 'Waktu buka review harus dalam format yang valid.';
    case 'jumlah_soal_exceeds_pool':
      return 'Jumlah soal melebihi jumlah soal ulangan yang tersedia di bab ini.';
    case 'ulangan_pool_empty':
      return 'Belum ada soal mode ulangan di bab ini. Tambahkan soal dulu sebelum mengaktifkan setting.';
    case 'forbidden':
      return 'Lu tidak punya akses untuk mengubah setting bab ini.';
    case 'not_found':
      return 'Bab tidak ditemukan.';
    case 'bab_archived':
      return 'Bab sudah diarsipkan; setting tidak bisa diubah.';
    case 'version_conflict':
      return 'Setting baru saja di-update orang lain. Form sudah di-refresh dengan data terbaru — silakan ulangi perubahan lu.';
    default:
      return err.message || 'Gagal menyimpan setting ulangan.';
  }
}
