/**
 * Siswa API client — types + functions for /api/v1/siswa/* (Phase 2.C).
 *
 * Backend contracts:
 *   POST /siswa/kelas/join  → body {kode_invite}. 201 inserted=true (first), 200 inserted=false (idempotent).
 *                             400 kode_invite_required, 404 kode_invite_not_found, 409 kelas_archived,
 *                             409 enrollment_removed, 403 forbidden (not siswa).
 *   GET  /siswa/kelas       → list enrollment siswa current dgn detail kelas. Query: page, page_size.
 *                             Hides removed enrollments.
 */

import { api } from '@/lib/api';
import type { Kelas } from '@/lib/kelas-api';

export type JoinedVia = 'kode' | 'admin';

export interface MyKelasGuru {
  id: string;
  nama: string;
  email: string;
}

export interface MyKelasItem {
  kelas: Kelas;
  joined_at: string;
  joined_via: JoinedVia;
  guru?: MyKelasGuru;
}

export interface MyKelasListResponse {
  items: MyKelasItem[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface JoinByKodeResponse {
  kelas: Kelas;
  inserted: boolean;
}

export async function listMyKelas(params: {
  page?: number;
  pageSize?: number;
} = {}): Promise<MyKelasListResponse> {
  const q = new URLSearchParams();
  if (params.page) q.set('page', String(params.page));
  if (params.pageSize) q.set('page_size', String(params.pageSize));
  const qs = q.toString();
  return api<MyKelasListResponse>(`/siswa/kelas${qs ? `?${qs}` : ''}`);
}

export async function joinKelasByKode(
  kodeInvite: string,
): Promise<JoinByKodeResponse> {
  return api<JoinByKodeResponse>('/siswa/kelas/join', {
    method: 'POST',
    body: { kode_invite: kodeInvite },
  });
}
