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

export interface ChatThreadPayload {
  conversation: ChatConversation;
  messages: ChatMessage[];
}

export interface ChatConversationList {
  data: ChatConversation[];
  total: number;
}

interface Envelope<T> {
  data: T;
}

export async function listGuruChatConversations(
  kelasID: string,
): Promise<ChatConversationList> {
  return api<ChatConversationList>(`/kelas/${kelasID}/chat/conversations`);
}

export async function getGuruChatMessages(
  kelasID: string,
  conversationID: string,
): Promise<ChatThreadPayload> {
  const res = await api<Envelope<ChatThreadPayload>>(
    `/kelas/${kelasID}/chat/conversations/${conversationID}/messages`,
  );
  return res.data;
}

export async function sendGuruChatMessage(
  kelasID: string,
  conversationID: string,
  body: string,
): Promise<ChatMessage> {
  const res = await api<Envelope<ChatMessage>>(
    `/kelas/${kelasID}/chat/conversations/${conversationID}/messages`,
    { method: 'POST', body: { body } },
  );
  return res.data;
}

export async function markGuruChatRead(
  kelasID: string,
  conversationID: string,
): Promise<void> {
  await api(`/kelas/${kelasID}/chat/conversations/${conversationID}/read`, {
    method: 'POST',
  });
}

export async function setGuruChatStatus(
  kelasID: string,
  conversationID: string,
  status: ChatStatus,
  version: number,
): Promise<ChatConversation> {
  const res = await api<Envelope<ChatConversation>>(
    `/kelas/${kelasID}/chat/conversations/${conversationID}/status`,
    { method: 'PATCH', body: { status, version } },
  );
  return res.data;
}
