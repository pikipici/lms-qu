/**
 * Kelas API client — types + functions for /api/v1/kelas.
 *
 * Backend contracts (commit 620594f):
 *   GET    /kelas                  → list (guru: own; admin: all). Query: page, page_size, include_archived.
 *   POST   /kelas                  → create (guru). Body: { nama, deskripsi?, sekolah_id? }.
 *   GET    /kelas/:id              → detail.
 *   PATCH  /kelas/:id              → partial update. Body: { version, nama, deskripsi? }. 409 on stale version.
 *   POST   /kelas/:id/archive      → archive (idempotent guard 409 already_archived).
 *   POST   /kelas/:id/duplicate    → duplicate (regenerates kode invite, version=1, drops archived_at).
 *
 * PATCH partial semantics: omit a field → leave unchanged.
 * Class-level bobot remains in responses for legacy compatibility only.
 */

import { api } from '@/lib/api';

export interface Kelas {
  id: string;
  nama: string;
  deskripsi: string;
  kode_invite: string;
  guru_id: string;
  sekolah_id?: string | null;
  sekolah_nama?: string;
  bobot_soal_ulangan: number;
  bobot_tugas: number;
  jumlah_murid?: number;
  version: number;
  archived_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface KelasListResponse {
  items: Kelas[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface CreateKelasInput {
  nama: string;
  deskripsi?: string;
  sekolah_id?: string;
  guru_id?: string;
}

export interface UpdateKelasInput {
  version: number;
  nama: string;
  /** Omit to leave unchanged. */
  deskripsi?: string;
}

export interface DuplicateKelasInput {
  new_nama?: string;
}

export async function listKelas(params: {
  page?: number;
  pageSize?: number;
  includeArchived?: boolean;
  sekolahId?: string;
}): Promise<KelasListResponse> {
  const q = new URLSearchParams();
  if (params.page) q.set('page', String(params.page));
  if (params.pageSize) q.set('page_size', String(params.pageSize));
  if (params.includeArchived) q.set('include_archived', 'true');
  if (params.sekolahId) q.set('sekolah_id', params.sekolahId);
  const qs = q.toString();
  return api<KelasListResponse>(`/kelas${qs ? `?${qs}` : ''}`);
}

export async function createKelas(input: CreateKelasInput): Promise<{ kelas: Kelas }> {
  return api<{ kelas: Kelas }>('/kelas', { method: 'POST', body: input });
}

export async function getKelas(id: string): Promise<{ kelas: Kelas }> {
  return api<{ kelas: Kelas }>(`/kelas/${id}`);
}

export async function updateKelas(
  id: string,
  input: UpdateKelasInput,
): Promise<{ kelas: Kelas }> {
  return api<{ kelas: Kelas }>(`/kelas/${id}`, { method: 'PATCH', body: input });
}

export async function archiveKelas(id: string): Promise<{ kelas: Kelas }> {
  return api<{ kelas: Kelas }>(`/kelas/${id}/archive`, { method: 'POST' });
}

export async function duplicateKelas(
  id: string,
  input: DuplicateKelasInput = {},
): Promise<{ kelas: Kelas }> {
  return api<{ kelas: Kelas }>(`/kelas/${id}/duplicate`, {
    method: 'POST',
    body: input,
  });
}

// ----- Enrollment list (Task 2.C.4) -----
//
// Backend: GET /api/v1/kelas/:id/enrollments
//   Query params: page, page_size, include_removed
//   Auth: guru-owner OR admin (canManage gate)
//   Removed enrollments hidden by default; include_removed=true is admin-side.

export type EnrollmentStatus = 'active' | 'removed';
export type EnrollmentJoinedVia = 'admin' | 'kode';

export interface EnrollmentItem {
  siswa_id: string;
  nama: string;
  email: string;
  status: EnrollmentStatus;
  joined_via: EnrollmentJoinedVia;
  joined_at: string;
}

export interface EnrollmentListResponse {
  items: EnrollmentItem[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export async function listKelasEnrollments(
  kelasID: string,
  params: { page?: number; pageSize?: number; includeRemoved?: boolean } = {},
): Promise<EnrollmentListResponse> {
  const q = new URLSearchParams();
  if (params.page) q.set('page', String(params.page));
  if (params.pageSize) q.set('page_size', String(params.pageSize));
  if (params.includeRemoved) q.set('include_removed', 'true');
  const qs = q.toString();
  return api<EnrollmentListResponse>(
    `/kelas/${kelasID}/enrollments${qs ? `?${qs}` : ''}`,
  );
}
