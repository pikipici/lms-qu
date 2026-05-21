/**
 * Guru-side API client (Task 4.E.2 — pending counters partial; activity
 * feed full deferred ke Fase 7).
 */

import { api } from '@/lib/api';

export interface PendingCountsResponse {
  ungraded_submissions: number;
  // Forward-compat (Fase 5/7): pending_review_ulangan_bab,
  // pending_review_ulangan_harian akan di-add tanpa breaking shape.
}

/** GET /api/v1/guru/pending-counts — cumulative cross-kelas. */
export async function getPendingCounts(): Promise<PendingCountsResponse> {
  return api<PendingCountsResponse>('/guru/pending-counts');
}
