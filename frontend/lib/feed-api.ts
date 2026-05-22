/**
 * Activity feed API client (Task 7.C, locked #39+#55).
 *
 *   GET /api/v1/guru/feed?cursor=<base64>&limit=20
 *
 * Cursor: opaque base64 of (at_unix_micro, id) descending. Empty → latest.
 * Polling 30s pakai cursor=null. Load-more pakai next_cursor.
 */

import { api } from '@/lib/api';

export type FeedEventKind =
  | 'submission_baru'
  | 'ulangan_selesai'
  | 'siswa_join';

export interface FeedEvent {
  id: string;
  kind: FeedEventKind;
  at: string;
  kelas_id: string;
  kelas_nama: string;
  siswa_id: string;
  siswa_nama: string;
  // Submission-specific
  tugas_id?: string;
  tugas_judul?: string;
  is_late?: boolean;
  // Ulangan-specific
  ujian_id?: string;
  ujian_judul?: string;
  hasil_id?: string;
  nilai_total?: number | null;
}

export interface FeedListResponse {
  events: FeedEvent[];
  next_cursor: string;
}

export async function listGuruFeed(params: {
  cursor?: string;
  limit?: number;
} = {}): Promise<FeedListResponse> {
  const q = new URLSearchParams();
  if (params.cursor) q.set('cursor', params.cursor);
  if (params.limit) q.set('limit', String(params.limit));
  const qs = q.toString();
  return api<FeedListResponse>(`/guru/feed${qs ? `?${qs}` : ''}`);
}
