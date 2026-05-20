/**
 * Import-CSV API client — types + functions for /api/v1/admin/import-csv.
 *
 * Backend contracts (commits 1b46030 + a9dbbc3):
 *   POST   /admin/import-csv/upload                multipart `file` -> 201 PreviewResponse
 *   GET    /admin/import-csv/:job_id               -> PreviewResponse (resume preview tab)
 *   POST   /admin/import-csv/:job_id/cancel        -> CancelResponse
 *   POST   /admin/import-csv/:job_id/confirm       -> ConfirmResponse
 *   GET    /admin/import-csv/:job_id/credentials.csv  302 redirect → R2 presigned URL
 *
 * Lifecycle (locked decision #54):
 *   preview → processing → completed
 *           ↘ cancelled / expired / failed
 *
 * TTLs:
 *   - PreviewTTL          = 1h after upload (then status=expired via cron)
 *   - CredentialsTTL      = 1h after CompletedAt (R2 credentials.csv evicted)
 *   - PresignURL TTL      = 15min (forced attachment Content-Disposition)
 */

import { api, ApiError, API_BASE } from '@/lib/api';
import { useAuthStore } from '@/lib/auth';

export type ImportRowStatus = 'valid' | 'invalid' | 'duplicate';
export type ImportJobStatus =
  | 'preview'
  | 'processing'
  | 'completed'
  | 'expired'
  | 'cancelled'
  | 'failed';

export interface ImportRow {
  line_no: number;
  nama: string;
  email: string;
  kode_kelas: string;
  status: ImportRowStatus;
  errors?: string[] | null;
}

export interface PreviewResponse {
  job_id: string;
  valid_count: number;
  invalid_count: number;
  total_rows: number;
  preview_rows: ImportRow[];
  expires_at: string;
  filename?: string;
  status?: ImportJobStatus;
}

export interface CancelResponse {
  job_id: string;
  status: ImportJobStatus;
  filename: string;
}

export interface ConfirmFailure {
  line_no: number;
  email: string;
  reason: string;
  detail?: string;
}

export interface ConfirmResponse {
  job_id: string;
  status: ImportJobStatus;
  filename: string;
  success_count: number;
  fail_count: number;
  credentials_object_key: string;
  failures: ConfirmFailure[];
}

/**
 * Upload a CSV via multipart. Returns the preview job id + parsed stats +
 * the first ~200 preview rows. On 415/413 the backend rejects via stable
 * code (`unsupported_mime` / `file_too_large` / `csv_too_large`).
 *
 * NOTE: we cannot reuse `api()` here because it forces JSON Content-Type.
 * Multipart uploads need the browser to set Content-Type: multipart/form-data
 * with boundary, so we hand-roll fetch with the auth header from the store.
 */
export async function uploadImportCSV(file: File): Promise<PreviewResponse> {
  const fd = new FormData();
  fd.append('file', file);

  const token = useAuthStore.getState().access;
  const headers = new Headers();
  if (token) headers.set('Authorization', `Bearer ${token}`);

  const res = await fetch(`${API_BASE}/admin/import-csv/upload`, {
    method: 'POST',
    headers,
    body: fd,
  });

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

  return payload as PreviewResponse;
}

export async function getImportPreview(jobID: string): Promise<PreviewResponse> {
  return api<PreviewResponse>(`/admin/import-csv/${jobID}`);
}

export async function cancelImport(jobID: string): Promise<CancelResponse> {
  return api<CancelResponse>(`/admin/import-csv/${jobID}/cancel`, {
    method: 'POST',
  });
}

export async function confirmImport(jobID: string): Promise<ConfirmResponse> {
  return api<ConfirmResponse>(`/admin/import-csv/${jobID}/confirm`, {
    method: 'POST',
  });
}

/**
 * Build the credentials.csv download URL. Backend 302-redirects to a
 * presigned R2 URL; the browser will follow automatically and trigger
 * download via the embedded Content-Disposition header.
 *
 * Bearer token must be sent on the initial request, so we cannot just use
 * a plain `<a href>`. Caller fetches via `downloadCredentialsCSV` instead,
 * which lets the redirect happen and rewrites window.location to the
 * presigned URL (or, on browsers that follow 302 with auth header stripped
 * by the time the redirect lands at R2, just opens the final URL).
 */
export function credentialsDownloadPath(jobID: string): string {
  return `/admin/import-csv/${jobID}/credentials.csv`;
}

/**
 * Trigger credentials.csv download. We do a manual fetch so we can attach
 * the bearer token on the first hop, then read the Location header (302)
 * and open the presigned URL in a new tab (or assign to window.location
 * for direct save-as-file behaviour).
 *
 * Errors come back as ApiError (404 not_found / 409 not_completed / 410
 * credentials_expired / 404 credentials_missing).
 */
export async function downloadCredentialsCSV(jobID: string): Promise<string> {
  const token = useAuthStore.getState().access;
  const headers = new Headers();
  if (token) headers.set('Authorization', `Bearer ${token}`);

  const res = await fetch(`${API_BASE}${credentialsDownloadPath(jobID)}`, {
    method: 'GET',
    headers,
    redirect: 'manual',
  });

  // 302/303 → grab Location header
  if (res.type === 'opaqueredirect' || (res.status >= 300 && res.status < 400)) {
    const loc = res.headers.get('Location');
    if (loc) return loc;
  }

  // Some browsers swallow Location on opaqueredirect; do a fallback
  // follow request (browser auto-follow) — works because R2 presigned URLs
  // don't require auth.
  if (res.type === 'opaqueredirect') {
    // Re-issue with auto-follow so the browser picks up the final URL.
    const followed = await fetch(`${API_BASE}${credentialsDownloadPath(jobID)}`, {
      method: 'GET',
      headers,
      redirect: 'follow',
    });
    if (followed.ok) {
      // followed.url is the final R2 presigned URL.
      return followed.url;
    }
  }

  if (!res.ok) {
    const requestId = res.headers.get('X-Request-ID') ?? undefined;
    const contentType = res.headers.get('Content-Type') ?? '';
    const payload = contentType.includes('application/json')
      ? await res.json().catch(() => null)
      : null;
    const message =
      (payload && typeof payload === 'object' && 'error' in payload
        ? String((payload as { error: unknown }).error)
        : null) ?? `Download failed (${res.status})`;
    const code =
      (payload && typeof payload === 'object' && 'code' in payload
        ? String((payload as { code: unknown }).code)
        : null) ?? 'unknown';
    throw new ApiError({ status: res.status, code, message, requestId, details: payload });
  }

  // Fallback: response was already a 200 (shouldn't happen with current
  // backend, but be defensive). Return its url so caller can window.open it.
  return res.url;
}

/**
 * Stable copy mapping for ConfirmFailure.reason (matches backend Reason*
 * constants in service.go). Keeps row-level error strings non-cryptic.
 */
export const confirmReasonLabel: Record<string, string> = {
  invalid_row: 'Baris tidak valid',
  duplicate_in_db: 'Email sudah terdaftar',
  user_create_error: 'Gagal membuat user',
  hash_error: 'Gagal generate password',
  kelas_not_found: 'Kode kelas tidak ditemukan',
  enroll_error: 'Gagal enroll ke kelas',
};
