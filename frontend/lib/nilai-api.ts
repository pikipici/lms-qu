/**
 * Nilai API client — types + functions for /api/v1/siswa/* nilai endpoints
 * (Task 7.A.2 FE consumer of BE Task 7.A.1).
 *
 * Backend contracts (locked #89-#94):
 *   GET /siswa/kelas/:id/nilai
 *     → SiswaKelasNilaiResponse: kelas info, bab[] with breakdown ulangan/tugas,
 *       ulangan_harian[] aggregate, total_kelas (avg of non-null bab totals).
 *     403 forbidden: not enrolled / not siswa role.
 *     400 invalid_id: kelas id bukan UUID valid.
 *
 *   GET /siswa/nilai
 *     → SiswaListResponse: items[] = one card per active enrollment, with
 *       total_kelas + bab_count + ulangan_count.
 *     403 forbidden: bukan siswa role.
 *
 * NULL handling: pct nullable di breakdown items + nilai_* nullable di rows.
 * FE render "—" untuk null. Total kelas null kalau semua bab kosong.
 */

import { api } from '@/lib/api';

// ---------- Types (mirror backend internal/nilai/model.go) ----------

export interface BabBreakdownItem {
  pct: number | null;
  w: number;
}

export interface BabBreakdown {
  ulangan_bab: BabBreakdownItem;
  tugas: BabBreakdownItem;
}

export interface NilaiBabRow {
  bab_id: string;
  nomor: number;
  judul: string;
  nilai_ulangan_bab: number | null;
  nilai_tugas_bab: number | null;
  total: number | null;
  breakdown: BabBreakdown;
  jumlah_tugas: number;
  jumlah_tugas_dinilai: number;
  jumlah_soal_ulangan_bab: number;
  hasil_ulangan_id?: string;
}

export interface NilaiUjianRow {
  ujian_id: string;
  judul: string;
  nilai_terbaik: number | null;
  nilai_terakhir: number | null;
  attempt_count: number;
  hasil_id?: string;
}

export interface NilaiKelasInfo {
  id: string;
  nama: string;
  bobot_soal_ulangan: number;
  bobot_tugas: number;
}

export interface SiswaKelasNilaiResponse {
  kelas: NilaiKelasInfo;
  bab: NilaiBabRow[];
  ulangan_harian: NilaiUjianRow[];
  total_kelas: number | null;
}

export interface SiswaKelasSummary {
  kelas_id: string;
  kelas_nama: string;
  guru_nama: string;
  total_kelas: number | null;
  bab_count: number;
  ulangan_count: number;
}

export interface SiswaListResponse {
  items: SiswaKelasSummary[];
}

// ---------- Fetchers ----------

export async function getSiswaKelasNilai(
  kelasID: string,
): Promise<SiswaKelasNilaiResponse> {
  return api<SiswaKelasNilaiResponse>(`/siswa/kelas/${kelasID}/nilai`);
}

export async function listSiswaNilai(): Promise<SiswaListResponse> {
  return api<SiswaListResponse>('/siswa/nilai');
}

// ---------- Formatters ----------

/** Format nilai 0..100 sebagai 1-decimal string. NULL → "—". */
export function formatNilai(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—';
  if (Number.isInteger(n)) return String(n);
  return n.toFixed(1);
}

/** Hint label untuk breakdown weight: "bobot 60%" misal. */
export function bobotLabel(w: number): string {
  return `bobot ${w}%`;
}
