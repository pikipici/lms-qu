/**
 * Pengumuman API client — types + functions untuk endpoint /api/v1 yang
 * sudah di-ship oleh sub-fase 3.F BE (Task 3.F.1, commit `cf8c5bc`).
 *
 * Backend contracts (lihat backend/internal/pengumuman/handler.go):
 *
 *   POST   /kelas/:id/pengumuman               (guru/admin own kelas)
 *   GET    /kelas/:id/pengumuman?bab_id=&status=&limit=
 *   GET    /siswa/kelas/:id/pengumuman?bab_id=&limit=
 *   GET    /pengumuman/:id
 *   PATCH  /pengumuman/:id
 *   DELETE /pengumuman/:id
 *
 * bab_id query semantics:
 *   - absent       → no filter (semua di kelas)
 *   - 'null'/'none'→ pin bab_id IS NULL (kelas-wide pengumuman)
 *   - <uuid>       → pin bab_id = uuid (bab-scoped)
 *
 * status query (guru/admin only — siswa always forced to 'published'):
 *   - 'published' | 'archived' | absent (= all)
 *
 * Locked decisions referenced:
 *   - #56 optimistic concurrency: PATCH wajib `version`.
 *   - #66 Pengumuman passive timestamp: badge "Baru" client-side kalau
 *         created_at < 7 hari sejak now. No per-siswa read receipt.
 *   - #20 BabID nullable di backend (kelas-wide vs bab-scoped).
 */

import { ApiError, api } from '@/lib/api';

export type PengumumanStatus = 'published' | 'archived';

export interface Pengumuman {
  id: string;
  kelas_id: string;
  bab_id: string | null;
  judul: string;
  isi: string;
  created_by_id: string;
  status: PengumumanStatus;
  version: number;
  created_at: string;
  updated_at: string;
  attachment_object_key?: string | null;
  attachment_filename?: string | null;
  attachment_mime?: string | null;
  attachment_size?: number | null;
}

export interface PengumumanListResponse {
  items: Pengumuman[];
  total: number;
}

export interface PengumumanDetailResponse {
  pengumuman: Pengumuman;
}

export interface PengumumanDeleteResponse {
  pengumuman_id: string;
}

export interface PengumumanAttachmentURLResponse {
  url: string;
  expires_at: string;
}

export interface CreatePengumumanBody {
  bab_id?: string | null;
  judul: string;
  /** Markdown body — max 50KB (locked roadmap §3.F.1, mirror materi). */
  isi: string;
}

export interface UpdatePengumumanBody {
  version: number;
  judul?: string;
  isi?: string;
  status?: PengumumanStatus;
}

/**
 * BabFilter — caller picks one:
 *   - undefined: no filter (return all)
 *   - null:      pin bab_id IS NULL (kelas-wide)
 *   - string:    pin bab_id = uuid (bab-scoped)
 */
export interface ListPengumumanOptions {
  babID?: string | null;
  status?: PengumumanStatus;
  limit?: number;
}

/** Cap markdown body at 50KB sebelum kirim ke backend (mirror locked #63 materi). */
export const MAX_PENGUMUMAN_ISI_BYTES = 50 * 1024;

/** Cap judul (200 chars) — backend MaxJudulBytes. */
export const MAX_PENGUMUMAN_JUDUL_LENGTH = 200;

export const MAX_PENGUMUMAN_ATTACHMENT_BYTES = 20 * 1024 * 1024;

export const PENGUMUMAN_ATTACHMENT_ACCEPT = 'image/*,application/pdf';

/**
 * "Baru" badge threshold: 7 hari sejak created_at (locked #66).
 */
export const PENGUMUMAN_NEW_THRESHOLD_MS = 7 * 24 * 60 * 60 * 1000;

/** True kalau pengumuman dibuat dalam 7 hari terakhir. */
export function isPengumumanNew(
  pengumuman: Pick<Pengumuman, 'created_at'>,
  now: Date = new Date(),
): boolean {
  const created = new Date(pengumuman.created_at).getTime();
  if (Number.isNaN(created)) return false;
  return now.getTime() - created < PENGUMUMAN_NEW_THRESHOLD_MS;
}

function buildBabQuery(babID: string | null | undefined): string | null {
  if (babID === undefined) return null;
  if (babID === null) return 'null';
  if (typeof babID === 'string' && babID) return babID;
  return null;
}

/** Guru/admin list: GET /kelas/:id/pengumuman. */
export async function listPengumuman(
  kelasID: string,
  opts: ListPengumumanOptions = {},
): Promise<PengumumanListResponse> {
  const q = new URLSearchParams();
  const bab = buildBabQuery(opts.babID);
  if (bab !== null) q.set('bab_id', bab);
  if (opts.status) q.set('status', opts.status);
  if (typeof opts.limit === 'number' && opts.limit > 0) {
    q.set('limit', String(opts.limit));
  }
  const qs = q.toString();
  return api<PengumumanListResponse>(
    `/kelas/${kelasID}/pengumuman${qs ? `?${qs}` : ''}`,
  );
}

/** Siswa enrolled list: GET /siswa/kelas/:id/pengumuman (server forces status=published). */
export async function listSiswaPengumuman(
  kelasID: string,
  opts: Omit<ListPengumumanOptions, 'status'> = {},
): Promise<PengumumanListResponse> {
  const q = new URLSearchParams();
  const bab = buildBabQuery(opts.babID);
  if (bab !== null) q.set('bab_id', bab);
  if (typeof opts.limit === 'number' && opts.limit > 0) {
    q.set('limit', String(opts.limit));
  }
  const qs = q.toString();
  return api<PengumumanListResponse>(
    `/siswa/kelas/${kelasID}/pengumuman${qs ? `?${qs}` : ''}`,
  );
}

export async function getPengumuman(id: string): Promise<PengumumanDetailResponse> {
  return api<PengumumanDetailResponse>(`/pengumuman/${id}`);
}

export async function createPengumuman(
  kelasID: string,
  input: CreatePengumumanBody,
): Promise<PengumumanDetailResponse> {
  return api<PengumumanDetailResponse>(`/kelas/${kelasID}/pengumuman`, {
    method: 'POST',
    body: input,
  });
}

export async function updatePengumuman(
  id: string,
  input: UpdatePengumumanBody,
): Promise<PengumumanDetailResponse> {
  return api<PengumumanDetailResponse>(`/pengumuman/${id}`, {
    method: 'PATCH',
    body: input,
  });
}

export async function uploadPengumumanAttachment(
  id: string,
  file: File,
): Promise<PengumumanDetailResponse> {
  const form = new FormData();
  form.set('file', file);
  return api<PengumumanDetailResponse>(`/pengumuman/${id}/attachment`, {
    method: 'PUT',
    body: form,
  });
}

export async function deletePengumumanAttachment(
  id: string,
): Promise<PengumumanDetailResponse> {
  return api<PengumumanDetailResponse>(`/pengumuman/${id}/attachment`, {
    method: 'DELETE',
  });
}

export async function getPengumumanAttachmentURL(
  id: string,
): Promise<PengumumanAttachmentURLResponse> {
  return api<PengumumanAttachmentURLResponse>(`/pengumuman/${id}/attachment-url`);
}

export async function deletePengumuman(
  id: string,
): Promise<PengumumanDeleteResponse> {
  return api<PengumumanDeleteResponse>(`/pengumuman/${id}`, {
    method: 'DELETE',
  });
}

export type PengumumanAction =
  | 'create'
  | 'update'
  | 'archive'
  | 'delete'
  | 'attachment'
  | 'list'
  | 'get';

/**
 * Friendly Indonesian error untuk ApiError dari endpoint pengumuman.
 * Mirror pola friendlyMateriError. Caller pakai untuk isi description toast.
 */
export function friendlyPengumumanError(
  err: ApiError,
  action: PengumumanAction,
): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID pengumuman atau kelas tidak valid.';
    case 'invalid_body':
    case 'invalid_input':
      return 'Input tidak valid. Periksa kembali data yang lu kirim.';
    case 'invalid_status':
      return 'Status tidak valid. Pilih published atau archived.';
    case 'invalid_version':
      return 'Versi pengumuman tidak valid. Refresh halaman dulu.';
    case 'invalid_limit':
      return 'Limit harus angka positif.';
    case 'version_conflict':
      return 'Pengumuman ini baru saja di-update orang lain. Form sudah di-refresh — ulangi perubahan lu.';
    case 'forbidden':
      return action === 'list' || action === 'get'
        ? 'Lu tidak punya akses ke pengumuman kelas ini.'
        : 'Lu tidak punya akses untuk mengubah pengumuman ini.';
    case 'not_found':
      return action === 'create'
        ? 'Kelas tidak ditemukan (mungkin sudah dihapus).'
        : 'Pengumuman tidak ditemukan (mungkin sudah dihapus).';
    case 'kelas_archived':
      return 'Kelas sudah diarsipkan; pengumuman tidak bisa dibuat baru.';
    case 'bab_not_in_kelas':
      return 'Bab yang dipilih bukan milik kelas ini.';
    case 'payload_too_large':
      return action === 'attachment'
        ? `Lampiran terlalu besar. Batas ${MAX_PENGUMUMAN_ATTACHMENT_BYTES / 1024 / 1024} MB.`
        : `Konten terlalu panjang. Batas ${MAX_PENGUMUMAN_ISI_BYTES / 1024} KB.`;
    case 'unsupported_attachment':
      return 'Lampiran harus berupa gambar atau PDF.';
    case 'attachment_not_found':
      return 'Lampiran tidak ditemukan.';
    case 'storage_unavailable':
      return 'Penyimpanan lampiran belum tersedia. Coba lagi nanti.';
    default:
      return err.message;
  }
}
