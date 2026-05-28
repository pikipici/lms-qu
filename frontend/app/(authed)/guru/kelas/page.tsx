'use client';

/**
 * /guru/kelas — list kelas yang di-assign admin ke guru.
 *
 * Backend contract:
 *   GET /api/v1/kelas?page&page_size&include_archived&sekolah_id
 *     -> { items, page, page_size, total, total_pages }
 *
 * UX:
 *   - Card grid (1/2/3 col responsive).
 *   - Filter `include_archived` checkbox.
 *   - Pagination via Prev/Next + total info, mirrors /admin/pengguna.
 *   - Kode invite copy-to-clipboard from each card.
 */

import * as React from 'react';
import Link from 'next/link';
import {
  useQuery,
  keepPreviousData,
} from '@tanstack/react-query';
import {
  Archive,
  ArchiveRestore,
  ClipboardCheck,
  ClipboardCopy,
  RotateCcw,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type Kelas,
  listKelas,
} from '@/lib/kelas-api';
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
import { Label } from '@/components/ui/label';

const PAGE_SIZE = 12;

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

function KelasCard({ kelas }: { kelas: Kelas }) {
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

export default function GuruKelasListPage() {
  const [page, setPage] = React.useState(1);
  const [includeArchived, setIncludeArchived] = React.useState(false);
  const [selectedSekolahId, setSelectedSekolahId] = React.useState('');

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

  const items = kelasQuery.data?.items ?? [];
  const total = kelasQuery.data?.total ?? 0;
  const totalPages = kelasQuery.data?.total_pages ?? 0;

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">Kelas</h1>
          <p className="text-sm text-muted-foreground">
            Daftar kelas yang ditugaskan admin ke akun lu. Salin kode invite,
            atau buka detail untuk atur siswa dan materi.
          </p>
        </div>
      </header>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-3 space-y-0">
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
          <div className="flex flex-wrap items-center gap-3">
            <Label htmlFor="sekolah-filter" className="sr-only">
              Filter sekolah
            </Label>
            <select
              id="sekolah-filter"
              className="h-9 min-w-48 rounded-md border border-input bg-background px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
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
                ? 'Belum ada kelas.'
                : 'Belum ada kelas aktif. Centang "Tampilkan diarsipkan" atau hubungi admin.'}
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {items.map((k) => (
                <KelasCard key={k.id} kelas={k} />
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

    </div>
  );
}
