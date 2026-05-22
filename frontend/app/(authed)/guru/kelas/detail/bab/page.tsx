'use client';

/**
 * /guru/kelas/detail/bab?id=:kelasID&bid=:babID — bab detail shell page
 * (Task 3.B.2).
 *
 * Static export (Next 14 `output: 'export'`) tidak izinkan dynamic route
 * tanpa generateStaticParams; mirror pola query-string dari
 * /guru/kelas/detail (Task 2.B.4) + /admin/pengguna/detail.
 *
 * Header: nama bab + status badge + breadcrumb (kelas → bab) + tombol
 * Refresh / Edit / Duplikat / Arsipkan (REUSE dialog dari Task 3.B.1
 * via `BabFormDialog`/`ArchiveBabDialog`/`DuplicateBabDialog`).
 *
 * Sub-tabs (placeholder pointer ke task lanjutan):
 *   - Materi    → Task 3.D.1 (FE Materi guru)
 *   - Soal      → Fase 5 (Soal Bab)
 *   - Tugas     → Fase 4 (Tugas)
 *   - Pengumuman→ tab Pengumuman (Task 3.F.2)
 *   - Pengaturan→ aktif sekarang: form basic edit (judul/nomor/deskripsi/status)
 *                  pakai mutation langsung (mini-version dari BabFormDialog body)
 *
 * Optimistic concurrency (#56): semua PATCH/POST kirim version dari snapshot
 * cache; 409 → toast + invalidate + form re-sync via React.useEffect.
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
  ClipboardList,
  Copy,
  FileText,
  Megaphone,
  Pencil,
  RotateCcw,
  Settings,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type Bab,
  friendlyBabError,
  getBab,
  updateBab,
} from '@/lib/bab-api';
import { type Kelas, getKelas } from '@/lib/kelas-api';
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
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { StatusBadge } from '@/components/bab/BabSortableCard';
import { BabFormDialog } from '@/components/bab/BabFormDialog';
import { ArchiveBabDialog } from '@/components/bab/ArchiveBabDialog';
import { DuplicateBabDialog } from '@/components/bab/DuplicateBabDialog';
import { MateriList } from '@/components/materi/MateriList';
import { PengumumanList } from '@/components/pengumuman/PengumumanList';
import { SoalBabList } from '@/components/soalbab/SoalBabList';
import { TugasList } from '@/components/tugas/TugasList';

// ---------- Helpers ----------

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

// ---------- Pengaturan tab (inline edit form) ----------

const settingsSchema = z.object({
  nomor: z.coerce
    .number({ invalid_type_error: 'Nomor harus angka.' })
    .int({ message: 'Nomor harus angka bulat.' })
    .min(1, { message: 'Minimal 1.' })
    .max(999, { message: 'Maksimal 999.' }),
  judul: z
    .string()
    .trim()
    .min(1, { message: 'Judul wajib diisi.' })
    .max(200, { message: 'Maksimal 200 karakter.' }),
  deskripsi: z
    .string()
    .trim()
    .max(2000, { message: 'Maksimal 2000 karakter.' }),
  status: z.enum(['draft', 'published']),
});

type SettingsForm = z.infer<typeof settingsSchema>;

function PengaturanTab({
  bab,
  invalidateKeys,
}: {
  bab: Bab;
  invalidateKeys: readonly (readonly unknown[])[];
}) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const archived = bab.status === 'archived';

  const defaults = React.useMemo<SettingsForm>(
    () => ({
      nomor: bab.nomor,
      judul: bab.judul,
      deskripsi: bab.deskripsi,
      status: archived ? 'draft' : (bab.status as 'draft' | 'published'),
    }),
    [bab.nomor, bab.judul, bab.deskripsi, bab.status, archived],
  );

  const form = useForm<SettingsForm>({
    resolver: zodResolver(settingsSchema),
    defaultValues: defaults,
  });

  React.useEffect(() => {
    form.reset(defaults);
  }, [defaults, form]);

  const mutation = useMutation({
    mutationFn: (input: SettingsForm) =>
      updateBab(bab.id, {
        version: bab.version,
        nomor: input.nomor,
        judul: input.judul,
        deskripsi: input.deskripsi,
        status: input.status,
      }),
    onSuccess: ({ bab: updated }) => {
      queryClient.setQueryData(['guru', 'bab', 'detail', bab.id], {
        bab: updated,
      });
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: 'Bab diperbarui',
        description: `Versi naik ke ${updated.version}.`,
      });
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      if (apiErr?.code === 'version_conflict') {
        for (const key of invalidateKeys) {
          queryClient.invalidateQueries({ queryKey: key });
        }
      }
      const message = apiErr
        ? friendlyBabError(apiErr, 'update')
        : 'Gagal menyimpan perubahan.';
      const requestId = apiErr?.requestId;
      toast({
        title:
          apiErr?.code === 'version_conflict'
            ? 'Bab sudah berubah'
            : 'Gagal menyimpan bab',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const onSubmit = form.handleSubmit((values) => mutation.mutate(values));
  const isDirty = form.formState.isDirty;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Pengaturan bab</CardTitle>
        <CardDescription>
          Ubah nomor, judul, deskripsi, dan status. Versi saat ini:{' '}
          {bab.version}. Untuk arsipkan bab, pakai tombol khusus di header.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form onSubmit={onSubmit} className="space-y-4">
            <div className="grid grid-cols-[5rem_1fr] gap-3">
              <FormField
                control={form.control}
                name="nomor"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Nomor</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min={1}
                        max={999}
                        disabled={archived || mutation.isPending}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="judul"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Judul</FormLabel>
                    <FormControl>
                      <Input
                        disabled={archived || mutation.isPending}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <FormField
              control={form.control}
              name="deskripsi"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Deskripsi</FormLabel>
                  <FormControl>
                    <textarea
                      className="flex min-h-[96px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                      placeholder="Catatan singkat tentang isi bab ini."
                      disabled={archived || mutation.isPending}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription className="text-xs">
                    Opsional. Maksimal 2000 karakter.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="status"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Status</FormLabel>
                  <FormControl>
                    <select
                      className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                      disabled={archived || mutation.isPending}
                      value={field.value}
                      onChange={(e) =>
                        field.onChange(
                          e.target.value as 'draft' | 'published',
                        )
                      }
                    >
                      <option value="draft">
                        Draft (siswa tidak melihat)
                      </option>
                      <option value="published">
                        Published (siswa lihat)
                      </option>
                    </select>
                  </FormControl>
                  <FormDescription className="text-xs">
                    Untuk mengarsipkan, pakai tombol Arsipkan di header.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            {archived && (
              <p className="text-xs text-muted-foreground">
                Bab ini sudah diarsipkan. Edit dinonaktifkan — duplikat untuk
                membuat salinan baru.
              </p>
            )}
            <div className="flex items-center gap-2">
              <Button
                type="submit"
                size="sm"
                disabled={!isDirty || archived || mutation.isPending}
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
          </form>
        </Form>
      </CardContent>
    </Card>
  );
}

// ---------- Placeholder tab ----------

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

// ---------- Page content ----------

type SubTabKey = 'materi' | 'soal' | 'tugas' | 'pengumuman' | 'pengaturan';

const SUB_TABS: {
  key: SubTabKey;
  label: string;
  Icon: React.ComponentType<{ className?: string }>;
}[] = [
  { key: 'materi', label: 'Materi', Icon: FileText },
  { key: 'soal', label: 'Soal', Icon: BookOpen },
  { key: 'tugas', label: 'Tugas', Icon: ClipboardList },
  { key: 'pengumuman', label: 'Pengumuman', Icon: Megaphone },
  { key: 'pengaturan', label: 'Pengaturan', Icon: Settings },
];

function GuruBabDetailContent({
  kelasID,
  babID,
}: {
  kelasID: string;
  babID: string;
}) {
  const router = useRouter();
  const [tab, setTab] = React.useState<SubTabKey>('materi');
  const [editOpen, setEditOpen] = React.useState(false);
  const [archiveOpen, setArchiveOpen] = React.useState(false);
  const [duplicateOpen, setDuplicateOpen] = React.useState(false);

  const detailQuery = useQuery({
    queryKey: ['guru', 'bab', 'detail', babID],
    queryFn: () => getBab(babID),
    staleTime: 15_000,
  });

  const kelasQuery = useQuery({
    queryKey: ['guru', 'kelas', 'detail', kelasID],
    queryFn: () => getKelas(kelasID),
    staleTime: 15_000,
  });

  const invalidateKeys = React.useMemo(
    () =>
      [
        ['guru', 'bab', 'detail', babID] as const,
        ['guru', 'kelas', 'bab', kelasID] as const,
      ] as const,
    [babID, kelasID],
  );

  if (detailQuery.isPending) {
    return (
      <div className="space-y-4">
        <div className="h-8 w-64 animate-pulse rounded bg-muted" />
        <div className="h-32 animate-pulse rounded-md border bg-muted/40" />
        <div className="h-64 animate-pulse rounded-md border bg-muted/40" />
      </div>
    );
  }

  if (detailQuery.isError) {
    const err = detailQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    const isNotFound = apiErr?.code === 'not_found';
    const isForbidden = apiErr?.code === 'forbidden';
    const requestId = apiErr?.requestId;
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {isNotFound
              ? 'Bab tidak ditemukan'
              : isForbidden
                ? 'Akses ditolak'
                : 'Gagal memuat bab'}
          </CardTitle>
          <CardDescription>
            {isNotFound
              ? 'ID bab tidak valid atau sudah dihapus.'
              : isForbidden
                ? 'Lu hanya bisa lihat bab di kelas yang lu kelola.'
                : apiErr
                  ? apiErr.message
                  : 'Terjadi kesalahan tidak terduga.'}
            {requestId ? ` (req: ${requestId})` : ''}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline" size="sm">
            <Link href={`/guru/kelas/detail?id=${kelasID}`}>
              <ArrowLeft className="size-4" />
              Kembali ke kelas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  const bab = detailQuery.data!.bab;
  const kelas: Kelas | undefined = kelasQuery.data?.kelas;
  const archived = bab.status === 'archived';

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-2">
          <Button asChild variant="ghost" size="sm" className="-ml-3">
            <Link href={`/guru/kelas/detail?id=${kelasID}`}>
              <ArrowLeft className="size-4" />
              {kelas ? `Kelas ${kelas.nama}` : 'Kembali ke kelas'}
            </Link>
          </Button>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm text-muted-foreground">
              Bab {bab.nomor}
            </span>
            <h1 className="text-2xl font-semibold tracking-tight">
              {bab.judul}
            </h1>
            <StatusBadge status={bab.status} />
          </div>
          <p className="text-sm text-muted-foreground">
            Versi {bab.version} · Dibuat {formatDate(bab.created_at)} ·
            Diperbarui {formatDate(bab.updated_at)}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
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
            onClick={() => setEditOpen(true)}
            disabled={archived}
            type="button"
          >
            <Pencil className="size-4" />
            Edit
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
        </div>
      </header>

      {/* Sub-tab nav */}
      <div className="flex flex-wrap gap-1 border-b">
        {SUB_TABS.map(({ key, label, Icon }) => {
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

      {/* Sub-tab content */}
      {tab === 'materi' && (
        <MateriList
          kelasID={kelasID}
          babID={babID}
          contextLabel={`Materi untuk Bab ${bab.nomor} — ${bab.judul}.`}
          disabled={archived}
        />
      )}
      {tab === 'soal' && (
        <SoalBabList
          babID={babID}
          contextLabel={`Bank soal untuk Bab ${bab.nomor} — ${bab.judul}.`}
          disabled={archived}
        />
      )}
      {tab === 'tugas' && (
        <TugasList
          kelasID={kelasID}
          babID={babID}
          contextLabel={`Tugas untuk Bab ${bab.nomor} — ${bab.judul}.`}
          disabled={archived}
        />
      )}
      {tab === 'pengumuman' && (
        <PengumumanList
          kelasID={kelasID}
          babID={babID}
          contextLabel={`Pengumuman untuk Bab ${bab.nomor} — ${bab.judul}.`}
          disabled={archived}
        />
      )}
      {tab === 'pengaturan' && (
        <PengaturanTab bab={bab} invalidateKeys={invalidateKeys} />
      )}

      <BabFormDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        mode="edit"
        kelasID={kelasID}
        bab={bab}
        invalidateKeys={invalidateKeys}
      />
      <ArchiveBabDialog
        open={archiveOpen}
        onOpenChange={setArchiveOpen}
        bab={bab}
        invalidateKeys={invalidateKeys}
      />
      <DuplicateBabDialog
        open={duplicateOpen}
        onOpenChange={setDuplicateOpen}
        bab={bab}
        invalidateKeys={invalidateKeys}
        onSuccess={(dup) => {
          router.push(
            `/guru/kelas/detail/bab?id=${dup.kelas_id}&bid=${dup.id}`,
          );
        }}
      />
    </div>
  );
}

export default function GuruBabDetailPage() {
  const searchParams = useSearchParams();
  const kelasID = searchParams?.get('id') ?? '';
  const babID = searchParams?.get('bid') ?? '';

  if (!kelasID || !babID) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Parameter tidak lengkap</CardTitle>
          <CardDescription>
            URL ini butuh parameter <code>?id=&lt;kelas_id&gt;&amp;bid=&lt;bab_id&gt;</code>
            . Kembali ke daftar bab di kelas detail untuk pilih satu.
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

  return <GuruBabDetailContent kelasID={kelasID} babID={babID} />;
}
