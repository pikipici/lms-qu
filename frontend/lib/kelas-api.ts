/**
 * Kelas API client — types + functions for /api/v1/kelas.
 *
 * Backend contracts (commit 620594f):
 *   GET    /kelas                  → list (guru: own; admin: all). Query: page, page_size, include_archived.
 *   POST   /kelas                  → create (guru). Body: { nama, deskripsi?, bobot_soal_ulangan?, bobot_tugas? }.
 *   GET    /kelas/:id              → detail.
 *   PATCH  /kelas/:id              → partial update. Body: { version, nama, deskripsi?, bobot_*? }. 409 on stale version.
 *   POST   /kelas/:id/archive      → archive (idempotent guard 409 already_archived).
 *   POST   /kelas/:id/duplicate    → duplicate (regenerates kode invite, version=1, drops archived_at).
 *
 * PATCH partial semantics (commit 620594f): omit a field → leave unchanged.
 * Sending bobot requires both halves to keep total = 100, else 400 invalid_bobot.
 */

import { api } from '@/lib/api';

export interface Kelas {
  id: string;
  nama: string;
  deskripsi: string;
  kode_invite: string;
  guru_id: string;
  bobot_soal_ulangan: number;
  bobot_tugas: number;
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
  bobot_soal_ulangan?: number;
  bobot_tugas?: number;
}

export interface UpdateKelasInput {
  version: number;
  nama: string;
  /** Omit to leave unchanged. */
  deskripsi?: string;
  /** Pair w/ bobot_tugas — backend requires sum = 100. Omit to leave both unchanged. */
  bobot_soal_ulangan?: number;
  bobot_tugas?: number;
}

export interface DuplicateKelasInput {
  new_nama?: string;
}

export async function listKelas(params: {
  page?: number;
  pageSize?: number;
  includeArchived?: boolean;
}): Promise<KelasListResponse> {
  const q = new URLSearchParams();
  if (params.page) q.set('page', String(params.page));
  if (params.pageSize) q.set('page_size', String(params.pageSize));
  if (params.includeArchived) q.set('include_archived', 'true');
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
