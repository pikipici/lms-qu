'use client';

/**
 * /guru/kelas/detail?id=:id — kelas detail + edit + duplicate + archive
 * (Fase 2.B.4).
 *
 * Static export (Next 14 `output: 'export'`) tidak izinkan dynamic route
 * tanpa generateStaticParams. Karena ID kelas runtime-only, kita pakai
 * query string seperti pola /admin/pengguna/detail.
 *
 * Backend contracts (commits c14640d → 620594f):
 *   GET    /api/v1/kelas/:id          → { kelas }
 *   PATCH  /api/v1/kelas/:id          body { version, nama, deskripsi? } → { kelas }
 *   POST   /api/v1/kelas/:id/archive  → { kelas } (idempotent: 409 already_archived)
 *   POST   /api/v1/kelas/:id/duplicate body { new_nama? } → 201 { kelas }
 *
 * Optimistic concurrency: PATCH 409 `version_conflict` → toast "konten ke-update
 * orang lain" + auto-refetch. Form di-reset ke state fresh.
 *
 * Locked decision #59 — guru audit scope: backend audit log entries
 * `kelas_*` punya `target_kelas_id`, jadi guru cuma lihat aksi pada kelas
 * miliknya. Tab placeholder Siswa/Pengaturan/Pengumuman akan terisi di
 * Task 2.C / 2.E.
 */

import * as React from 'react';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import {
  Archive,
  ArrowLeft,
  BookOpen,
  ClipboardCheck,
  ClipboardCopy,
  ClipboardList,
  Copy,
  GraduationCap,
  History,
  Megaphone,
  MessageCircle,
  RotateCcw,
  Send,
  ScrollText,
  Settings,
  Users,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type EnrollmentItem,
  type EnrollmentJoinedVia,
  type Kelas,
  archiveKelas,
  duplicateKelas,
  getKelas,
  listKelasEnrollments,
  updateKelas,
} from '@/lib/kelas-api';
import {
  getGuruChatMessages,
  listGuruChatConversations,
  markGuruChatRead,
  sendGuruChatMessage,
  setGuruChatStatus,
  type ChatConversation,
  type ChatThreadPayload,
} from '@/lib/guru-chat-api';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { BabListSection } from '@/components/bab/BabListSection';
import { PengumumanList } from '@/components/pengumuman/PengumumanList';
import { TugasList } from '@/components/tugas/TugasList';
import { UjianList } from '@/components/ujian/UjianList';

// ---------- Schema & helpers ----------

const editSchema = z.object({
  nama: z
    .string()
    .trim()
    .min(1, { message: 'Nama wajib diisi.' })
    .max(120, { message: 'Maksimal 120 karakter.' }),
  deskripsi: z.string().trim().max(500, { message: 'Maksimal 500 karakter.' }),
});

type EditForm = z.infer<typeof editSchema>;

const duplicateSchema = z.object({
  new_nama: z
    .string()
    .trim()
    .max(120, { message: 'Maksimal 120 karakter.' })
    .default(''),
});
type DuplicateForm = z.infer<typeof duplicateSchema>;

function friendlyUpdateError(err: ApiError): string {
  switch (err.code) {
    case 'invalid_input':
      return 'Input tidak valid. Cek nama kelas.';
    case 'version_conflict':
      return 'Kelas ini baru saja di-update orang lain. Form sudah di-refresh dengan data terbaru — silakan ulangi perubahan kamu.';
    case 'forbidden':
      return 'Kamu tidak memiliki akses ke kelas ini.';
    case 'not_found':
      return 'Kelas tidak ditemukan (mungkin sudah dihapus).';
    default:
      return err.message;
  }
}

function friendlyArchiveError(err: ApiError): string {
  switch (err.code) {
    case 'already_archived':
      return 'Kelas ini sudah diarsipkan.';
    case 'forbidden':
      return 'Kamu tidak memiliki akses ke kelas ini.';
    default:
      return err.message;
  }
}

function formatDate(input?: string | null): string {
  if (!input) return '—';
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

// ---------- Sub-components ----------

function CopyKodeButton({ kode }: { kode: string }) {
  const { toast } = useToast();
  const [copied, setCopied] = React.useState(false);

  const onCopy = React.useCallback(async () => {
    try {
      await navigator.clipboard.writeText(kode);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
      toast({ title: 'Kode invite tersalin', description: kode });
    } catch {
      toast({
        title: 'Gagal menyalin kode',
        description: 'Browser blok clipboard. Salin manual.',
        variant: 'destructive',
      });
    }
  }, [kode, toast]);

  return (
    <Button variant="outline" size="sm" onClick={onCopy} type="button">
      {copied ? (
        <>
          <ClipboardCheck className="size-4" />
          Tersalin
        </>
      ) : (
        <>
          <ClipboardCopy className="size-4" />
          Salin Kode
        </>
      )}
    </Button>
  );
}

function EditKelasForm({ kelas }: { kelas: Kelas }) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const defaults: EditForm = React.useMemo(
    () => ({
      nama: kelas.nama,
      deskripsi: kelas.deskripsi,
    }),
    [kelas.nama, kelas.deskripsi],
  );

  const form = useForm<EditForm>({
    resolver: zodResolver(editSchema),
    defaultValues: defaults,
  });

  // Re-sync form whenever kelas object changes (post-update / post-409 refetch).
  React.useEffect(() => {
    form.reset(defaults);
  }, [defaults, form]);

  const mutation = useMutation({
    mutationFn: (input: EditForm) =>
      updateKelas(kelas.id, {
        version: kelas.version,
        nama: input.nama.trim(),
        deskripsi: input.deskripsi.trim(),
      }),
    onSuccess: ({ kelas: updated }) => {
      // Patch cache so the form re-syncs to the new version + values.
      queryClient.setQueryData(['guru', 'kelas', 'detail', kelas.id], {
        kelas: updated,
      });
      queryClient.invalidateQueries({ queryKey: ['guru', 'kelas'] });
      toast({
        title: 'Kelas diperbarui',
        description: `Versi naik ke ${updated.version}.`,
      });
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        // Version conflict → refetch + show friendly toast.
        if (err.code === 'version_conflict') {
          queryClient.invalidateQueries({
            queryKey: ['guru', 'kelas', 'detail', kelas.id],
          });
        }
        const message = friendlyUpdateError(err);
        const requestId = err.requestId;
        toast({
          title:
            err.code === 'version_conflict'
              ? 'Kelas sudah berubah'
              : 'Gagal menyimpan perubahan',
          description: requestId ? `${message} (req: ${requestId})` : message,
          variant: 'destructive',
        });
      } else {
        toast({
          title: 'Gagal menyimpan perubahan',
          description: 'Terjadi kesalahan tidak terduga.',
          variant: 'destructive',
        });
      }
    },
  });

  const onSubmit = form.handleSubmit((values) => mutation.mutate(values));
  const isDirty = form.formState.isDirty;
  const isArchived = Boolean(kelas.archived_at);

  return (
    <Form {...form}>
      <form onSubmit={onSubmit} className="space-y-4">
        <FormField
          control={form.control}
          name="nama"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Nama kelas</FormLabel>
              <FormControl>
                <Input disabled={isArchived || mutation.isPending} {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="deskripsi"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Deskripsi</FormLabel>
              <FormControl>
                <textarea
                  className="flex min-h-[72px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                  placeholder="Catatan singkat tentang kelas ini."
                  disabled={isArchived || mutation.isPending}
                  {...field}
                />
              </FormControl>
              <FormDescription className="text-xs">
                Opsional. Maksimal 500 karakter.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormDescription className="text-xs">
          Versi saat ini: {kelas.version}.
        </FormDescription>
        <div className="flex items-center gap-2">
          <Button
            type="submit"
            size="sm"
            disabled={!isDirty || isArchived || mutation.isPending}
          >
            {mutation.isPending ? 'Menyimpan…' : 'Simpan perubahan'}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => form.reset(defaults)}
            disabled={!isDirty || mutation.isPending}
          >
            Batal
          </Button>
        </div>
        {isArchived && (
          <p className="text-xs text-muted-foreground">
            Kelas ini sudah diarsipkan. Edit dinonaktifkan — duplikat untuk
            membuat salinan baru.
          </p>
        )}
      </form>
    </Form>
  );
}

function ArchiveDialog({
  open,
  onOpenChange,
  kelas,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kelas: Kelas;
}) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const mutation = useMutation({
    mutationFn: () => archiveKelas(kelas.id),
    onSuccess: ({ kelas: updated }) => {
      queryClient.setQueryData(['guru', 'kelas', 'detail', kelas.id], {
        kelas: updated,
      });
      queryClient.invalidateQueries({ queryKey: ['guru', 'kelas'] });
      toast({
        title: 'Kelas diarsipkan',
        description: `${updated.nama} sekarang read-only.`,
      });
      onOpenChange(false);
    },
    onError: (err) => {
      const message =
        err instanceof ApiError ? friendlyArchiveError(err) : 'Gagal mengarsipkan kelas.';
      const requestId = err instanceof ApiError ? err.requestId : undefined;
      toast({
        title: 'Tidak bisa mengarsipkan',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Arsipkan kelas?</DialogTitle>
          <DialogDescription>
            <span className="font-medium">{kelas.nama}</span> akan
            dinonaktifkan: tidak muncul di list aktif, edit dimatikan, dan
            kode invite tetap ada untuk referensi. Kamu tetap bisa lihat dengan
            filter &quot;Tampilkan diarsipkan&quot;.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={mutation.isPending}
          >
            Batal
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending}
          >
            {mutation.isPending ? 'Mengarsipkan…' : 'Arsipkan'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DuplicateDialog({
  open,
  onOpenChange,
  kelas,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kelas: Kelas;
}) {
  const { toast } = useToast();
  const router = useRouter();
  const queryClient = useQueryClient();
  const form = useForm<DuplicateForm>({
    resolver: zodResolver(duplicateSchema),
    defaultValues: { new_nama: '' },
  });

  React.useEffect(() => {
    if (!open) form.reset({ new_nama: '' });
  }, [open, form]);

  const mutation = useMutation({
    mutationFn: (input: DuplicateForm) =>
      duplicateKelas(kelas.id, {
        new_nama: input.new_nama.trim() || undefined,
      }),
    onSuccess: ({ kelas: dup }) => {
      queryClient.invalidateQueries({ queryKey: ['guru', 'kelas'] });
      toast({
        title: 'Kelas berhasil diduplikasi',
        description: `${dup.nama} (kode: ${dup.kode_invite})`,
      });
      onOpenChange(false);
      router.push(`/guru/kelas/detail?id=${dup.id}`);
    },
    onError: (err) => {
      const message = err instanceof ApiError ? err.message : 'Gagal menduplikasi kelas.';
      const requestId = err instanceof ApiError ? err.requestId : undefined;
      toast({
        title: 'Tidak bisa menduplikasi',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const onSubmit = form.handleSubmit((values) => mutation.mutate(values));

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Duplikasi kelas</DialogTitle>
          <DialogDescription>
            Buat kelas baru dengan nama dan deskripsi yang sama seperti{' '}
            <span className="font-medium">{kelas.nama}</span>. Kode
            invite akan di-generate ulang. Siswa, materi, dan tugas tidak
            ikut tersalin (Fase 3).
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={onSubmit} className="space-y-4">
            <FormField
              control={form.control}
              name="new_nama"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Nama baru (opsional)</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={`${kelas.nama} (Salinan)`}
                      autoFocus
                      {...field}
                    />
                  </FormControl>
                  <FormDescription className="text-xs">
                    Kosongkan untuk pakai default &quot;Nama (Salinan)&quot;.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={mutation.isPending}
              >
                Batal
              </Button>
              <Button type="submit" disabled={mutation.isPending}>
                {mutation.isPending ? 'Menduplikasi…' : 'Duplikat'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

function PlaceholderTab({
  Icon,
  title,
  body,
  taskRef,
}: {
  Icon: React.ComponentType<{ className?: string }>;
  title: string;
  body: string;
  taskRef: string;
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Icon className="size-5 text-muted-foreground" />
          <CardTitle className="text-base">{title}</CardTitle>
        </div>
        <CardDescription>{body}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
          Akan tersedia di {taskRef}.
        </div>
      </CardContent>
    </Card>
  );
}

// ---------- Siswa tab (Task 2.C.4) ----------
//
// Read-only roster of active enrollments di kelas ini. Locked decision v0.7.2:
// guru gak punya tombol remove di MVP — admin scope, di-defer ke Fase 2 backlog
// atau v0.9. Kalau dibutuhkan: tambahkan endpoint admin-side dulu, jangan
// shortcut PATCH dari sini.

const ENROLLMENTS_PAGE_SIZE = 20;

function joinedViaLabel(via: EnrollmentJoinedVia): string {
  switch (via) {
    case 'admin':
      return 'Diundang admin';
    case 'kode':
      return 'Via kode invite';
    default:
      return via;
  }
}

function SiswaTab({ kelasID }: { kelasID: string }) {
  const [page, setPage] = React.useState(1);

  const query = useQuery({
    queryKey: ['guru', 'kelas', 'enrollments', kelasID, page],
    queryFn: () =>
      listKelasEnrollments(kelasID, {
        page,
        pageSize: ENROLLMENTS_PAGE_SIZE,
      }),
    placeholderData: (prev) => prev,
    staleTime: 15_000,
  });

  if (query.isPending) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Siswa</CardTitle>
          <CardDescription>Memuat daftar siswa…</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {[0, 1, 2].map((i) => (
              <div
                key={i}
                className="h-12 animate-pulse rounded-md border bg-muted/40"
              />
            ))}
          </div>
        </CardContent>
      </Card>
    );
  }

  if (query.isError) {
    const err = query.error;
    const isForbidden = err instanceof ApiError && err.code === 'forbidden';
    const requestId = err instanceof ApiError ? err.requestId : undefined;
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Gagal memuat daftar siswa</CardTitle>
          <CardDescription>
            {isForbidden
              ? 'Kamu hanya bisa lihat siswa di kelas yang kamu kelola.'
              : err instanceof ApiError
                ? err.message
                : 'Terjadi kesalahan tidak terduga.'}
            {requestId ? ` (req: ${requestId})` : ''}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => query.refetch()}
            disabled={query.isFetching}
          >
            <RotateCcw className="size-4" />
            Coba lagi
          </Button>
        </CardContent>
      </Card>
    );
  }

  const data = query.data!;
  const items: EnrollmentItem[] = data.items;
  const total = data.total;
  const totalPages = data.total_pages;

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div className="space-y-1.5">
          <CardTitle className="text-base">Siswa</CardTitle>
          <CardDescription>
            {total === 0
              ? 'Belum ada siswa terdaftar di kelas ini.'
              : `Total ${total} siswa aktif. Bagikan kode invite untuk
                  menambah peserta baru.`}
          </CardDescription>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => query.refetch()}
          disabled={query.isFetching}
        >
          <RotateCcw className="size-4" />
          Refresh
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        {items.length === 0 ? (
          <div className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">
            Belum ada siswa. Bagikan kode invite di header untuk mulai
            mengundang.
          </div>
        ) : (
          <div className="overflow-x-auto rounded-md border">
            <table className="w-full text-sm">
              <thead className="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="px-3 py-2 font-medium">Nama</th>
                  <th className="px-3 py-2 font-medium">Email</th>
                  <th className="px-3 py-2 font-medium">Bergabung via</th>
                  <th className="px-3 py-2 font-medium">Tanggal join</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.siswa_id} className="border-t">
                    <td className="px-3 py-2 font-medium">
                      {item.nama || (
                        <span className="text-muted-foreground">
                          (tanpa nama)
                        </span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-muted-foreground">
                      {item.email || '—'}
                    </td>
                    <td className="px-3 py-2 text-muted-foreground">
                      {joinedViaLabel(item.joined_via)}
                    </td>
                    <td className="px-3 py-2 text-muted-foreground">
                      {formatDate(item.joined_at)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {totalPages > 1 && (
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>
              Halaman {data.page} dari {totalPages}
            </span>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={page <= 1 || query.isFetching}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                Sebelumnya
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={page >= totalPages || query.isFetching}
                onClick={() => setPage((p) => p + 1)}
              >
                Berikutnya
              </Button>
            </div>
          </div>
        )}

        <p className="text-xs text-muted-foreground">
          Read-only di MVP. Untuk mengeluarkan siswa, hubungi admin.
        </p>
      </CardContent>
    </Card>
  );
}

function ChatTab({ kelasID }: { kelasID: string }) {
  const queryClient = useQueryClient();
  const [selectedID, setSelectedID] = React.useState<string | null>(null);
  const [body, setBody] = React.useState('');

  const listQuery = useQuery({
    queryKey: ['guru', 'kelas', 'chat', kelasID, 'conversations'],
    queryFn: () => listGuruChatConversations(kelasID),
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
    queryKey: ['guru', 'kelas', 'chat', kelasID, 'conversation', selected?.id],
    queryFn: () => getGuruChatMessages(kelasID, selected!.id),
    enabled: Boolean(selected?.id),
    refetchInterval: 8_000,
    staleTime: 3_000,
  });

  React.useEffect(() => {
    if (selected?.id && selected.guru_unread_count > 0) {
      void markGuruChatRead(kelasID, selected.id).then(() => {
        queryClient.setQueryData<ChatThreadPayload>(
          ['guru', 'kelas', 'chat', kelasID, 'conversation', selected.id],
          (old) => old
            ? {
                ...old,
                conversation: { ...old.conversation, guru_unread_count: 0 },
              }
            : old,
        );
        void queryClient.invalidateQueries({
          queryKey: ['guru', 'kelas', 'chat', kelasID, 'conversations'],
        });
      }).catch(() => undefined);
    }
  }, [kelasID, queryClient, selected?.guru_unread_count, selected?.id]);

  const sendMutation = useMutation({
    mutationFn: (text: string) => sendGuruChatMessage(kelasID, selected!.id, text),
    onSuccess: () => {
      setBody('');
      void queryClient.invalidateQueries({
        queryKey: ['guru', 'kelas', 'chat', kelasID],
      });
    },
  });

  const statusMutation = useMutation({
    mutationFn: (input: { conversation: ChatConversation; status: 'open' | 'closed' }) =>
      setGuruChatStatus(kelasID, input.conversation.id, input.status, input.conversation.version),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ['guru', 'kelas', 'chat', kelasID],
      });
    },
  });

  const messages = threadQuery.data?.messages ?? [];
  const activeConversation = threadQuery.data?.conversation ?? selected;

  return (
    <div className="grid gap-4 lg:grid-cols-[320px_minmax(0,1fr)]">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <MessageCircle className="size-4" />
            Inbox chat
          </CardTitle>
          <CardDescription>
            Percakapan siswa di kelas ini. Dibuat otomatis saat siswa mulai chat.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
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

          {listQuery.isPending ? (
            <div className="space-y-2">
              {[0, 1, 2].map((i) => (
                <div key={i} className="h-20 animate-pulse rounded-md bg-muted" />
              ))}
            </div>
          ) : conversations.length === 0 ? (
            <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
              Belum ada chat dari siswa.
            </div>
          ) : (
            <div className="space-y-2">
              {conversations.map((conv) => {
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
                      {conv.guru_unread_count > 0 ? (
                        <span className="rounded-full bg-primary px-2 py-0.5 text-xs font-semibold text-primary-foreground">
                          {conv.guru_unread_count}
                        </span>
                      ) : null}
                    </div>
                    <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                      {conv.last_message_preview || 'Belum ada pesan.'}
                    </p>
                    <div className="mt-2 flex items-center justify-between text-[11px] text-muted-foreground">
                      <span>{conv.status === 'closed' ? 'Ditutup' : 'Terbuka'}</span>
                      <span>{formatDate(conv.last_message_at)}</span>
                    </div>
                  </button>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle className="text-base">
                {activeConversation?.siswa_name || 'Pilih chat'}
              </CardTitle>
              <CardDescription>
                {activeConversation
                  ? `Status: ${activeConversation.status === 'closed' ? 'ditutup' : 'terbuka'}`
                  : 'Pilih salah satu percakapan di kiri.'}
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
              Belum ada percakapan aktif.
            </div>
          ) : threadQuery.isError ? (
            <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm">
              Gagal memuat pesan.
            </div>
          ) : (
            <div className="max-h-[520px] space-y-3 overflow-y-auto rounded-md border bg-muted/30 p-3">
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
                  const mine = msg.sender_role === 'guru';
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
                          <span>{mine ? 'Guru' : msg.sender_name || msg.sender_role}</span>
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
                placeholder="Balas pesan siswa..."
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
                  {sendMutation.isPending ? 'Mengirim...' : 'Kirim balasan'}
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
  );
}

// ---------- Page ----------

type TabKey =
  | 'bab'
  | 'pengaturan'
  | 'siswa'
  | 'pengumuman'
  | 'tugas'
  | 'ujian'
  | 'chat';

const TABS: { key: TabKey; label: string; Icon: React.ComponentType<{ className?: string }> }[] = [
  { key: 'bab', label: 'Bab', Icon: BookOpen },
  { key: 'pengaturan', label: 'Pengaturan', Icon: Settings },
  { key: 'siswa', label: 'Siswa', Icon: Users },
  { key: 'tugas', label: 'Tugas', Icon: ClipboardList },
  { key: 'ujian', label: 'Ulangan', Icon: GraduationCap },
  { key: 'pengumuman', label: 'Pengumuman', Icon: Megaphone },
  { key: 'chat', label: 'Chat', Icon: MessageCircle },
];

function isTabKey(value: string | null): value is TabKey {
  return TABS.some((tab) => tab.key === value);
}

function GuruKelasDetailContent({ id, initialTab }: { id: string; initialTab?: TabKey }) {
  const [tab, setTab] = React.useState<TabKey>(initialTab ?? 'bab');
  const [archiveOpen, setArchiveOpen] = React.useState(false);
  const [duplicateOpen, setDuplicateOpen] = React.useState(false);

  const detailQuery = useQuery({
    queryKey: ['guru', 'kelas', 'detail', id],
    queryFn: () => getKelas(id),
    staleTime: 15_000,
  });

  if (detailQuery.isPending) {
    return (
      <div className="space-y-4">
        <div className="h-8 w-48 animate-pulse rounded bg-muted" />
        <div className="h-32 animate-pulse rounded-md border bg-muted/40" />
        <div className="h-64 animate-pulse rounded-md border bg-muted/40" />
      </div>
    );
  }

  if (detailQuery.isError) {
    const err = detailQuery.error;
    const isNotFound = err instanceof ApiError && err.code === 'not_found';
    const isForbidden = err instanceof ApiError && err.code === 'forbidden';
    const requestId = err instanceof ApiError ? err.requestId : undefined;
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {isNotFound
              ? 'Kelas tidak ditemukan'
              : isForbidden
                ? 'Akses ditolak'
                : 'Gagal memuat kelas'}
          </CardTitle>
          <CardDescription>
            {isNotFound
              ? 'ID kelas tidak valid atau sudah dihapus.'
              : isForbidden
                ? 'Kamu hanya bisa lihat kelas yang kamu kelola.'
                : err instanceof ApiError
                  ? err.message
                  : 'Terjadi kesalahan tidak terduga.'}
            {requestId ? ` (req: ${requestId})` : ''}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline" size="sm">
            <Link href="/guru/kelas">
              <ArrowLeft className="size-4" />
              Kembali ke daftar kelas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  const kelas = detailQuery.data!.kelas;
  const archived = Boolean(kelas.archived_at);

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-2">
          <Button asChild variant="ghost" size="sm" className="-ml-3">
            <Link href="/guru/kelas">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </Button>
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              {kelas.nama}
            </h1>
            {archived ? (
              <span className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
                <Archive className="size-3" />
                Diarsipkan
              </span>
            ) : (
              <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-400">
                Aktif
              </span>
            )}
          </div>
          <p className="text-sm text-muted-foreground">
            Kode invite{' '}
            <span className="font-mono font-semibold tracking-wider text-foreground">
              {kelas.kode_invite}
            </span>{' '}
            · Dibuat {formatDate(kelas.created_at)}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <CopyKodeButton kode={kelas.kode_invite} />
          <Button
            variant="outline"
            size="sm"
            onClick={() => detailQuery.refetch()}
            disabled={detailQuery.isFetching}
            type="button"
          >
            <RotateCcw className="size-4" />
            Refresh
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setDuplicateOpen(true)}
            type="button"
          >
            <Copy className="size-4" />
            Duplikat
          </Button>
          {!archived && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setArchiveOpen(true)}
              type="button"
              className="text-destructive hover:text-destructive"
            >
              <Archive className="size-4" />
              Arsipkan
            </Button>
          )}
          <Button asChild variant="outline" size="sm">
            <Link href={`/guru/kelas/detail/rekap?id=${kelas.id}`}>
              <ScrollText className="size-4" />
              Rekap nilai
            </Link>
          </Button>
          <Button asChild variant="outline" size="sm">
            <Link href={`/guru/kelas/detail/audit?id=${kelas.id}`}>
              <History className="size-4" />
              Audit log
            </Link>
          </Button>
        </div>
      </header>

      {/* Tab nav */}
      <div className="flex gap-1 border-b">
        {TABS.map(({ key, label, Icon }) => {
          const active = tab === key;
          return (
            <button
              key={key}
              type="button"
              onClick={() => setTab(key)}
              className={cn(
                'flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm transition-colors',
                active
                  ? 'border-primary font-medium text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground',
              )}
            >
              <Icon className="size-4" />
              {label}
            </button>
          );
        })}
      </div>

      {/* Tab content */}
      {tab === 'bab' && <BabListSection kelasID={kelas.id} archived={archived} />}

      {tab === 'pengaturan' && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Pengaturan kelas</CardTitle>
            <CardDescription>
              Ubah nama dan deskripsi kelas. Perubahan dijaga lewat versi —
              kalau ada yang ngedit barengan, kamu dikasih warning untuk
              refresh dulu.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <EditKelasForm kelas={kelas} />
          </CardContent>
        </Card>
      )}

      {tab === 'siswa' && <SiswaTab kelasID={kelas.id} />}

      {tab === 'tugas' && (
        <TugasList
          kelasID={kelas.id}
          babID={null}
          contextLabel={`Tugas kelas-wide untuk ${kelas.nama}.`}
          disabled={archived}
        />
      )}

      {tab === 'ujian' && (
        <UjianList kelasID={kelas.id} disabled={archived} />
      )}

      {tab === 'pengumuman' && (
        <PengumumanList
          kelasID={kelas.id}
          babID={null}
          contextLabel={`Pengumuman ke seluruh siswa kelas ${kelas.nama}.`}
          disabled={archived}
        />
      )}

      {tab === 'chat' && <ChatTab kelasID={kelas.id} />}

      <ArchiveDialog
        open={archiveOpen}
        onOpenChange={setArchiveOpen}
        kelas={kelas}
      />
      <DuplicateDialog
        open={duplicateOpen}
        onOpenChange={setDuplicateOpen}
        kelas={kelas}
      />
    </div>
  );
}

export default function GuruKelasDetailPage() {
  const searchParams = useSearchParams();
  const id = searchParams?.get('id') ?? '';
  const tabParam = searchParams?.get('tab') ?? null;
  const initialTab = isTabKey(tabParam) ? tabParam : undefined;

  if (!id) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">ID kelas tidak ada</CardTitle>
          <CardDescription>
            URL ini butuh parameter <code>?id=...</code>. Kembali ke daftar
            kelas untuk pilih satu.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline" size="sm">
            <Link href="/guru/kelas">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  return <GuruKelasDetailContent id={id} initialTab={initialTab} />;
}
