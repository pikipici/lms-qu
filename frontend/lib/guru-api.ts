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

/** GET /api/v1/guru/pending-counts — cumulative cross-kelas. */
export async function getPendingCounts(): Promise<PendingCountsResponse> {
  return api<PendingCountsResponse>('/guru/pending-counts');
}
