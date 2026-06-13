/**
 * BankSoal API client — types + functions untuk /api/v1/bank-soal
 * (Fase 6 Task 6.F.1).
 *
 * Backend contracts (Task 6.B):
 *   POST   /bank-soal                     → create soal (guru/admin)
 *   GET    /bank-soal                     → list per-guru + filter
 *                                          mapel/tingkat/topik + pagination
 *   POST   /bank-soal/bulk                → bulk paste pipe-delimited (cap 200)
 *   GET    /bank-soal/:id                 → detail (owner-only / admin)
 *   PATCH  /bank-soal/:id                 → partial update + version (#56)
 *   DELETE /bank-soal/:id                 → soft delete + version
 *   POST   /bank-soal/:id/image           → multipart image upload (slot)
 *   DELETE /bank-soal/:id/image           → clear slot (slot via ?slot=)
 *   GET    /bank-soal/:id/image-url       → presign URL (slot via ?slot=)
 *
 * Locked decisions:
 *   - #56: PATCH wajib version. 409 version_conflict → caller refetch + retry.
 *   - #62: image access via presigned URL.
 *   - #69 / #84: soft delete (BankSoal pernah dipake HasilUjian → tetap valid).
 *   - #78: image inline 6-slot 5MB resize 1920px. Image swap NOT bump version.
 *   - #84: BankSoal scope per-guru pribadi (no share antar-guru).
 *   - #88: backend coverage gate (deferred Fase 8).
 *
 * Beda dari soalbab-api.ts:
 *   - No `mode` field (BankSoal cross-bab, no latihan/ulangan tag)
 *   - Tag fields: mapel/tingkat/topik (free-form, indexed di backend)
 *   - List endpoint top-level (not nested di /bab/:id) + supports pagination
 *   - Bulk paste 8 kolom (drop `mode`), top-level body kirim mapel/tingkat/topik
 */

import { API_BASE, ApiError, api } from '@/lib/api';
import { useAuthStore } from '@/lib/auth';

// ---------- Types ----------

export type BankSoalJawaban = 'a' | 'b' | 'c' | 'd' | 'e';

/**
 * Image slot identifier matching backend ?slot= query param values.
 * Backend accepts: pertanyaan|a|b|c|d|e (locked #78).
 */
export type BankSoalImageSlot = 'pertanyaan' | 'a' | 'b' | 'c' | 'd' | 'e';

export interface BankSoal {
  id: string;
  owner_guru_id: string;
  mapel: string;
  tingkat: string;
  topik: string;
  tags: string[];
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
  jawaban: BankSoalJawaban;
  poin: number;
  version: number;
  deleted_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface BankSoalListResponse {
  items: BankSoal[];
  total: number;
  limit: number;
  offset: number;
}

export interface ListBankSoalOptions {
  mapel?: string;
  tingkat?: string;
  topik?: string;
  tags?: string[];
  limit?: number;
  offset?: number;
}

export interface CreateBankSoalInput {
  mapel?: string;
  tingkat?: string;
  topik?: string;
  tags?: string[];
  pertanyaan: string;
  opsi_a: string;
  opsi_b: string;
  opsi_c: string;
  opsi_d: string;
  opsi_e: string;
  jawaban: BankSoalJawaban;
  poin?: number; // default 1
}

export interface UpdateBankSoalInput {
  version: number;
  mapel?: string;
  tingkat?: string;
  topik?: string;
  tags?: string[];
  pertanyaan?: string;
  opsi_a?: string;
  opsi_b?: string;
  opsi_c?: string;
  opsi_d?: string;
  opsi_e?: string;
  jawaban?: BankSoalJawaban;
  poin?: number;
}

export interface BankSoalBulkLineError {
  line: number;
  reason: string;
  raw?: string;
}

export interface BankSoalBulkResponse {
  created: number;
  errors: BankSoalBulkLineError[];
}

export interface BankSoalBulkInput {
  rows: string;
  mapel?: string;
  tingkat?: string;
  topik?: string;
}

export interface BankSoalImageUploadResponse {
  soal: BankSoal;
}

export interface BankSoalImagePresignResponse {
  url: string;
  expires_at: string;
}

// ---------- Functions ----------

export async function listBankSoal(
  opts: ListBankSoalOptions = {},
): Promise<BankSoalListResponse> {
  const q = new URLSearchParams();
  if (opts.mapel) q.set('mapel', opts.mapel);
  if (opts.tingkat) q.set('tingkat', opts.tingkat);
  if (opts.topik) q.set('topik', opts.topik);
  if (opts.tags?.length) q.set('tags', opts.tags.join(','));
  if (typeof opts.limit === 'number' && opts.limit > 0) {
    q.set('limit', String(opts.limit));
  }
  if (typeof opts.offset === 'number' && opts.offset > 0) {
    q.set('offset', String(opts.offset));
  }
  const qs = q.toString();
  return api<BankSoalListResponse>(`/bank-soal${qs ? `?${qs}` : ''}`);
}

export async function getBankSoal(id: string): Promise<{ soal: BankSoal }> {
  return api<{ soal: BankSoal }>(`/bank-soal/${id}`);
}

export async function createBankSoal(
  input: CreateBankSoalInput,
): Promise<{ soal: BankSoal }> {
  return api<{ soal: BankSoal }>('/bank-soal', {
    method: 'POST',
    body: input,
  });
}

export async function updateBankSoal(
  id: string,
  input: UpdateBankSoalInput,
): Promise<{ soal: BankSoal }> {
  return api<{ soal: BankSoal }>(`/bank-soal/${id}`, {
    method: 'PATCH',
    body: input,
  });
}

/**
 * Soft-delete BankSoal. Body kirim version (locked #56). Backend respon
 * 200 {soal_id, deleted: true}.
 */
export async function deleteBankSoal(
  id: string,
  version: number,
): Promise<{ soal_id: string; deleted: boolean }> {
  return api<{ soal_id: string; deleted: boolean }>(`/bank-soal/${id}`, {
    method: 'DELETE',
    body: { version },
  });
}

export async function bulkCreateBankSoal(
  input: BankSoalBulkInput,
): Promise<BankSoalBulkResponse> {
  return api<BankSoalBulkResponse>('/bank-soal/bulk', {
    method: 'POST',
    body: input,
  });
}

/**
 * Presign URL untuk image slot. Backend GET ?slot=<name>.
 */
export async function getBankSoalImageURL(
  id: string,
  slot: BankSoalImageSlot,
): Promise<BankSoalImagePresignResponse> {
  return api<BankSoalImagePresignResponse>(
    `/bank-soal/${id}/image-url?slot=${encodeURIComponent(slot)}`,
  );
}

/**
 * Clear image slot — DELETE with ?slot=. Backend respon updated soal +
 * compensating R2 delete pada prior key.
 */
export async function deleteBankSoalImage(
  id: string,
  slot: BankSoalImageSlot,
): Promise<{ soal: BankSoal }> {
  return api<{ soal: BankSoal }>(
    `/bank-soal/${id}/image?slot=${encodeURIComponent(slot)}`,
    { method: 'DELETE' },
  );
}

/**
 * Multipart upload image ke slot. Skip api() helper karena Content-Type
 * harus multipart/form-data dengan browser-set boundary. Mirror
 * uploadSoalImage.
 */
export async function uploadBankSoalImage(input: {
  id: string;
  slot: BankSoalImageSlot;
  file: File;
}): Promise<BankSoalImageUploadResponse> {
  const fd = new FormData();
  fd.append('file', input.file);

  const token = useAuthStore.getState().access;
  const headers = new Headers();
  if (token) headers.set('Authorization', `Bearer ${token}`);

  const res = await fetch(
    `${API_BASE}/bank-soal/${input.id}/image?slot=${encodeURIComponent(input.slot)}`,
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

  return (
    (payload as BankSoalImageUploadResponse) ?? { soal: {} as BankSoal }
  );
}

// ---------- Friendly error mapper ----------

export type BankSoalAction =
  | 'create'
  | 'update'
  | 'delete'
  | 'upload-image'
  | 'delete-image'
  | 'bulk';

export function friendlyBankSoalError(
  err: ApiError,
  action: BankSoalAction,
): string {
  switch (err.code) {
    case 'invalid_id':
      return 'ID soal tidak valid.';
    case 'invalid_body':
      return (
        err.message || 'Input tidak valid. Periksa kembali data yang kamu kirim.'
      );
    case 'jawaban_invalid':
      return 'Jawaban menunjuk ke opsi yang kosong (teks/gambar).';
    case 'invalid_version':
      return 'Versi soal tidak valid. Refresh dulu.';
    case 'version_conflict':
      return 'Soal ini baru saja di-update. Form sudah di-refresh dengan data terbaru — silakan ulangi perubahan kamu.';
    case 'forbidden':
      return 'Kamu tidak punya akses ke soal ini (hanya pemilik yang bisa edit).';
    case 'not_found':
      return 'Soal tidak ditemukan (mungkin sudah dihapus).';
    case 'payload_too_large':
      return 'Gambar terlalu besar. Maksimal 5 MB.';
    case 'unsupported_mime':
      return 'Format gambar tidak didukung. Pakai JPG, PNG, atau WebP.';
    case 'image_decode_failed':
      return 'Gambar gagal di-decode. Coba file lain.';
    case 'image_slot_empty':
      return 'Slot gambar masih kosong.';
    case 'r2_unavailable':
      return 'Penyimpanan gambar belum dikonfigurasi. Hubungi admin.';
    case 'missing_file':
      return 'File gambar belum dipilih.';
    case 'invalid_slot':
      return 'Slot gambar tidak valid.';
    case 'too_many':
      return 'Bulk paste melebihi 200 baris. Pecah jadi beberapa batch.';
    case 'rows_required':
      return 'Bulk paste kosong. Tempel minimal 1 baris.';
    case 'invalid_limit':
      return 'Limit harus angka positif.';
    case 'invalid_offset':
      return 'Offset harus angka non-negatif.';
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
