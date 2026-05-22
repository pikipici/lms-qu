/**
 * SoalBab API client — types + functions for /api/v1/bab/:id/soal +
 * /api/v1/soal-bab/:id (Fase 5 Task 5.F.1).
 *
 * Backend contracts:
 *   POST   /bab/:id/soal                  → create soal (guru/admin owner)
 *   GET    /bab/:id/soal                  → list per bab + filter mode + limit
 *   POST   /bab/:id/soal/bulk             → bulk paste pipe-delimited (cap 200)
 *   GET    /soal-bab/:id                  → detail
 *   PATCH  /soal-bab/:id                  → partial update + version (#56)
 *   DELETE /soal-bab/:id                  → hard delete + R2 compensating
 *   POST   /soal-bab/:id/image            → multipart image upload (slot)
 *   DELETE /soal-bab/:id/image            → clear slot (slot via ?slot=)
 *   GET    /soal-bab/:id/image-url        → presign URL (slot via ?slot=)
 *
 * Locked decisions:
 *   - #56: PATCH wajib version. 409 version_conflict → caller refetch + retry.
 *   - #69: Hard delete + R2 compensating cleanup di backend.
 *   - #77: Bulk paste pipe-delimited 9 kolom. Cap 200 baris. Partial-success.
 *   - #78: Image inline 6-slot 5MB resize 1920px. Image swap NOT bump version.
 *   - #82: Coverage gate 70% backend.
 */

import { API_BASE, ApiError, api } from '@/lib/api';
import { useAuthStore } from '@/lib/auth';

// ---------- Types ----------

export type SoalMode = 'latihan' | 'ulangan' | 'keduanya';
export type SoalJawaban = 'a' | 'b' | 'c' | 'd' | 'e';

/**
 * Image slot identifier matching backend ?slot= query param values.
 * Backend accepts: pertanyaan|a|b|c|d|e (locked #78).
 */
export type SoalImageSlot = 'pertanyaan' | 'a' | 'b' | 'c' | 'd' | 'e';

export interface SoalBab {
  id: string;
  bab_id: string;
  kelas_id: string;
  pertanyaan: string;
  pertanyaan_object_key?: string;
  opsi_a: string;
  opsi_a_object_key?: string;
  opsi_b: string;
  opsi_b_object_key?: string;
  opsi_c: string;
  opsi_c_object_key?: string;
  opsi_d: string;
  opsi_d_object_key?: string;
  opsi_e: string;
  opsi_e_object_key?: string;
  jawaban: SoalJawaban;
  poin: number;
  mode: SoalMode;
  urutan: number;
  version: number;
  created_by_id: string;
  created_at: string;
  updated_at: string;
}

export interface SoalListResponse {
  items: SoalBab[];
  total: number;
}

export interface ListSoalOptions {
  mode?: SoalMode;
  limit?: number;
}

export interface CreateSoalInput {
  pertanyaan: string;
  opsi_a: string;
  opsi_b: string;
  opsi_c: string;
  opsi_d: string;
  opsi_e: string;
  jawaban: SoalJawaban;
  poin?: number; // default 1
  mode?: SoalMode; // default keduanya
  urutan?: number;
}

export interface UpdateSoalInput {
  version: number;
  pertanyaan?: string;
  opsi_a?: string;
  opsi_b?: string;
  opsi_c?: string;
  opsi_d?: string;
  opsi_e?: string;
  jawaban?: SoalJawaban;
  poin?: number;
  mode?: SoalMode;
  urutan?: number;
}

export interface BulkSoalLineError {
  line: number;
  reason: string;
  raw?: string;
}

export interface BulkSoalResponse {
  inserted: number;
  errors: BulkSoalLineError[];
}

export interface ImageUploadResponse {
  soal: SoalBab;
}

export interface ImagePresignResponse {
  url: string;
  expires_at: string;
}

// ---------- Functions ----------

export async function listSoal(
  babID: string,
  opts: ListSoalOptions = {},
): Promise<SoalListResponse> {
  const q = new URLSearchParams();
  if (opts.mode) q.set('mode', opts.mode);
  if (opts.limit && opts.limit > 0) q.set('limit', String(opts.limit));
  const qs = q.toString();
  return api<SoalListResponse>(`/bab/${babID}/soal${qs ? `?${qs}` : ''}`);
}

export async function getSoal(id: string): Promise<{ soal: SoalBab }> {
  return api<{ soal: SoalBab }>(`/soal-bab/${id}`);
}

export async function createSoal(
  babID: string,
  input: CreateSoalInput,
): Promise<{ soal: SoalBab }> {
  return api<{ soal: SoalBab }>(`/bab/${babID}/soal`, {
    method: 'POST',
    body: input,
  });
}

export async function updateSoal(
  id: string,
  input: UpdateSoalInput,
): Promise<{ soal: SoalBab }> {
  return api<{ soal: SoalBab }>(`/soal-bab/${id}`, {
    method: 'PATCH',
    body: input,
  });
}

export async function deleteSoal(id: string): Promise<void> {
  await api<{ ok: boolean }>(`/soal-bab/${id}`, { method: 'DELETE' });
}

export async function bulkCreateSoal(
  babID: string,
  raw: string,
): Promise<BulkSoalResponse> {
  return api<BulkSoalResponse>(`/bab/${babID}/soal/bulk`, {
    method: 'POST',
    body: { raw },
  });
}

/**
 * Presign URL for an image slot. Uses backend GET endpoint with ?slot=<name>.
 */
export async function getSoalImageURL(
  id: string,
  slot: SoalImageSlot,
): Promise<ImagePresignResponse> {
  return api<ImagePresignResponse>(
    `/soal-bab/${id}/image-url?slot=${encodeURIComponent(slot)}`,
  );
}

/**
 * Clear an image slot — DELETE with ?slot=. Backend returns updated soal +
 * compensating R2 delete on the prior key.
 */
export async function deleteSoalImage(
  id: string,
  slot: SoalImageSlot,
): Promise<{ soal: SoalBab }> {
  return api<{ soal: SoalBab }>(
    `/soal-bab/${id}/image?slot=${encodeURIComponent(slot)}`,
    { method: 'DELETE' },
  );
}

/**
 * Multipart upload an image to a slot. Returns the updated soal row.
 * Skip api() helper because Content-Type must be multipart/form-data with
 * the browser-set boundary. Mirrors uploadMateriPDF pattern.
 */
export async function uploadSoalImage(input: {
  id: string;
  slot: SoalImageSlot;
  file: File;
}): Promise<ImageUploadResponse> {
  const fd = new FormData();
  fd.append('file', input.file);
  fd.append('slot', input.slot);

  const token = useAuthStore.getState().access;
  const headers = new Headers();
  if (token) headers.set('Authorization', `Bearer ${token}`);

  const res = await fetch(`${API_BASE}/soal-bab/${input.id}/image`, {
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

  return (payload as ImageUploadResponse) ?? { soal: {} as SoalBab };
}

// ---------- Friendly error mapper ----------

export type SoalAction =
  | 'create'
  | 'update'
  | 'delete'
  | 'upload-image'
  | 'delete-image'
  | 'bulk';

export function friendlySoalError(err: ApiError, action: SoalAction): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID soal tidak valid.';
    case 'invalid_body':
    case 'invalid_input':
      return err.message || 'Input tidak valid. Periksa kembali data yang lu kirim.';
    case 'invalid_jawaban':
      return 'Jawaban harus salah satu dari a, b, c, d, atau e.';
    case 'invalid_mode':
      return 'Mode harus latihan, ulangan, atau keduanya.';
    case 'invalid_version':
      return 'Versi soal tidak valid. Refresh dulu.';
    case 'version_conflict':
      return 'Soal ini baru saja di-update orang lain. Form sudah di-refresh dengan data terbaru — silakan ulangi perubahan lu.';
    case 'forbidden':
      return 'Lu tidak punya akses ke soal ini.';
    case 'not_found':
      return 'Soal tidak ditemukan (mungkin sudah dihapus).';
    case 'bab_archived':
      return 'Bab sudah diarsipkan; soal tidak bisa diubah.';
    case 'image_too_large':
      return 'Gambar terlalu besar. Maksimal 5 MB.';
    case 'image_invalid_type':
      return 'Format gambar tidak didukung. Pakai JPG, PNG, atau WebP.';
    case 'image_decode_failed':
      return 'Gambar gagal di-decode. Coba file lain.';
    case 'invalid_slot':
      return 'Slot gambar tidak valid.';
    case 'bulk_too_many_lines':
      return 'Bulk paste melebihi 200 baris. Pecah jadi beberapa batch.';
    case 'bulk_empty':
      return 'Bulk paste kosong. Tempel minimal 1 baris.';
    case 'bulk_no_valid_lines':
      return 'Tidak ada baris yang valid. Periksa format pipe-delimited.';
    case 'bulk_kelas_archived':
    case 'kelas_archived':
      return 'Kelas sudah diarsipkan; soal tidak bisa dibuat baru.';
    default:
      switch (action) {
        case 'create':
          return err.message || 'Gagal membuat soal.';
        case 'update':
          return err.message || 'Gagal menyimpan soal.';
        case 'delete':
          return err.message || 'Gagal menghapus soal.';
        case 'upload-image':
          return err.message || 'Gagal mengunggah gambar.';
        case 'delete-image':
          return err.message || 'Gagal menghapus gambar.';
        case 'bulk':
          return err.message || 'Gagal memproses bulk paste.';
        default:
          return err.message;
      }
  }
}
