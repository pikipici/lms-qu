/**
 * Bab API client — types + functions for /api/v1/bab and /api/v1/kelas/:id/bab.
 *
 * Backend contracts:
 *   GET    /kelas/:id/bab               → list (guru: own; admin: all). Query: status, include_archived.
 *   POST   /kelas/:id/bab               → create (guru). Body: { nomor, judul, deskripsi? }.
 *   POST   /kelas/:id/bab/reorder       → bulk reorder (guru). Body: { order: [...], versions: { id: v } }.
 *   GET    /bab/:id                     → detail.
 *   PATCH  /bab/:id                     → partial update. Body: { version, nomor?, judul?, deskripsi?, urutan?, status? }.
 *   POST   /bab/:id/archive             → archive (idempotent guard 409 already_archived).
 *   POST   /bab/:id/duplicate           → duplicate (default suffix " (Salinan)", new bab status='draft').
 *
 * Backend commits referenced:
 *   - aafcfa4 — migration 000005_bab + GORM model + repo (Task 3.A.1)
 *   - 377eed8 — CRUD service + handler (Task 3.A.2)
 *   - 6b0f041 — bulk reorder endpoint (Task 3.A.3)
 *   - fcbf532 — duplicate endpoint (Task 3.A.4)
 *
 * PATCH partial semantics: omit a field → leave unchanged. `version` is
 * REQUIRED (#56 optimistic concurrency). 409 version_conflict → caller must
 * refetch + retry.
 *
 * Status enum drives lifecycle (Section 6.1 — no separate archived_at):
 * draft = invisible to siswa, published = visible, archived = hidden tapi
 * tetap exist. Un-archive bukan MVP — kalau perlu, hubungi backend.
 */

import { ApiError, api } from '@/lib/api';

export type BabStatus = 'draft' | 'published' | 'archived';

export interface Bab {
  id: string;
  kelas_id: string;
  nomor: number;
  judul: string;
  deskripsi: string;
  urutan: number;
  status: BabStatus;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface BabListResponse {
  items: Bab[];
  total: number;
}

export interface CreateBabInput {
  nomor: number;
  judul: string;
  deskripsi?: string;
}

export interface UpdateBabInput {
  version: number;
  nomor?: number;
  judul?: string;
  deskripsi?: string;
  urutan?: number;
  status?: BabStatus;
}

export interface ReorderInput {
  order: string[];
  versions: Record<string, number>;
}

export interface ReorderConflict {
  bab_id: string;
  current_version: number;
}

export interface ListBabOptions {
  status?: BabStatus;
  includeArchived?: boolean;
}

export async function listBab(
  kelasID: string,
  opts: ListBabOptions = {},
): Promise<BabListResponse> {
  const q = new URLSearchParams();
  if (opts.status) q.set('status', opts.status);
  if (opts.includeArchived) q.set('include_archived', 'true');
  const qs = q.toString();
  return api<BabListResponse>(`/kelas/${kelasID}/bab${qs ? `?${qs}` : ''}`);
}

export async function createBab(
  kelasID: string,
  input: CreateBabInput,
): Promise<{ bab: Bab }> {
  return api<{ bab: Bab }>(`/kelas/${kelasID}/bab`, {
    method: 'POST',
    body: input,
  });
}

export async function getBab(id: string): Promise<{ bab: Bab }> {
  return api<{ bab: Bab }>(`/bab/${id}`);
}

export async function updateBab(
  id: string,
  input: UpdateBabInput,
): Promise<{ bab: Bab }> {
  return api<{ bab: Bab }>(`/bab/${id}`, { method: 'PATCH', body: input });
}

export async function archiveBab(id: string): Promise<{ bab: Bab }> {
  return api<{ bab: Bab }>(`/bab/${id}/archive`, { method: 'POST' });
}

export async function duplicateBab(
  id: string,
  input: { judul?: string } = {},
): Promise<{ bab: Bab }> {
  return api<{ bab: Bab }>(`/bab/${id}/duplicate`, {
    method: 'POST',
    body: input,
  });
}

export async function reorderBab(
  kelasID: string,
  input: ReorderInput,
): Promise<BabListResponse> {
  return api<BabListResponse>(`/kelas/${kelasID}/bab/reorder`, {
    method: 'POST',
    body: input,
  });
}

export type BabAction = 'create' | 'update' | 'archive' | 'duplicate' | 'reorder';

/**
 * Friendly Indonesian error message untuk `ApiError` dari endpoint bab.
 *
 * Mirror pola `friendlyUpdateError` di guru/kelas/detail/page.tsx — caller
 * pakai ini untuk isi `description` di `useToast`. Default fallback ke
 * `err.message` apa adanya.
 */
export function friendlyBabError(err: ApiError, action: BabAction): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID bab tidak valid.';
    case 'invalid_body':
    case 'invalid_input':
      return 'Input tidak valid. Periksa kembali data yang lu kirim.';
    case 'invalid_status':
      return 'Status bab tidak valid.';
    case 'invalid_version':
      return 'Versi bab tidak valid. Refresh halaman dulu.';
    case 'version_conflict':
      return action === 'reorder'
        ? 'Susunan bab berubah barengan orang lain. Daftar sudah di-refresh — silakan ulangi geser-geser lu.'
        : 'Bab ini baru saja di-update orang lain. Form sudah di-refresh dengan data terbaru — silakan ulangi perubahan lu.';
    case 'forbidden':
      return 'Lu tidak punya akses ke bab ini.';
    case 'not_found':
      return 'Bab tidak ditemukan (mungkin sudah dihapus).';
    case 'already_archived':
      return 'Bab ini sudah diarsipkan.';
    case 'kelas_archived':
      return 'Kelas sudah diarsipkan; bab tidak bisa dibuat baru atau diubah.';
    case 'duplicate_in_order':
      return 'Susunan tidak valid: ada bab yang muncul lebih dari sekali.';
    case 'bab_not_in_kelas':
      return 'Salah satu bab di susunan bukan milik kelas ini.';
    case 'reorder_missing_bab':
      return 'Susunan tidak lengkap — refresh halaman dulu lalu coba ulang.';
    default:
      return err.message;
  }
}
