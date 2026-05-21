/**
 * Materi API client — types + functions untuk endpoint /api/v1 yang sudah
 * di-ship oleh sub-fase 3.C BE (Task 3.C.1 → 3.C.4).
 *
 * Backend contracts (lihat backend/internal/materi/handler.go +
 * backend/internal/materi/upload.go + backend/internal/materi/read.go):
 *
 *   GET    /kelas/:id/materi[?bab_id=<uuid|null>]   list materi (guru/admin)
 *   POST   /kelas/:id/materi                        create youtube/markdown
 *   POST   /kelas/:id/materi/upload                 multipart PDF upload
 *   GET    /materi/:id                              detail
 *   GET    /materi/:id/file-url                     presigned R2 GET (PDF only)
 *   PATCH  /materi/:id                              partial update + version
 *   DELETE /materi/:id                              delete + R2 cleanup signal
 *   POST   /siswa/materi/:id/read                   siswa mark-as-read (idempotent)
 *
 * Locked decisions referenced:
 *   - #46 file upload hardening (mime sniff server-side, 20MB cap PDF)
 *   - #56 optimistic concurrency: PATCH wajib `version`
 *   - #62 upload flow: client → backend → R2 (no direct browser→R2)
 *   - #62 download access: presigned GET URL TTL 15 menit
 *   - #63 tipe enum 3 modes (pdf | youtube | markdown)
 *   - #64 PDF cap 20MB
 *   - #65 YouTube store hanya video_id 11-char
 */

import { ApiError, API_BASE, api } from '@/lib/api';
import { useAuthStore } from '@/lib/auth';

export type MateriTipe = 'pdf' | 'youtube' | 'markdown';

export interface Materi {
  id: string;
  kelas_id: string;
  bab_id: string | null;
  judul: string;
  tipe: MateriTipe;
  konten: string;
  object_key?: string | null;
  original_filename?: string | null;
  mime_type?: string | null;
  size_bytes?: number | null;
  urutan: number;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface MateriListResponse {
  items: Materi[];
  total: number;
}

export interface MateriDetailResponse {
  materi: Materi;
}

export interface CreateMateriBody {
  bab_id?: string | null;
  judul: string;
  tipe: 'youtube' | 'markdown';
  /**
   * Untuk youtube: URL apapun yang valid (backend parse → video_id).
   * Untuk markdown: body markdown mentah (max 50KB locked #63).
   */
  konten: string;
}

export interface UpdateMateriBody {
  version: number;
  judul?: string;
  konten?: string;
  urutan?: number;
}

export interface UploadMateriResponse {
  materi: Materi;
  object_key: string;
  original_filename: string;
  size_bytes: number;
}

export interface FileURLResponse {
  url: string;
  expires_at: string;
  original_filename: string;
  mime_type: string;
}

export interface DeleteMateriResponse {
  materi_id: string;
  tipe: MateriTipe;
  object_key?: string;
  pending_r2_cleanup?: boolean;
}

export interface MarkReadResponse {
  materi_id: string;
  read_at: string;
  was_new: boolean;
}

export interface ListMateriOptions {
  /**
   * UUID untuk pin bab tertentu, atau string `'null'` untuk pin
   * `bab_id IS NULL` (materi berdiri bebas). Undefined = no filter.
   */
  babID?: string | null;
}

/** Constant cap (locked #64) untuk validasi FE-side sebelum upload. */
export const MAX_PDF_BYTES = 20 * 1024 * 1024;

/** Constant cap (locked #63) untuk markdown body — sebelum kirim ke backend. */
export const MAX_MARKDOWN_BYTES = 50 * 1024;

export async function listMateri(
  kelasID: string,
  opts: ListMateriOptions = {},
): Promise<MateriListResponse> {
  const q = new URLSearchParams();
  if (opts.babID === null) {
    q.set('bab_id', 'null');
  } else if (typeof opts.babID === 'string' && opts.babID) {
    q.set('bab_id', opts.babID);
  }
  const qs = q.toString();
  return api<MateriListResponse>(
    `/kelas/${kelasID}/materi${qs ? `?${qs}` : ''}`,
  );
}

export async function getMateri(id: string): Promise<MateriDetailResponse> {
  return api<MateriDetailResponse>(`/materi/${id}`);
}

export async function createMateri(
  kelasID: string,
  input: CreateMateriBody,
): Promise<MateriDetailResponse> {
  return api<MateriDetailResponse>(`/kelas/${kelasID}/materi`, {
    method: 'POST',
    body: input,
  });
}

export async function updateMateri(
  id: string,
  input: UpdateMateriBody,
): Promise<MateriDetailResponse> {
  return api<MateriDetailResponse>(`/materi/${id}`, {
    method: 'PATCH',
    body: input,
  });
}

export async function deleteMateri(id: string): Promise<DeleteMateriResponse> {
  return api<DeleteMateriResponse>(`/materi/${id}`, { method: 'DELETE' });
}

/**
 * Presigned R2 GET URL untuk PDF materi. TTL ~15 menit (locked #62).
 * Dipanggil dari `<PdfViewer>` di Task 3.D.2 — di Task 3.D.1 dipakai
 * untuk tombol "Buka PDF" di list guru kalau perlu preview.
 */
export async function getMateriFileURL(id: string): Promise<FileURLResponse> {
  return api<FileURLResponse>(`/materi/${id}/file-url`);
}

/**
 * Siswa mark-as-read untuk materi. Idempotent — server pakai
 * ON CONFLICT DO NOTHING (locked #25). First call → was_new=true,
 * subsequent calls → was_new=false (read_at preserved dari original).
 *
 * Endpoint terkunci di siswaGroup BearerAuth + ForceChangePassword +
 * RoleGuard(siswa) di backend, plus enrollment guard di service. Caller
 * di FE harus siswa yang enrolled di kelas materi tsb.
 */
export async function markMateriRead(id: string): Promise<MarkReadResponse> {
  return api<MarkReadResponse>(`/siswa/materi/${id}/read`, {
    method: 'POST',
  });
}

/**
 * Multipart upload untuk PDF (locked #62: client → backend → R2).
 *
 * Tidak pakai `api()` karena `api()` set Content-Type JSON. Multipart
 * butuh browser yang set Content-Type: multipart/form-data dengan
 * boundary, jadi hand-roll fetch + bearer header. Pola sama dengan
 * `uploadImportCSV` di import-api.ts.
 */
export async function uploadMateriPDF(input: {
  kelasID: string;
  babID?: string | null;
  judul: string;
  file: File;
}): Promise<UploadMateriResponse> {
  const fd = new FormData();
  fd.append('file', input.file);
  fd.append('judul', input.judul);
  if (input.babID) fd.append('bab_id', input.babID);

  const token = useAuthStore.getState().access;
  const headers = new Headers();
  if (token) headers.set('Authorization', `Bearer ${token}`);

  const res = await fetch(
    `${API_BASE}/kelas/${input.kelasID}/materi/upload`,
    {
      method: 'POST',
      headers,
      body: fd,
    },
  );

  const requestId = res.headers.get('X-Request-ID') ?? undefined;
  const contentType = res.headers.get('Content-Type') ?? '';
  const payload = contentType.includes('application/json')
    ? await res.json().catch(() => null)
    : await res.text().catch(() => null);

  if (!res.ok) {
    const message =
      (payload && typeof payload === 'object' && 'error' in payload
        ? String((payload as { error: unknown }).error)
        : null) ?? `Upload failed (${res.status})`;
    const code =
      (payload && typeof payload === 'object' && 'code' in payload
        ? String((payload as { code: unknown }).code)
        : null) ?? 'unknown';
    throw new ApiError({
      status: res.status,
      code,
      message,
      requestId,
      details: payload,
    });
  }
  return payload as UploadMateriResponse;
}

export type MateriAction =
  | 'create'
  | 'upload'
  | 'update'
  | 'delete'
  | 'file-url';

/**
 * Friendly Indonesian error message untuk `ApiError` dari endpoint
 * materi. Mirror pola `friendlyBabError`. Caller pakai ini untuk isi
 * `description` di `useToast`.
 */
export function friendlyMateriError(
  err: ApiError,
  action: MateriAction,
): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID materi atau bab tidak valid.';
    case 'invalid_body':
    case 'invalid_input':
      return 'Input tidak valid. Periksa kembali data yang lu kirim.';
    case 'invalid_tipe':
      return 'Tipe materi tidak valid.';
    case 'invalid_version':
      return 'Versi materi tidak valid. Refresh halaman dulu.';
    case 'tipe_unsupported':
      return action === 'create'
        ? 'Untuk PDF, gunakan tab Upload PDF (multipart) — tidak bisa via JSON.'
        : 'Aksi ini tidak didukung untuk tipe materi tsb.';
    case 'tipe_immutable':
      return 'Tipe materi tidak bisa diubah. Hapus lalu buat ulang dengan tipe baru.';
    case 'version_conflict':
      return 'Materi ini baru saja di-update orang lain. Form sudah di-refresh — ulangi perubahan lu.';
    case 'forbidden':
      return 'Lu tidak punya akses ke materi/kelas ini.';
    case 'not_found':
      return 'Materi tidak ditemukan (mungkin sudah dihapus).';
    case 'kelas_archived':
      return 'Kelas sudah diarsipkan; materi tidak bisa dibuat baru atau diubah.';
    case 'bab_not_in_kelas':
      return 'Bab yang dipilih bukan milik kelas ini.';
    case 'unsupported_mime':
      return 'File harus PDF (application/pdf).';
    case 'payload_too_large':
      return 'File terlalu besar. Batas 20 MB per PDF.';
    case 'missing_file':
      return 'File belum dipilih.';
    case 'open_failed':
    case 'read_failed':
      return 'Gagal membaca file yang lu unggah.';
    case 'r2_put_failed':
      return 'Upload ke object store gagal. Coba ulangi sebentar lagi.';
    case 'r2_unavailable':
      return 'Object store tidak tersedia saat ini. Hubungi admin.';
    default:
      return err.message;
  }
}
