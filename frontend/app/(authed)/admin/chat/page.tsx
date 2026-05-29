'use client';

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { MessageCircle, RotateCcw, Send } from 'lucide-react';

import { ApiError } from '@/lib/api';
import { listKelas } from '@/lib/kelas-api';
import {
  getAdminChatMessages,
  listAdminChatConversations,
  markAdminChatRead,
  sendAdminChatMessage,
  setAdminChatStatus,
  type ChatConversation,
  type ChatScope,
  type ChatStatus,
  type ChatThreadPayload,
} from '@/lib/admin-chat-api';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

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

export default function AdminChatPage() {
  const queryClient = useQueryClient();
  const [selectedID, setSelectedID] = React.useState<string | null>(null);
  const [body, setBody] = React.useState('');
  const [status, setStatus] = React.useState<ChatStatus | 'all'>('all');
  const [scope, setScope] = React.useState<ChatScope>('kelas');
  const [unreadOnly, setUnreadOnly] = React.useState(false);
  const [kelasID, setKelasID] = React.useState('');

  const kelasQuery = useQuery({
    queryKey: ['admin', 'chat', 'kelas-filter'],
    queryFn: () => listKelas({ page: 1, pageSize: 100 }),
    staleTime: 60_000,
  });

  const listQuery = useQuery({
    queryKey: ['admin', 'chat', 'conversations', scope, status, unreadOnly, kelasID],
    queryFn: () => listAdminChatConversations({ scope, status, unread: unreadOnly, kelasID, limit: 100 }),
    refetchInterval: 12_000,
    staleTime: 5_000,
  });

  const conversations = React.useMemo(
    () => listQuery.data?.data ?? [],
    [listQuery.data?.data],
  );
  const selected = React.useMemo(
    () => conversations.find((it) => it.id === selectedID) ?? conversations[0],
    [conversations, selectedID],
  );

  React.useEffect(() => {
    if (!selectedID && conversations[0]?.id) setSelectedID(conversations[0].id);
  }, [conversations, selectedID]);

  const threadQuery = useQuery({
    queryKey: ['admin', 'chat', 'conversation', selected?.id],
    queryFn: () => getAdminChatMessages(selected!.id),
    enabled: Boolean(selected?.id),
    refetchInterval: 8_000,
    staleTime: 3_000,
  });

  React.useEffect(() => {
    if (selected?.id && selected.admin_unread_count > 0) {
      void markAdminChatRead(selected.id).then(() => {
        queryClient.setQueryData<ChatThreadPayload>(
          ['admin', 'chat', 'conversation', selected.id],
          (old) => old
            ? {
                ...old,
                conversation: { ...old.conversation, admin_unread_count: 0 },
              }
            : old,
        );
        void queryClient.invalidateQueries({ queryKey: ['admin', 'chat'] });
      }).catch(() => undefined);
    }
  }, [queryClient, selected?.admin_unread_count, selected?.id]);

  const sendMutation = useMutation({
    mutationFn: (text: string) => sendAdminChatMessage(selected!.id, text),
    onSuccess: () => {
      setBody('');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'chat'] });
    },
  });

  const statusMutation = useMutation({
    mutationFn: (input: { conversation: ChatConversation; status: ChatStatus }) =>
      setAdminChatStatus(input.conversation.id, input.status, input.conversation.version),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['admin', 'chat'] });
    },
  });

  const activeConversation = threadQuery.data?.conversation ?? selected;
  const messages = threadQuery.data?.messages ?? [];

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h1 className="flex items-center gap-2 text-2xl font-semibold tracking-tight">
            <MessageCircle className="size-6" />
            Monitor Chat
          </h1>
          <p className="text-sm text-muted-foreground">
            Pantau percakapan siswa dan guru lintas kelas. Admin bisa ikut membalas
            kalau perlu eskalasi.
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => listQuery.refetch()}
          disabled={listQuery.isFetching}
        >
          <RotateCcw className="size-4" />
          Refresh
        </Button>
      </header>

      <div className="flex flex-wrap items-center gap-2">
        {(['kelas', 'admin'] as const).map((item) => (
          <Button
            key={item}
            type="button"
            variant={scope === item ? 'default' : 'outline'}
            size="sm"
            onClick={() => {
              setScope(item);
              setKelasID('');
              setSelectedID(null);
            }}
          >
            {item === 'kelas' ? 'Chat kelas' : 'Bantuan Admin'}
          </Button>
        ))}
        {(['all', 'open', 'closed'] as const).map((item) => (
          <Button
            key={item}
            type="button"
            variant={status === item ? 'default' : 'outline'}
            size="sm"
            onClick={() => {
              setStatus(item);
              setSelectedID(null);
            }}
          >
            {item === 'all' ? 'Semua' : item === 'open' ? 'Terbuka' : 'Ditutup'}
          </Button>
        ))}
        <Button
          type="button"
          variant={unreadOnly ? 'default' : 'outline'}
          size="sm"
          onClick={() => {
            setUnreadOnly((v) => !v);
            setSelectedID(null);
          }}
        >
          Belum dibaca admin
        </Button>
        {scope === 'kelas' ? (
          <select
            value={kelasID}
            onChange={(e) => {
              setKelasID(e.target.value);
              setSelectedID(null);
            }}
            className="h-9 min-w-56 rounded-md border bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
            aria-label="Filter kelas"
          >
            <option value="">Semua kelas</option>
            {(kelasQuery.data?.items ?? []).map((kelas) => (
              <option key={kelas.id} value={kelas.id}>{kelas.nama}</option>
            ))}
          </select>
        ) : null}
      </div>

      <div className="grid gap-4 xl:grid-cols-[380px_minmax(0,1fr)]">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Percakapan</CardTitle>
            <CardDescription>
              {listQuery.data?.total ?? 0} conversation sesuai filter.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {listQuery.isPending ? (
              <div className="space-y-2">
                {[0, 1, 2].map((i) => (
                  <div key={i} className="h-24 animate-pulse rounded-md bg-muted" />
                ))}
              </div>
            ) : listQuery.isError ? (
              <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm">
                Gagal memuat daftar chat.
              </div>
            ) : conversations.length === 0 ? (
              <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
                Belum ada conversation untuk filter ini.
              </div>
            ) : (
              conversations.map((conv) => {
                const active = conv.id === activeConversation?.id;
                return (
                  <button
                    key={conv.id}
                    type="button"
                    onClick={() => setSelectedID(conv.id)}
                    className={cn(
                      'w-full rounded-md border p-3 text-left text-sm transition-colors hover:bg-muted/60',
                      active ? 'border-primary bg-muted' : 'bg-card',
                    )}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <p className="truncate font-medium">{conv.siswa_name || 'Siswa'}</p>
                      {conv.admin_unread_count > 0 ? (
                        <span className="rounded-full bg-primary px-2 py-0.5 text-xs font-semibold text-primary-foreground">
                          {conv.admin_unread_count}
                        </span>
                      ) : null}
                    </div>
                    <p className="mt-0.5 truncate text-xs text-muted-foreground">
                      {conv.scope === 'admin'
                        ? 'Bantuan Admin'
                        : `${conv.kelas_nama || 'Kelas'} · Guru ${conv.guru_name || '-'}`}
                    </p>
                    <p className="mt-2 line-clamp-2 text-xs text-muted-foreground">
                      {conv.last_message_preview || 'Belum ada pesan.'}
                    </p>
                    <div className="mt-2 flex items-center justify-between text-[11px] text-muted-foreground">
                      <span>{conv.status === 'closed' ? 'Ditutup' : 'Terbuka'}</span>
                      <span>{formatDate(conv.last_message_at)}</span>
                    </div>
                  </button>
                );
              })
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <CardTitle className="text-base">
                  {activeConversation?.siswa_name || 'Pilih percakapan'}
                </CardTitle>
                <CardDescription>
                  {activeConversation
                    ? activeConversation.scope === 'admin'
                      ? 'Bantuan Admin'
                      : `${activeConversation.kelas_nama || 'Kelas'} · Guru ${activeConversation.guru_name || '-'}`
                    : 'Pilih salah satu conversation di kiri.'}
                </CardDescription>
              </div>
              {activeConversation ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={statusMutation.isPending}
                  onClick={() => statusMutation.mutate({
                    conversation: activeConversation,
                    status: activeConversation.status === 'closed' ? 'open' : 'closed',
                  })}
                >
                  {activeConversation.status === 'closed' ? 'Buka lagi' : 'Tutup chat'}
                </Button>
              ) : null}
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            {!activeConversation ? (
              <div className="rounded-md border border-dashed p-10 text-center text-sm text-muted-foreground">
                Belum ada percakapan dipilih.
              </div>
            ) : threadQuery.isError ? (
              <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm">
                {threadQuery.error instanceof ApiError
                  ? threadQuery.error.message
                  : 'Gagal memuat pesan.'}
              </div>
            ) : (
              <div className="max-h-[560px] space-y-3 overflow-y-auto rounded-md border bg-muted/30 p-3">
                {threadQuery.isPending ? (
                  <div className="space-y-2">
                    {[0, 1, 2].map((i) => (
                      <div key={i} className="h-14 animate-pulse rounded-md bg-muted" />
                    ))}
                  </div>
                ) : messages.length === 0 ? (
                  <div className="rounded-md border border-dashed bg-background p-6 text-center text-sm text-muted-foreground">
                    Belum ada pesan.
                  </div>
                ) : (
                  messages.map((msg) => {
                    const mine = msg.sender_role === 'admin';
                    return (
                      <div key={msg.id} className={cn('flex', mine ? 'justify-end' : 'justify-start')}>
                        <div className={cn(
                          'max-w-[82%] rounded-lg border px-3 py-2 text-sm shadow-sm',
                          mine ? 'bg-primary text-primary-foreground' : 'bg-background',
                        )}>
                          <div className={cn(
                            'mb-1 flex items-center gap-2 text-[11px] font-medium uppercase tracking-wide',
                            mine ? 'text-primary-foreground/70' : 'text-muted-foreground',
                          )}>
                            <span>{mine ? 'Admin' : msg.sender_name || msg.sender_role}</span>
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

            {activeConversation ? (
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
                  placeholder="Balas sebagai admin..."
                  className="min-h-24 w-full rounded-md border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
                />
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <p className="text-xs text-muted-foreground">{body.trim().length}/4000 karakter</p>
                  <Button
                    type="submit"
                    size="sm"
                    disabled={sendMutation.isPending || body.trim().length === 0}
                  >
                    <Send className="size-4" />
                    {sendMutation.isPending ? 'Mengirim...' : 'Kirim sebagai admin'}
                  </Button>
                </div>
                {sendMutation.isError ? (
                  <p className="text-sm text-destructive">
                    {sendMutation.error instanceof ApiError
                      ? sendMutation.error.message
                      : 'Pesan gagal dikirim.'}
                  </p>
                ) : null}
              </form>
            ) : null}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
