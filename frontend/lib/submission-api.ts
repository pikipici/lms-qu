/**
 * Submission API client — types + functions untuk endpoint /api/v1
 * yang sudah di-ship oleh sub-fase 4.C BE (Task 4.C.1 → 4.C.4, commit
 * 6200d16).
 *
 * Backend contracts (lihat backend/internal/submission/handler.go):
 *
 *   POST   /siswa/tugas/:id/submit                (siswa enrolled, multipart)
 *   GET    /siswa/tugas/:id/submission            (siswa enrolled — own + tugas info)
 *   GET    /tugas/:id/submissions?status=         (guru/admin owner — rekap)
 *   GET    /submission/:id                        (owner OR siswa pemilik)
 *   GET    /submission/:id/attachments/:attID/url (presigned 15-min GET)
 *   POST   /submission/:id/grade                  (guru/admin owner)
 *
 * Locked decisions referenced:
 *   - #46 attachment mime allowlist + size cap.
 *   - #56 optimistic concurrency: PATCH wajib `version`.
 *   - #62 upload flow client → backend → R2 (no direct browser → R2).
 *   - #70 single-row + version bump on resubmit.
 *   - #71 late submission gating: IsLate + penalty calc.
 *   - #72 attachment policy: 0..N optional, cap 5 × 20MB.
 *   - #73 SELECT FOR UPDATE + idempotent guard (already_graded → 409).
 */

import { ApiError, API_BASE, api } from '@/lib/api';
import { useAuthStore } from '@/lib/auth';

export type SubmissionStatus = 'submitted' | 'graded' | 'returned';

export interface SubmissionAttachment {
  id: string;
  submission_id: string;
  object_key: string;
  original_filename: string;
  mime_type: string;
  size_bytes: number;
  created_at: string;
}

export interface Submission {
  id: string;
  tugas_id: string;
  siswa_id: string;
  catatan: string;
  status: SubmissionStatus;
  is_late: boolean;
  nilai_asli?: number | null;
  penalty_persen_applied?: number | null;
  nilai_setelah_penalty?: number | null;
  feedback: string;
  graded_by_id?: string | null;
  graded_at?: string | null;
  version: number;
  submitted_at: string;
  updated_at: string;
  attachments?: SubmissionAttachment[];
}

/** Tugas info snapshot returned alongside MySubmission untuk pre-fill UI. */
export interface SubmissionTugasInfo {
  id: string;
  kelas_id: string;
  judul: string;
  deadline: string | null;
  izinkan_late: boolean;
  penalty_persen: number;
  wajib_attachment: boolean;
}

export interface MySubmissionResponse {
  submission?: Submission;
  tugas: SubmissionTugasInfo;
}

export interface SubmitResponse {
  submission: Submission;
  is_resubmit: boolean;
}

export interface SubmissionListResponse {
  items: Submission[];
  total: number;
}

/** Row item dari GET /siswa/submissions — flat DTO siswa lintas kelas. */
export interface MySubmissionItem {
  submission_id: string;
  tugas_id: string;
  kelas_id: string;
  bab_id?: string | null;
  judul: string;
  deadline?: string | null;
  izinkan_late: boolean;
  penalty_persen: number;
  status: SubmissionStatus;
  is_late: boolean;
  nilai_asli?: number | null;
  penalty_persen_applied?: number | null;
  nilai_setelah_penalty?: number | null;
  feedback: string;
  graded_at?: string | null;
  submitted_at: string;
  version: number;
}

export interface MySubmissionListResponse {
  items: MySubmissionItem[];
  total: number;
}

export interface GradeBody {
  nilai_asli: number;
  feedback?: string;
  version: number;
}

export interface AttachmentURLResponse {
  url: string;
  expires_at: string;
  original_filename: string;
  mime_type: string;
}

// Caps mirror backend (locked #72).
export const MAX_SUBMISSION_CATATAN_BYTES = 50 * 1024;
export const MAX_SUBMISSION_FEEDBACK_BYTES = 5 * 1024;
export const MAX_SUBMISSION_ATTACHMENT_BYTES = 20 * 1024 * 1024;
export const MAX_SUBMISSION_ATTACHMENTS = 5;

/** Mime allowlist (locked #46) untuk hint UI sebelum upload. */
export const SUBMISSION_ATTACHMENT_ACCEPT =
  'application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document,image/jpeg,image/png,application/zip,.pdf,.docx,.jpg,.jpeg,.png,.zip';

/**
 * Siswa: GET /siswa/tugas/:id/submission — own submission + tugas info.
 * `submission` field absent kalau siswa belum submit.
 */
export async function getMySubmission(
  tugasID: string,
): Promise<MySubmissionResponse> {
  return api<MySubmissionResponse>(`/siswa/tugas/${tugasID}/submission`);
}

/**
 * Siswa: GET /siswa/submissions?limit= — semua submission siswa lintas kelas
 * (Task 4.D.2). JOIN-backed, tugas snapshot di-include per row.
 */
export async function listMySubmissions(
  opts: { limit?: number } = {},
): Promise<MySubmissionListResponse> {
  const q = new URLSearchParams();
  if (opts.limit) q.set('limit', String(opts.limit));
  const qs = q.toString();
  return api<MySubmissionListResponse>(
    `/siswa/submissions${qs ? `?${qs}` : ''}`,
  );
}

/**
 * Siswa: POST /siswa/tugas/:id/submit (multipart). Hand-roll fetch karena
 * api() set Content-Type JSON. Pola sama dengan uploadAttachment di
 * tugas-api.ts.
 */
export async function submitTugas(input: {
  tugasID: string;
  catatan: string;
  files: File[];
}): Promise<SubmitResponse> {
  const fd = new FormData();
  fd.append('catatan', input.catatan ?? '');
  for (const f of input.files) {
    fd.append('files', f);
  }

  const token = useAuthStore.getState().access;
  const headers = new Headers();
  if (token) headers.set('Authorization', `Bearer ${token}`);

  const res = await fetch(
    `${API_BASE}/siswa/tugas/${input.tugasID}/submit`,
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
        : null) ?? `Submit failed (${res.status})`;
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
  return payload as SubmitResponse;
}

/** Guru/admin: GET /tugas/:id/submissions?status=. */
export async function listTugasSubmissions(
  tugasID: string,
  opts: { status?: SubmissionStatus } = {},
): Promise<SubmissionListResponse> {
  const q = new URLSearchParams();
  if (opts.status) q.set('status', opts.status);
  const qs = q.toString();
  return api<SubmissionListResponse>(
    `/tugas/${tugasID}/submissions${qs ? `?${qs}` : ''}`,
  );
}

/** GET /submission/:id (guru/admin owner OR siswa pemilik). */
export async function getSubmission(id: string): Promise<Submission> {
  return api<Submission>(`/submission/${id}`);
}

/** GET /submission/:id/attachments/:attID/url — presigned 15-min GET URL. */
export async function getSubmissionAttachmentURL(
  submissionID: string,
  attachmentID: string,
): Promise<AttachmentURLResponse> {
  return api<AttachmentURLResponse>(
    `/submission/${submissionID}/attachments/${attachmentID}/url`,
  );
}

/** POST /submission/:id/grade (guru/admin owner). */
export async function gradeSubmission(
  id: string,
  body: GradeBody,
): Promise<Submission> {
  return api<Submission>(`/submission/${id}/grade`, {
    method: 'POST',
    body,
  });
}

export type SubmissionAction =
  | 'submit'
  | 'resubmit'
  | 'grade'
  | 'list'
  | 'get'
  | 'attachment-url';

/**
 * Friendly Indonesian error untuk ApiError dari endpoint submission.
 * Mirror pola friendlyTugasError. Caller pakai untuk isi description toast.
 */
export function friendlySubmissionError(
  err: ApiError,
  action: SubmissionAction,
): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID submission/tugas/attachment tidak valid.';
    case 'invalid_input':
      return 'Input tidak valid. Periksa kembali data yang kamu kirim.';
    case 'invalid_form':
      return 'Format form tidak valid. Refresh halaman lalu coba lagi.';
    case 'version_conflict':
      return 'Submission ini baru saja di-update orang lain. Refresh dulu.';
    case 'forbidden':
      return 'Kamu tidak punya akses ke submission ini.';
    case 'not_found':
      return action === 'submit' || action === 'resubmit'
        ? 'Tugas tidak ditemukan atau sudah tidak terbuka.'
        : 'Submission tidak ditemukan.';
    case 'deadline_passed':
      return 'Deadline sudah lewat dan late submission tidak diizinkan untuk tugas ini.';
    case 'already_graded':
      return action === 'submit' || action === 'resubmit'
        ? 'Tugas kamu sudah dinilai oleh guru — tidak bisa resubmit lagi.'
        : 'Submission ini sudah dinilai sebelumnya.';
    case 'attachment_required':
      return 'Tugas ini wajib upload minimal 1 lampiran.';
    case 'attachment_limit_reached':
      return `Maksimal ${MAX_SUBMISSION_ATTACHMENTS} lampiran per submission.`;
    case 'payload_too_large':
      return `File terlalu besar. Batas ${MAX_SUBMISSION_ATTACHMENT_BYTES / (1024 * 1024)} MB per lampiran.`;
    case 'unsupported_mime':
      return 'Format file tidak didukung. Pakai PDF, DOCX, JPG, PNG, atau ZIP.';
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

/** Helper: format ISO timestamp → lokal Asia/Jakarta (id-ID). */
export function formatSubmissionTimestamp(input?: string | null): string {
  if (!input) return '—';
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

/** True kalau tugas deadline sudah lewat. */
export function isTugasOverdue(
  tugas: Pick<SubmissionTugasInfo, 'deadline'>,
  now: Date = new Date(),
): boolean {
  if (!tugas.deadline) return false;
  const dl = new Date(tugas.deadline).getTime();
  if (Number.isNaN(dl)) return false;
  return now.getTime() > dl;
}

/** Compute label status submission untuk UI. */
export function statusLabel(status: SubmissionStatus): string {
  switch (status) {
    case 'submitted':
      return 'Terkirim, menunggu nilai';
    case 'graded':
      return 'Sudah dinilai';
    case 'returned':
      return 'Dikembalikan untuk revisi';
  }
}

/**
 * Compute preview nilai_setelah_penalty untuk UI guru sebelum submit grade.
 * Mirror backend logic: penalty hanya apply kalau is_late + penalty_persen > 0.
 */
export function previewNilaiSetelahPenalty(
  nilaiAsli: number,
  isLate: boolean,
  penaltyPersen: number,
): number {
  if (!isLate || penaltyPersen <= 0) {
    return Math.round(nilaiAsli * 100) / 100;
  }
  return Math.round(nilaiAsli * (1 - penaltyPersen / 100) * 100) / 100;
}
