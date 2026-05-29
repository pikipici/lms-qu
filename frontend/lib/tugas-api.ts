/**
 * Tugas (assignment) API client — types + functions untuk endpoint /api/v1
 * yang sudah di-ship oleh sub-fase 4.A BE (Task 4.A.1 → 4.A.3).
 *
 * Backend contracts (lihat backend/internal/tugas/handler.go +
 * backend/internal/tugas/attachment_handler.go):
 *
 *   POST   /kelas/:id/tugas                       (guru/admin own kelas)
 *   GET    /kelas/:id/tugas?bab_id=&status=&limit=
 *   GET    /siswa/kelas/:id/tugas?bab_id=&limit=
 *   GET    /tugas/:id
 *   PATCH  /tugas/:id
 *   DELETE /tugas/:id
 *   POST   /tugas/:id/attachments                 (multipart, guru/admin)
 *   GET    /tugas/:id/attachments
 *   DELETE /tugas/:id/attachments/:attID
 *   GET    /tugas/:id/attachments/:attID/url      (presigned 15-min GET)
 *
 * bab_id query semantics:
 *   - absent       → no filter (semua di kelas)
 *   - 'null'/'none'→ pin bab_id IS NULL (kelas-wide tugas)
 *   - <uuid>       → pin bab_id = uuid (bab-scoped)
 *
 * status query (guru/admin only — siswa always forced ke 'published'):
 *   - 'draft' | 'published' | 'archived' | absent (= all guru-visible)
 *
 * Locked decisions referenced:
 *   - #20 BabID nullable (kelas-wide vs bab-scoped).
 *   - #46 attachment mime allowlist + size cap.
 *   - #56 optimistic concurrency: PATCH wajib `version`.
 *   - #62 upload flow client→backend→R2 (no direct browser→R2).
 *   - #71 late submission gating: IzinkanLate + PenaltyPersen (0-100).
 *   - #72 submission attachment policy: WajibAttachment per-tugas.
 *   - #74 tugas attachment cap 5 file × 20MB, R2 prefix "tugas/".
 */

import { ApiError, API_BASE, api } from '@/lib/api';
import { useAuthStore } from '@/lib/auth';

export type TugasStatus = 'draft' | 'published' | 'archived';

export interface TugasAttachment {
  id: string;
  tugas_id: string;
  object_key: string;
  original_filename: string;
  mime_type: string;
  size_bytes: number;
  created_at: string;
}

export interface Tugas {
  id: string;
  kelas_id: string;
  bab_id: string | null;
  judul: string;
  deskripsi: string;
  deadline: string | null;
  izinkan_late: boolean;
  penalty_persen: number;
  wajib_attachment: boolean;
  bobot: number;
  status: TugasStatus;
  version: number;
  created_by_id: string;
  created_at: string;
  updated_at: string;
  attachments?: TugasAttachment[];
}

export interface TugasListResponse {
  items: Tugas[];
  total: number;
}

export interface TugasDetailResponse {
  tugas: Tugas;
}

export interface TugasDeleteResponse {
  tugas_id: string;
}

export interface CreateTugasBody {
  bab_id?: string | null;
  judul: string;
  /** Markdown body — max 50KB (mirror locked #63). */
  deskripsi?: string;
  /** ISO timestamp atau null = always-open. */
  deadline?: string | null;
  izinkan_late?: boolean;
  /** 0-100, hanya bermakna kalau izinkan_late=true. */
  penalty_persen?: number;
  wajib_attachment?: boolean;
  bobot?: number;
  status?: TugasStatus;
}

/**
 * UpdateTugasBody — partial update + optimistic concurrency.
 * Field absent = unchanged. `bab_id`/`deadline` null secara eksplisit =
 * clear ke kelas-wide / always-open.
 */
export interface UpdateTugasBody {
  version: number;
  judul?: string;
  deskripsi?: string;
  bab_id?: string | null;
  deadline?: string | null;
  izinkan_late?: boolean;
  penalty_persen?: number;
  wajib_attachment?: boolean;
  bobot?: number;
  status?: TugasStatus;
}

export interface ListTugasOptions {
  /**
   * undefined: no filter | null: pin bab_id IS NULL | string: pin uuid.
   */
  babID?: string | null;
  status?: TugasStatus;
  limit?: number;
}

export interface UploadAttachmentResponse {
  attachment: TugasAttachment;
  object_key: string;
  original_filename: string;
  size_bytes: number;
}

export interface AttachmentListResponse {
  items: TugasAttachment[];
  total: number;
}

export interface AttachmentDeleteResponse {
  tugas_id: string;
  attachment_id: string;
}

export interface AttachmentURLResponse {
  url: string;
  expires_at: string;
  original_filename: string;
  mime_type: string;
}

/** Cap markdown deskripsi 50KB sebelum kirim (mirror locked #63 materi). */
export const MAX_TUGAS_DESKRIPSI_BYTES = 50 * 1024;

/** Cap judul 200 chars (mirror backend MaxJudulBytes). */
export const MAX_TUGAS_JUDUL_LENGTH = 200;

/** Cap attachment 20MB per file (locked #74). */
export const MAX_TUGAS_ATTACHMENT_BYTES = 20 * 1024 * 1024;

/** Cap 5 attachment per tugas (locked #74). */
export const MAX_TUGAS_ATTACHMENTS = 5;

/** Mime allowlist (locked #46) untuk hint UI sebelum upload. */
export const TUGAS_ATTACHMENT_ACCEPT =
  'application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document,image/jpeg,image/png,application/zip,.pdf,.docx,.jpg,.jpeg,.png,.zip';

function buildBabQuery(babID: string | null | undefined): string | null {
  if (babID === undefined) return null;
  if (babID === null) return 'null';
  if (typeof babID === 'string' && babID) return babID;
  return null;
}

/** Guru/admin list: GET /kelas/:id/tugas. */
export async function listTugas(
  kelasID: string,
  opts: ListTugasOptions = {},
): Promise<TugasListResponse> {
  const q = new URLSearchParams();
  const bab = buildBabQuery(opts.babID);
  if (bab !== null) q.set('bab_id', bab);
  if (opts.status) q.set('status', opts.status);
  if (typeof opts.limit === 'number' && opts.limit > 0) {
    q.set('limit', String(opts.limit));
  }
  const qs = q.toString();
  return api<TugasListResponse>(
    `/kelas/${kelasID}/tugas${qs ? `?${qs}` : ''}`,
  );
}

/** Siswa enrolled list: GET /siswa/kelas/:id/tugas (server forces published). */
export async function listSiswaTugas(
  kelasID: string,
  opts: Omit<ListTugasOptions, 'status'> = {},
): Promise<TugasListResponse> {
  const q = new URLSearchParams();
  const bab = buildBabQuery(opts.babID);
  if (bab !== null) q.set('bab_id', bab);
  if (typeof opts.limit === 'number' && opts.limit > 0) {
    q.set('limit', String(opts.limit));
  }
  const qs = q.toString();
  return api<TugasListResponse>(
    `/siswa/kelas/${kelasID}/tugas${qs ? `?${qs}` : ''}`,
  );
}

export async function getTugas(id: string): Promise<TugasDetailResponse> {
  return api<TugasDetailResponse>(`/tugas/${id}`);
}

export async function createTugas(
  kelasID: string,
  input: CreateTugasBody,
): Promise<TugasDetailResponse> {
  return api<TugasDetailResponse>(`/kelas/${kelasID}/tugas`, {
    method: 'POST',
    body: input,
  });
}

export async function updateTugas(
  id: string,
  input: UpdateTugasBody,
): Promise<TugasDetailResponse> {
  return api<TugasDetailResponse>(`/tugas/${id}`, {
    method: 'PATCH',
    body: input,
  });
}

export async function deleteTugas(id: string): Promise<TugasDeleteResponse> {
  return api<TugasDeleteResponse>(`/tugas/${id}`, { method: 'DELETE' });
}

/** GET /tugas/:id/attachments — list attachments untuk satu tugas. */
export async function listAttachments(
  tugasID: string,
): Promise<AttachmentListResponse> {
  return api<AttachmentListResponse>(`/tugas/${tugasID}/attachments`);
}

/** DELETE /tugas/:id/attachments/:attID. */
export async function deleteAttachment(
  tugasID: string,
  attachmentID: string,
): Promise<AttachmentDeleteResponse> {
  return api<AttachmentDeleteResponse>(
    `/tugas/${tugasID}/attachments/${attachmentID}`,
    { method: 'DELETE' },
  );
}

/** GET /tugas/:id/attachments/:attID/url — presigned 15-min GET URL. */
export async function getAttachmentURL(
  tugasID: string,
  attachmentID: string,
): Promise<AttachmentURLResponse> {
  return api<AttachmentURLResponse>(
    `/tugas/${tugasID}/attachments/${attachmentID}/url`,
  );
}

/**
 * Multipart upload attachment ke tugas (locked #62: client → backend → R2).
 * Hand-roll fetch karena api() set Content-Type JSON. Pola sama dengan
 * uploadMateriPDF di materi-api.ts.
 */
export async function uploadAttachment(input: {
  tugasID: string;
  file: File;
}): Promise<UploadAttachmentResponse> {
  const fd = new FormData();
  fd.append('file', input.file);

  const token = useAuthStore.getState().access;
  const headers = new Headers();
  if (token) headers.set('Authorization', `Bearer ${token}`);

  const res = await fetch(
    `${API_BASE}/tugas/${input.tugasID}/attachments`,
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
  return payload as UploadAttachmentResponse;
}

export type TugasAction =
  | 'create'
  | 'update'
  | 'archive'
  | 'delete'
  | 'list'
  | 'get'
  | 'upload-attachment'
  | 'delete-attachment'
  | 'attachment-url';

/**
 * Friendly Indonesian error untuk ApiError dari endpoint tugas.
 * Mirror pola friendlyPengumumanError. Caller pakai untuk isi description toast.
 */
export function friendlyTugasError(
  err: ApiError,
  action: TugasAction,
): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID tugas atau kelas tidak valid.';
    case 'invalid_body':
    case 'invalid_input':
      return 'Input tidak valid. Periksa kembali data yang kamu kirim.';
    case 'invalid_status':
      return 'Status tidak valid. Pilih draft, published, atau archived.';
    case 'invalid_version':
      return 'Versi tugas tidak valid. Refresh halaman dulu.';
    case 'invalid_limit':
      return 'Limit harus angka positif.';
    case 'version_conflict':
      return 'Tugas ini baru saja di-update orang lain. Form sudah di-refresh — ulangi perubahan kamu.';
    case 'forbidden':
      return action === 'list' || action === 'get'
        ? 'Kamu tidak punya akses ke tugas kelas ini.'
        : 'Kamu tidak punya akses untuk mengubah tugas ini.';
    case 'not_found':
      return action === 'create'
        ? 'Kelas tidak ditemukan (mungkin sudah dihapus).'
        : action === 'delete-attachment' || action === 'attachment-url'
          ? 'Attachment tidak ditemukan (mungkin sudah dihapus).'
          : 'Tugas tidak ditemukan (mungkin sudah dihapus).';
    case 'kelas_archived':
      return 'Kelas sudah diarsipkan; tugas tidak bisa dibuat baru atau diubah.';
    case 'bab_not_in_kelas':
      return 'Bab yang dipilih bukan milik kelas ini.';
    case 'payload_too_large':
      return action === 'upload-attachment'
        ? `File terlalu besar. Batas ${MAX_TUGAS_ATTACHMENT_BYTES / (1024 * 1024)} MB per attachment.`
        : `Konten terlalu panjang. Batas ${MAX_TUGAS_DESKRIPSI_BYTES / 1024} KB.`;
    case 'unsupported_mime':
      return 'Format file tidak didukung. Pakai PDF, DOCX, JPG, PNG, atau ZIP.';
    case 'attachment_limit_reached':
      return `Maksimal ${MAX_TUGAS_ATTACHMENTS} attachment per tugas.`;
    case 'missing_file':
      return 'File belum dipilih.';
    case 'open_failed':
    case 'read_failed':
      return 'Gagal membaca file yang kamu unggah.';
    case 'r2_put_failed':
      return 'Upload ke object store gagal. Coba ulangi sebentar lagi.';
    case 'r2_unavailable':
      return 'Object store tidak tersedia saat ini. Hubungi admin.';
    default:
      return err.message;
  }
}

/** Helper: format deadline ke string lokal Asia/Jakarta (id-ID). */
export function formatDeadline(input?: string | null): string {
  if (!input) return 'Tanpa deadline';
  try {
    return new Date(input).toLocaleString('id-ID', {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: 'Asia/Jakarta',
    });
  } catch {
    return input;
  }
}

/** True kalau deadline sudah lewat (now > deadline). null = always-open = false. */
export function isOverdue(
  tugas: Pick<Tugas, 'deadline'>,
  now: Date = new Date(),
): boolean {
  if (!tugas.deadline) return false;
  const dl = new Date(tugas.deadline).getTime();
  if (Number.isNaN(dl)) return false;
  return now.getTime() > dl;
}
