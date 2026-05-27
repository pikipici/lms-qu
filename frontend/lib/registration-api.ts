import { api } from '@/lib/api';

export interface PublicSekolah {
  id: string;
  nama: string;
  siswa_registration_enabled: boolean;
  siswa_registration_mode: 'auto_approve' | 'approval_required';
}

export interface PublicKelas {
  id: string;
  nama: string;
}

export interface JoinRequest {
  id: string;
  siswa_id: string;
  siswa_name?: string;
  username?: string;
  sekolah_id: string;
  sekolah_nama?: string;
  kelas_id: string;
  kelas_nama?: string;
  status: 'pending' | 'approved' | 'rejected' | 'cancelled';
  requested_at: string;
  decided_at?: string;
  reject_reason?: string;
}

export interface RegisterSiswaInput {
  nama: string;
  username: string;
  password: string;
  password_confirm: string;
  sekolah_id: string;
  kelas_id: string;
}

export async function listPublicSekolah(): Promise<{ data: PublicSekolah[] }> {
  return api<{ data: PublicSekolah[] }>('/public/sekolah', { anon: true, skipRefresh: true });
}

export async function listPublicKelas(sekolahId: string): Promise<{ data: PublicKelas[] }> {
  return api<{ data: PublicKelas[] }>(`/public/sekolah/${sekolahId}/kelas`, { anon: true, skipRefresh: true });
}

export async function registerSiswa(input: RegisterSiswaInput): Promise<{
  status: 'registered';
  enrollment_status: 'active' | 'pending';
  message: string;
}> {
  return api<{
    status: 'registered';
    enrollment_status: 'active' | 'pending';
    message: string;
  }>('/auth/register-siswa', { method: 'POST', body: input, anon: true, skipRefresh: true });
}

export async function listJoinRequests(scope: 'admin' | 'guru', status = 'pending'): Promise<{ items: JoinRequest[] }> {
  return api<{ items: JoinRequest[] }>(`/${scope}/siswa-join-requests?status=${encodeURIComponent(status)}`);
}

export async function approveJoinRequest(scope: 'admin' | 'guru', id: string): Promise<{ status: 'ok' }> {
  return api<{ status: 'ok' }>(`/${scope}/siswa-join-requests/${id}/approve`, { method: 'POST' });
}

export async function rejectJoinRequest(scope: 'admin' | 'guru', id: string, reason = ''): Promise<{ status: 'ok' }> {
  return api<{ status: 'ok' }>(`/${scope}/siswa-join-requests/${id}/reject`, { method: 'POST', body: { reason } });
}
