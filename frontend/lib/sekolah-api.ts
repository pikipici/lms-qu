import { api } from '@/lib/api';

export interface Sekolah {
  id: string;
  nama: string;
  npsn?: string | null;
  alamat: string;
  created_at: string;
  updated_at: string;
}

export interface SekolahListResponse {
  items: Sekolah[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface SekolahInput {
  nama: string;
  npsn?: string;
  alamat?: string;
}

export async function listSekolah(params: {
  q?: string;
  page?: number;
  pageSize?: number;
} = {}): Promise<SekolahListResponse> {
  const q = new URLSearchParams();
  if (params.q) q.set('q', params.q);
  if (params.page) q.set('page', String(params.page));
  if (params.pageSize) q.set('page_size', String(params.pageSize));
  const qs = q.toString();
  return api<SekolahListResponse>(`/admin/sekolah${qs ? `?${qs}` : ''}`);
}

export async function createSekolah(input: SekolahInput): Promise<{ sekolah: Sekolah }> {
  return api<{ sekolah: Sekolah }>('/admin/sekolah', { method: 'POST', body: input });
}

export async function updateSekolah(id: string, input: SekolahInput): Promise<{ sekolah: Sekolah }> {
  return api<{ sekolah: Sekolah }>(`/admin/sekolah/${id}`, { method: 'PATCH', body: input });
}

export async function deleteSekolah(id: string): Promise<void> {
  await api<void>(`/admin/sekolah/${id}`, { method: 'DELETE' });
}
