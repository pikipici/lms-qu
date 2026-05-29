import { api } from '@/lib/api';
import type { ChatMessage, ChatThreadPayload } from '@/lib/admin-chat-api';

interface Envelope<T> {
  data: T;
}

export type SiswaAdminChatPayload = ChatThreadPayload;

export async function getSiswaAdminChat(): Promise<SiswaAdminChatPayload> {
  const res = await api<Envelope<SiswaAdminChatPayload>>('/siswa/bantuan-admin/chat');
  return res.data;
}

export async function sendSiswaAdminChatMessage(body: string): Promise<ChatMessage> {
  const res = await api<Envelope<ChatMessage>>('/siswa/bantuan-admin/chat/messages', {
    method: 'POST',
    body: { body },
  });
  return res.data;
}

export async function markSiswaAdminChatRead(): Promise<void> {
  await api('/siswa/bantuan-admin/chat/read', { method: 'POST' });
}
