'use client';

/**
 * /guru/kelas — list + create kelas (Fase 2.B.3).
 *
 * Backend contract (commits c14640d → 620594f):
 *   GET  /api/v1/kelas?page&page_size&include_archived&sekolah_id
 *     -> { items, page, page_size, total, total_pages }
 *   POST /api/v1/kelas { nama, deskripsi?, sekolah_id? }
 *     -> 201 { kelas }
 *
 * UX:
 *   - Card grid (1/2/3 col responsive).
 *   - Filter `include_archived` checkbox.
 *   - Pagination via Prev/Next + total info, mirrors /admin/pengguna.
 *   - "+ Buat Kelas Baru" opens shadcn Dialog with react-hook-form + zod.
 *   - Kode invite copy-to-clipboard from each card.
 *   - 2.B.4 (detail/edit) belum dibangun → tombol "Detail" disabled w/ note.
 */

import * as React from 'react';
import Link from 'next/link';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  useMutation,
  useQuery,
  keepPreviousData,
  useQueryClient,
} from '@tanstack/react-query';
import {
  Archive,
  ArchiveRestore,
  ClipboardCheck,
  ClipboardCopy,
  MessageCircle,
  Plus,
  RotateCcw,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type Kelas,
  createKelas,
  listKelas,
} from '@/lib/kelas-api';
import { listGuruChatUnread } from '@/lib/guru-chat-api';
import { listSekolahOptions } from '@/lib/sekolah-api';
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
import { Label } from '@/components/ui/label';

const PAGE_SIZE = 12;

const createSchema = z.object({
  nama: z
    .string()
    .trim()
    .min(1, { message: 'Nama wajib diisi.' })
    .max(120, { message: 'Maksimal 120 karakter.' }),
  deskripsi: z.string().trim().max(500, { message: 'Maksimal 500 karakter.' }),
  sekolah_id: z.string().trim(),
});

type CreateForm = z.infer<typeof createSchema>;

function friendlyCreateError(err: ApiError): string {
  switch (err.code) {
    case 'invalid_input':
      return 'Input tidak valid. Cek kembali nama kelas.';
    case 'invalid_bobot':
      return 'Konfigurasi bobot kelas tidak valid.';
    case 'kode_invite_collision':
      return 'Server gagal generate kode invite (collision). Coba lagi.';
    case 'forbidden':
      return 'Akun kamu tidak diizinkan membuat kelas baru.';
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

function KelasCard({ kelas, unread = 0 }: { kelas: Kelas; unread?: number }) {
  const { toast } = useToast();
  const [copied, setCopied] = React.useState(false);

  const onCopy = React.useCallback(async () => {
    try {
      await navigator.clipboard.writeText(kelas.kode_invite);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
      toast({ title: 'Kode invite tersalin', description: kelas.kode_invite });
    } catch {
      toast({
        title: 'Gagal menyalin kode',
        description: 'Browser blok clipboard. Salin manual.',
        variant: 'destructive',
      });
    }
  }, [kelas.kode_invite, toast]);

  const archived = Boolean(kelas.archived_at);

  return (
    <Card className={cn('flex flex-col', archived && 'opacity-70')}>
      <CardHeader className="space-y-1.5 pb-3">
        <div className="flex items-start justify-between gap-2">
          <CardTitle className="text-base leading-tight">{kelas.nama}</CardTitle>
          <div className="flex shrink-0 flex-col items-end gap-1.5">
            {unread > 0 ? (
              <span className="inline-flex items-center gap-1 rounded-full bg-amber-500/15 px-2 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-300">
                <MessageCircle className="size-3" />
                {unread} pesan
              </span>
            ) : null}
            {archived ? (
              <span className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
                <Archive className="size-3" />
                Diarsipkan
              </span>
            ) : (
              <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-400">
                <ArchiveRestore className="size-3" />
                Aktif
              </span>
            )}
          </div>
        </div>
        {kelas.deskripsi ? (
          <CardDescription className="line-clamp-2">
            {kelas.deskripsi}
          </CardDescription>
        ) : (
          <CardDescription className="italic text-muted-foreground/70">
            Tidak ada deskripsi.
          </CardDescription>
        )}
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-3 pb-4">
        <div className="space-y-2 rounded-md border bg-muted/30 p-3">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs uppercase tracking-wide text-muted-foreground">
              Kode Invite
            </span>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 gap-1.5 px-2 text-xs"
              onClick={onCopy}
              type="button"
            >
              {copied ? (
                <>
                  <ClipboardCheck className="size-3.5" />
                  Tersalin
                </>
              ) : (
                <>
                  <ClipboardCopy className="size-3.5" />
                  Salin
                </>
              )}
            </Button>
          </div>
          <p className="font-mono text-lg font-semibold tracking-wider">
            {kelas.kode_invite}
          </p>
        </div>

        <dl className="grid grid-cols-2 gap-x-3 gap-y-1.5 text-xs">
          <dt className="text-muted-foreground">Sekolah</dt>
          <dd className="text-right font-medium">
            {kelas.sekolah_nama || 'Tanpa sekolah'}
          </dd>
          <dt className="text-muted-foreground">Jumlah murid</dt>
          <dd className="text-right font-medium">
            {kelas.jumlah_murid ?? 0} murid
          </dd>
          <dt className="text-muted-foreground">Dibuat</dt>
          <dd className="text-right text-muted-foreground">
            {formatDate(kelas.created_at)}
          </dd>
        </dl>

        <Button asChild variant="outline" size="sm" className="mt-auto">
          <Link href={`/guru/kelas/detail?id=${kelas.id}`}>Detail</Link>
        </Button>
      </CardContent>
    </Card>
  );
}

function CreateKelasDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
}) {
  const { toast } = useToast();
  const form = useForm<CreateForm>({
    resolver: zodResolver(createSchema),
    defaultValues: {
      nama: '',
      deskripsi: '',
      sekolah_id: '',
    },
  });

  React.useEffect(() => {
    if (!open) form.reset();
  }, [open, form]);

  const sekolahQuery = useQuery({
    queryKey: ['sekolah-options'],
    queryFn: () => listSekolahOptions({ pageSize: 100 }),
    enabled: open,
    staleTime: 60_000,
  });

  const mutation = useMutation({
    mutationFn: (input: CreateForm) =>
      createKelas({
        nama: input.nama.trim(),
        deskripsi: input.deskripsi.trim() || undefined,
        sekolah_id: input.sekolah_id || undefined,
      }),
    onSuccess: ({ kelas }) => {
      toast({
        title: 'Kelas dibuat',
        description: `Kode invite: ${kelas.kode_invite}`,
      });
      onCreated();
      onOpenChange(false);
    },
    onError: (err) => {
      const message =
        err instanceof ApiError ? friendlyCreateError(err) : 'Gagal membuat kelas.';
      const requestId = err instanceof ApiError ? err.requestId : undefined;
      toast({
        title: 'Tidak bisa membuat kelas',
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
          <DialogTitle>Buat kelas baru</DialogTitle>
          <DialogDescription>
            Isi nama kelas. Kode invite akan di-generate otomatis (6 karakter,
            hindari karakter ambigu seperti O/0/I/1).
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={onSubmit} className="space-y-4">
            <FormField
              control={form.control}
              name="nama"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Nama kelas</FormLabel>
                  <FormControl>
                    <Input
                      autoFocus
                      placeholder="Misal: Matematika 7A 2026/2027"
                      {...field}
                    />
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
                  <FormLabel>Deskripsi (opsional)</FormLabel>
                  <FormControl>
                    <textarea
                      className="flex min-h-[64px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                      placeholder="Catatan singkat tentang kelas ini."
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="sekolah_id"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Sekolah (opsional)</FormLabel>
                  <FormControl>
                    <select
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                      {...field}
                    >
                      <option value="">Tanpa sekolah</option>
                      {(sekolahQuery.data?.items ?? []).map((s) => (
                        <option key={s.id} value={s.id}>{s.nama}</option>
                      ))}
                    </select>
                  </FormControl>
                  <FormDescription className="text-xs">
                    Pilihan ini muncul dari master sekolah admin.
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
                {mutation.isPending ? 'Menyimpan…' : 'Buat kelas'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

export default function GuruKelasListPage() {
  const [page, setPage] = React.useState(1);
  const [includeArchived, setIncludeArchived] = React.useState(false);
  const [selectedSekolahId, setSelectedSekolahId] = React.useState('');
  const [createOpen, setCreateOpen] = React.useState(false);
  const queryClient = useQueryClient();

  React.useEffect(() => {
    setPage(1);
  }, [includeArchived, selectedSekolahId]);

  const sekolahQuery = useQuery({
    queryKey: ['sekolah-options'],
    queryFn: () => listSekolahOptions({ pageSize: 100 }),
    staleTime: 60_000,
  });

  const kelasQuery = useQuery({
    queryKey: ['guru', 'kelas', { page, includeArchived, selectedSekolahId }],
    queryFn: () =>
      listKelas({
        page,
        pageSize: PAGE_SIZE,
        includeArchived,
        sekolahId: selectedSekolahId || undefined,
      }),
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });
  const unreadQ = useQuery({
    queryKey: ['guru', 'chat', 'unread'],
    queryFn: listGuruChatUnread,
    refetchInterval: 30_000,
    staleTime: 10_000,
  });

  const items = kelasQuery.data?.items ?? [];
  const unreadByKelas = React.useMemo(() => {
    const map = new Map<string, number>();
    for (const row of unreadQ.data ?? []) map.set(row.kelas_id, row.unread);
    return map;
  }, [unreadQ.data]);

  const total = kelasQuery.data?.total ?? 0;
  const totalPages = kelasQuery.data?.total_pages ?? 0;

  const onCreated = React.useCallback(() => {
    setPage(1);
    queryClient.invalidateQueries({ queryKey: ['guru', 'kelas'] });
  }, [queryClient]);

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">Kelas</h1>
          <p className="text-sm text-muted-foreground">
            Daftar kelas yang kamu kelola. Buat kelas baru, salin kode invite,
            atau buka detail untuk atur siswa dan materi (segera).
          </p>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="size-4" />
          Buat kelas baru
        </Button>
      </header>

      <Card>
        <CardHeader className="flex flex-col items-stretch justify-between gap-3 space-y-0 sm:flex-row sm:items-center">
          <div className="space-y-1">
            <CardTitle className="text-base">Filter</CardTitle>
            <CardDescription>
              {kelasQuery.isPending
                ? 'Memuat…'
                : `Total ${total} kelas${
                    totalPages > 1 ? ` • Halaman ${page} / ${totalPages}` : ''
                  }`}
            </CardDescription>
          </div>
          <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
            <Label htmlFor="sekolah-filter" className="sr-only">
              Filter sekolah
            </Label>
            <select
              id="sekolah-filter"
              className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring sm:min-w-48"
              value={selectedSekolahId}
              onChange={(e) => setSelectedSekolahId(e.target.value)}
              disabled={sekolahQuery.isLoading}
            >
              <option value="">Semua sekolah</option>
              {(sekolahQuery.data?.items ?? []).map((s) => (
                <option key={s.id} value={s.id}>{s.nama}</option>
              ))}
            </select>
            <Label
              htmlFor="include-archived"
              className="flex cursor-pointer items-center gap-2 text-xs text-muted-foreground"
            >
              <input
                id="include-archived"
                type="checkbox"
                className="size-4 rounded border-input"
                checked={includeArchived}
                onChange={(e) => setIncludeArchived(e.target.checked)}
              />
              Tampilkan diarsipkan
            </Label>
            <Button
              variant="outline"
              size="sm"
              onClick={() => kelasQuery.refetch()}
              disabled={kelasQuery.isFetching}
              className="w-full sm:w-auto"
            >
              <RotateCcw className="size-4" />
              Refresh
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {kelasQuery.isPending ? (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {Array.from({ length: 3 }).map((_, i) => (
                <div
                  key={i}
                  className="h-56 animate-pulse rounded-md border bg-muted/40"
                />
              ))}
            </div>
          ) : kelasQuery.isError ? (
            <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
              {kelasQuery.error instanceof ApiError &&
              kelasQuery.error.requestId
                ? `Gagal memuat daftar kelas (req: ${kelasQuery.error.requestId}).`
                : 'Gagal memuat daftar kelas.'}
            </div>
          ) : items.length === 0 ? (
            <div className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">
              {includeArchived
                ? 'Belum ada kelas. Buat kelas pertama kamu sekarang.'
                : 'Belum ada kelas aktif. Centang "Tampilkan diarsipkan" atau buat kelas baru.'}
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {items.map((k) => (
                <KelasCard key={k.id} kelas={k} unread={unreadByKelas.get(k.id) ?? 0} />
              ))}
            </div>
          )}

          <div className="mt-4 flex flex-wrap items-center justify-end gap-2 text-sm text-muted-foreground">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1 || kelasQuery.isFetching}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
            >
              Prev
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={
                totalPages > 0 ? page >= totalPages : items.length < PAGE_SIZE
              }
              onClick={() => setPage((p) => p + 1)}
            >
              Next
            </Button>
          </div>
        </CardContent>
      </Card>

      <CreateKelasDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={onCreated}
      />
    </div>
  );
}
