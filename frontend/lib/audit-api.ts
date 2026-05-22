/**
 * Audit log API client (Task 7.E, locked #59).
 *
 * Single source untuk guru transparency audit log per kelas.
 *  - GET /api/v1/guru/kelas/:id/audit  → ListByKelas
 *  - GET /api/v1/guru/audit-actions    → ListActions (filter dropdown source)
 */

import { api } from '@/lib/api';

export interface AuditEntry {
  id: string;
  actor_id?: string | null;
  actor_name?: string | null;
  actor_role?: string | null;
  action: string;
  target_type?: string | null;
  target_id?: string | null;
  target_kelas_id?: string | null;
  meta?: Record<string, unknown> | null;
  at: string;
}

export interface AuditListResponse {
  events: AuditEntry[];
  total: number;
  limit: number;
  offset: number;
}

export interface AuditActionsResponse {
  actions: string[];
}

export interface ListByKelasParams {
  kelasId: string;
  action?: string;
  limit?: number;
  offset?: number;
}

export async function listKelasAudit(p: ListByKelasParams): Promise<AuditListResponse> {
  const qs = new URLSearchParams();
  if (p.action) qs.set('action', p.action);
  if (p.limit !== undefined) qs.set('limit', String(p.limit));
  if (p.offset !== undefined) qs.set('offset', String(p.offset));
  const q = qs.toString() ? `?${qs.toString()}` : '';
  return api<AuditListResponse>(`/guru/kelas/${p.kelasId}/audit${q}`);
}

export async function listAuditActions(): Promise<AuditActionsResponse> {
  return api<AuditActionsResponse>('/guru/audit-actions');
}
