/**
 * Guru-side API client.
 *
 * Pending counters (locked #40 + #93, Task 7.D consolidated):
 * single endpoint `/guru/pending-counts` returns 3 attention counters
 * — ungraded submissions, pending review ulangan bab, pending review ujian.
 * Cumulative across kelas yang dimiliki guru. Polling 30s di FE.
 */

import { api } from '@/lib/api';

export interface PendingCountsResponse {
  ungraded_submissions: number;
  pending_review_ulangan: number;
  pending_review_ujian: number;
}

export interface PendingItem {
  id: string;
  kelas_id: string;
  kelas_nama: string;
  title: string;
  subtitle?: string;
  target_url: string;
  submitted_at?: string;
  finished_at?: string;
}

export interface PendingItemsResponse {
  ungraded_submissions?: PendingItem[];
  pending_review_ulangan?: PendingItem[];
  pending_review_ujian?: PendingItem[];
}

/** GET /api/v1/guru/pending-counts — cumulative cross-kelas. */
export async function getPendingCounts(): Promise<PendingCountsResponse> {
  return api<PendingCountsResponse>('/guru/pending-counts');
}

/** GET /api/v1/guru/pending-items — latest actionable rows per category. */
export async function getPendingItems(limit = 3): Promise<PendingItemsResponse> {
  return api<PendingItemsResponse>(`/guru/pending-items?limit=${limit}`);
}
