/**
 * Siswa-bab API client — types + functions untuk endpoint siswa-only yang
 * di-ship oleh sub-fase 3.E.1 BE (commit c0d795a, package siswabab).
 *
 * Backend contracts (lihat backend/internal/siswabab/student.go):
 *
 *   GET /api/v1/siswa/kelas/:id/bab → list bab status='published' di kelas yang
 *       siswa enroll, dengan progress per bab. 200 → { items, total }
 *   GET /api/v1/siswa/bab/:id → detail bab + materi list dgn read state.
 *       200 → SiswaBabDetail
 *
 * Authorization (server-side):
 *   - Caller MUST be siswa role (RoleGuard middleware).
 *   - Siswa MUST punya enrollment aktif di kelas (assertEnrollment).
 *   - Bab MUST status='published' (siswa scope; draft/archived → 404).
 *
 * Progress (Fase 3 partial, locked #68 + Section 6.4):
 *   - Persen = materi_read / materi_total × 100, rounded 2 dp.
 *   - materi_total=0 → bab_kosong=true + persen=0.
 *   - Komponen latihan/ulangan/tugas land later (Fase 4-7) — saat ini
 *     pct=null + w=0 di breakdown supaya FE bisa skip render.
 */

import { api } from '@/lib/api';

export type SiswaBabStatus = 'draft' | 'published' | 'archived';

/** Per-component progress entry. Pct null + w=0 → komponen belum ship. */
export interface SiswaBabBreakdownItem {
  pct: number | null;
  w: number;
}

export interface SiswaBabProgress {
  persen: number;
  breakdown: Record<string, SiswaBabBreakdownItem>;
  bab_kosong: boolean;
  materi_read: number;
  materi_total: number;
}

export interface SiswaBabItem {
  id: string;
  nomor: number;
  judul: string;
  deskripsi: string;
  urutan: number;
  status: SiswaBabStatus;
  progress: SiswaBabProgress;
}

export type SiswaMateriTipe = 'pdf' | 'youtube' | 'markdown';

export interface SiswaMateriCard {
  id: string;
  bab_id: string | null;
  judul: string;
  tipe: SiswaMateriTipe;
  /**
   * - youtube: 11-char video_id.
   * - markdown: konten markdown lengkap (≤ 50KB).
   * - pdf: kosong; download via presigned URL endpoint.
   */
  konten: string;
  urutan: number;
  sudah_dibaca: boolean;
}

export interface SiswaBabListResponse {
  items: SiswaBabItem[];
  total: number;
}

export interface SiswaBabDetailResponse {
  bab: SiswaBabItem;
  materi: SiswaMateriCard[];
}

/**
 * GET /siswa/kelas/:id/bab — list bab published di kelas siswa.
 * 403 forbidden  → caller bukan siswa atau tidak enroll
 * 400 invalid_id → kelas id bukan UUID
 */
export async function listSiswaBab(
  kelasID: string,
): Promise<SiswaBabListResponse> {
  return api<SiswaBabListResponse>(`/siswa/kelas/${kelasID}/bab`);
}

/**
 * GET /siswa/bab/:id — detail bab + materi list dengan read state.
 * 403 forbidden  → caller bukan siswa atau tidak enroll di kelas bab
 * 404 not_found  → bab missing OR status≠published
 * 400 invalid_id → bab id bukan UUID
 */
export async function getSiswaBab(
  babID: string,
): Promise<SiswaBabDetailResponse> {
  return api<SiswaBabDetailResponse>(`/siswa/bab/${babID}`);
}
