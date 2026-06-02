import { api } from '@/lib/api';

export interface Rombel {
  id: string;
  sekolah_id: string;
  nama: string;
  deskripsi: string;
  active: boolean;
  version: number;
  archived_at?: string | null;
  created_at: string;
  updated_at: string;
  jumlah_siswa?: number;
}

export interface RombelListResponse {
  items: Rombel[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface RombelMember {
  siswa_id: string;
  nama: string;
  email: string;
  rombel_id: string;
  joined_via: string;
  joined_at: string;
}

export interface RombelMembersResponse {
  items: RombelMember[];
}

export async function listRombels(sekolahId: string, params: { pageSize?: number; includeArchived?: boolean } = {}): Promise<RombelListResponse> {
  const q = new URLSearchParams();
  if (params.pageSize) q.set('page_size', String(params.pageSize));
  if (params.includeArchived) q.set('include_archived', 'true');
  const qs = q.toString();
  return api<RombelListResponse>(`/admin/sekolah/${sekolahId}/rombels${qs ? `?${qs}` : ''}`);
}

export async function createRombel(sekolahId: string, input: { nama: string; deskripsi?: string }): Promise<{ rombel: Rombel }> {
  return api<{ rombel: Rombel }>(`/admin/sekolah/${sekolahId}/rombels`, { method: 'POST', body: input });
}

export async function updateRombel(id: string, input: { version: number; nama: string; deskripsi?: string }): Promise<{ rombel: Rombel }> {
  return api<{ rombel: Rombel }>(`/admin/rombels/${id}`, { method: 'PATCH', body: input });
}

export async function archiveRombel(id: string): Promise<{ rombel: Rombel }> {
  return api<{ rombel: Rombel }>(`/admin/rombels/${id}/archive`, { method: 'POST' });
}

export async function deleteRombel(id: string): Promise<void> {
  await api<void>(`/admin/rombels/${id}`, { method: 'DELETE' });
}

export async function listRombelMembers(id: string): Promise<RombelMembersResponse> {
  return api<RombelMembersResponse>(`/admin/rombels/${id}/members`);
}

export async function moveRombelMember(input: { siswa_id: string; to_rombel_id: string }): Promise<{ status: 'ok' }> {
  return api<{ status: 'ok' }>('/admin/rombels/members/move', { method: 'POST', body: input });
}
