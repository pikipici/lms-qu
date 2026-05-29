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

export async function listAdminChatConversations(params: {
  status?: ChatStatus | 'all';
  unread?: boolean;
  limit?: number;
  offset?: number;
} = {}): Promise<ChatConversationList> {
  const query = new URLSearchParams();
  if (params.status && params.status !== 'all') query.set('status', params.status);
  if (params.unread) query.set('unread', 'true');
  if (params.limit) query.set('limit', String(params.limit));
  if (params.offset) query.set('offset', String(params.offset));
  const suffix = query.toString() ? `?${query}` : '';
  return api<ChatConversationList>(`/admin/chat/conversations${suffix}`);
}

export async function getAdminChatMessages(
  conversationID: string,
): Promise<ChatThreadPayload> {
  const res = await api<Envelope<ChatThreadPayload>>(
    `/admin/chat/conversations/${conversationID}/messages`,
  );
  return res.data;
}

export async function sendAdminChatMessage(
  conversationID: string,
  body: string,
): Promise<ChatMessage> {
  const res = await api<Envelope<ChatMessage>>(
    `/admin/chat/conversations/${conversationID}/messages`,
    { method: 'POST', body: { body } },
  );
  return res.data;
}

export async function markAdminChatRead(conversationID: string): Promise<void> {
  await api(`/admin/chat/conversations/${conversationID}/read`, { method: 'POST' });
}

export async function setAdminChatStatus(
  conversationID: string,
  status: ChatStatus,
  version: number,
): Promise<ChatConversation> {
  const res = await api<Envelope<ChatConversation>>(
    `/admin/chat/conversations/${conversationID}/status`,
    { method: 'PATCH', body: { status, version } },
  );
  return res.data;
}
