import { api } from '@/lib/api';

export type ChatStatus = 'open' | 'closed';
export type ChatSenderRole = 'siswa' | 'guru' | 'admin';

export interface ChatConversation {
  id: string;
  kelas_id: string;
  siswa_id: string;
  guru_id: string;
  status: ChatStatus;
  last_message_at?: string | null;
  last_message_preview: string;
  siswa_unread_count: number;
  guru_unread_count: number;
  admin_unread_count: number;
  version: number;
  created_at: string;
  updated_at: string;
  siswa_name?: string;
  guru_name?: string;
  kelas_nama?: string;
}

export interface ChatMessage {
  id: string;
  conversation_id: string;
  sender_id: string;
  sender_role: ChatSenderRole;
  body: string;
  created_at: string;
  sender_name?: string;
}

export interface SiswaChatPayload {
  conversation: ChatConversation;
  messages: ChatMessage[];
}

interface Envelope<T> {
  data: T;
}

export async function getSiswaChat(kelasID: string): Promise<SiswaChatPayload> {
  const res = await api<Envelope<SiswaChatPayload>>(
    `/siswa/kelas/${kelasID}/chat`,
  );
  return res.data;
}

export async function sendSiswaChatMessage(
  kelasID: string,
  body: string,
): Promise<ChatMessage> {
  const res = await api<Envelope<ChatMessage>>(
    `/siswa/kelas/${kelasID}/chat/messages`,
    { method: 'POST', body: { body } },
  );
  return res.data;
}

export async function markSiswaChatRead(kelasID: string): Promise<void> {
  await api(`/siswa/kelas/${kelasID}/chat/read`, { method: 'POST' });
}
