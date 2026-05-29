'use client';

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { LifeBuoy, Send } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  getSiswaAdminChat,
  markSiswaAdminChatRead,
  sendSiswaAdminChatMessage,
  type SiswaAdminChatPayload,
} from '@/lib/siswa-admin-chat-api';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

function formatDate(input?: string | null): string {
  if (!input) return '-';
  try {
    return new Date(input).toLocaleString('id-ID', {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: 'Asia/Jakarta',
    });
  } catch {
    return input;
  }
}

export default function BantuanAdminPage() {
  const queryClient = useQueryClient();
  const [body, setBody] = React.useState('');

  const chatQuery = useQuery({
    queryKey: ['siswa', 'bantuan-admin', 'chat'],
    queryFn: getSiswaAdminChat,
    refetchInterval: 8_000,
    staleTime: 3_000,
  });

  const conversation = chatQuery.data?.conversation;
  const messages = chatQuery.data?.messages ?? [];

  React.useEffect(() => {
    if (conversation && conversation.siswa_unread_count > 0) {
      void markSiswaAdminChatRead().then(() => {
        queryClient.setQueryData<SiswaAdminChatPayload>(['siswa', 'bantuan-admin', 'chat'], (old) =>
          old ? { ...old, conversation: { ...old.conversation, siswa_unread_count: 0 } } : old,
        );
      }).catch(() => undefined);
    }
  }, [conversation, queryClient]);

  const sendMutation = useMutation({
    mutationFn: (text: string) => sendSiswaAdminChatMessage(text),
    onSuccess: () => {
      setBody('');
      void queryClient.invalidateQueries({ queryKey: ['siswa', 'bantuan-admin', 'chat'] });
    },
  });

  return (
    <div className="space-y-6">
      <header className="rounded-siswa siswa-border siswa-shadow bg-siswa-blue p-5">
        <div className="flex flex-wrap items-center gap-3">
          <div className="grid size-12 place-items-center rounded-siswa siswa-border bg-white">
            <LifeBuoy className="size-6" />
          </div>
          <div>
            <h1 className="text-2xl font-black tracking-tight">Bantuan Admin</h1>
            <p className="text-sm font-semibold text-siswa-text/75">
              Tanya admin sekolah soal akun, kelas, akses, atau kendala teknis LMS.
            </p>
          </div>
        </div>
      </header>

      <Card className="siswa-border siswa-shadow overflow-hidden rounded-siswa bg-white">
        <CardHeader>
          <CardTitle>Chat dengan admin sekolah</CardTitle>
          <CardDescription>
            Pesan baru otomatis membuka ulang percakapan jika sebelumnya ditutup.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {chatQuery.isError ? (
            <div className="rounded-siswa siswa-border bg-siswa-yellow p-4 text-sm font-semibold">
              {chatQuery.error instanceof ApiError
                ? chatQuery.error.message
                : 'Gagal memuat Bantuan Admin.'}
            </div>
          ) : (
            <div className="max-h-[560px] space-y-3 overflow-y-auto rounded-siswa siswa-border bg-siswa-cream/60 p-3">
              {chatQuery.isPending ? (
                <div className="space-y-2">
                  {[0, 1, 2].map((i) => <div key={i} className="h-16 animate-pulse rounded-siswa bg-white" />)}
                </div>
              ) : messages.length === 0 ? (
                <div className="rounded-siswa siswa-border bg-white p-8 text-center text-sm font-semibold text-siswa-text/70">
                  Belum ada pesan. Kirim pertanyaan pertama kamu ke admin.
                </div>
              ) : (
                messages.map((msg) => {
                  const mine = msg.sender_role === 'siswa';
                  return (
                    <div key={msg.id} className={cn('flex', mine ? 'justify-end' : 'justify-start')}>
                      <div className={cn(
                        'max-w-[82%] rounded-siswa siswa-border px-3 py-2 text-sm siswa-shadow-sm',
                        mine ? 'bg-siswa-green' : 'bg-white',
                      )}>
                        <div className="mb-1 flex items-center gap-2 text-[11px] font-black uppercase tracking-wide text-siswa-text/60">
                          <span>{mine ? 'Kamu' : 'Admin'}</span>
                          <span>{formatDate(msg.created_at)}</span>
                        </div>
                        <p className="whitespace-pre-wrap leading-relaxed">{msg.body}</p>
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          )}

          <form
            className="space-y-2"
            onSubmit={(e) => {
              e.preventDefault();
              const text = body.trim();
              if (text) sendMutation.mutate(text);
            }}
          >
            <textarea
              value={body}
              onChange={(e) => setBody(e.target.value)}
              maxLength={4000}
              rows={3}
              placeholder="Tulis kendala kamu di sini..."
              className="min-h-24 w-full rounded-siswa siswa-border bg-white px-3 py-2 text-sm outline-none focus:ring-4 focus:ring-siswa-yellow"
            />
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-xs font-semibold text-siswa-text/60">{body.trim().length}/4000 karakter</p>
              <Button type="submit" disabled={sendMutation.isPending || body.trim().length === 0}>
                <Send className="size-4" />
                {sendMutation.isPending ? 'Mengirim...' : 'Kirim ke admin'}
              </Button>
            </div>
            {sendMutation.isError ? (
              <p className="text-sm font-semibold text-destructive">
                {sendMutation.error instanceof ApiError ? sendMutation.error.message : 'Pesan gagal dikirim.'}
              </p>
            ) : null}
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
