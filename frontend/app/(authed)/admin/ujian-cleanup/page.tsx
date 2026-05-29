'use client';

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, Loader2, RefreshCw, ShieldAlert, Trash2 } from 'lucide-react';

import { ApiError } from '@/lib/api';
import { type Kelas, listKelas } from '@/lib/kelas-api';
import {
  type Ujian,
  forceDeleteUjianTesting,
  friendlyUjianError,
  listUjianByKelas,
} from '@/lib/ujian-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

function formatDate(value?: string | null) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return new Intl.DateTimeFormat('id-ID', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}

function statusLabel(status: Ujian['status']) {
  switch (status) {
    case 'draft':
      return 'Draft';
    case 'published':
      return 'Published';
    case 'archived':
      return 'Archived';
    default:
      return status;
  }
}

export default function AdminUjianCleanupPage() {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [kelasID, setKelasID] = React.useState('');

  const kelasQuery = useQuery({
    queryKey: ['admin', 'ujian-cleanup', 'kelas'],
    queryFn: () => listKelas({ page: 1, pageSize: 100, includeArchived: true }),
    staleTime: 30_000,
  });

  const kelasItems = kelasQuery.data?.items ?? [];
  const selectedKelas = kelasItems.find((kelas) => kelas.id === kelasID) ?? null;

  React.useEffect(() => {
    const firstKelasID = kelasItems[0]?.id;
    if (!kelasID && firstKelasID) {
      setKelasID(firstKelasID);
    }
  }, [kelasID, kelasItems]);

  const ujianQueryKey = React.useMemo(
    () => ['admin', 'ujian-cleanup', 'kelas', kelasID, 'ujian'] as const,
    [kelasID],
  );

  const ujianQuery = useQuery({
    queryKey: ujianQueryKey,
    queryFn: () => listUjianByKelas(kelasID, { limit: 100 }),
    enabled: Boolean(kelasID),
    staleTime: 10_000,
  });

  const forceDeleteMutation = useMutation({
    mutationFn: (ujian: Ujian) => forceDeleteUjianTesting(ujian.id),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ujianQueryKey });
      toast({
        title: 'Data testing ujian dihapus',
        description: `${res.hasil_deleted} attempt dan ${res.jawaban_deleted} jawaban ikut dibersihkan.`,
      });
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyUjianError(apiErr, 'force_delete_testing')
        : 'Gagal menghapus data testing ujian.';
      toast({
        title: 'Gagal hapus data testing',
        description: apiErr?.requestId ? `${message} (req: ${apiErr.requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const onForceDelete = (ujian: Ujian) => {
    const kelasName = selectedKelas?.nama ?? 'kelas ini';
    if (
      confirm(
        `Hapus data testing ujian "${ujian.judul}" di ${kelasName}? Semua attempt dan jawaban siswa ikut terhapus permanen.`,
      ) &&
      confirm('Konfirmasi sekali lagi: aksi ini hanya untuk testing/dev dan tidak bisa di-undo.')
    ) {
      forceDeleteMutation.mutate(ujian);
    }
  };

  return (
    <div className="space-y-6">
      <header className="space-y-2">
        <div className="inline-flex items-center gap-2 rounded-full border border-destructive/30 bg-destructive/5 px-3 py-1 text-xs font-medium text-destructive">
          <ShieldAlert className="size-3.5" />
          Admin/dev destructive cleanup
        </div>
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">Cleanup Data Testing Ujian</h1>
          <p className="max-w-3xl text-sm text-muted-foreground">
            Pilih kelas, lalu hapus ujian beserta attempt dan jawaban siswa yang dibuat untuk testing.
            Fitur ini tetap dikunci oleh backend: admin-only dan non-production only.
          </p>
        </div>
      </header>

      <Card className="border-destructive/20 bg-destructive/5">
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base text-destructive">
            <AlertTriangle className="size-4" />
            Peringatan
          </CardTitle>
          <CardDescription className="text-destructive/80">
            Aksi ini menghapus permanen data ujian, hasil/attempt, dan jawaban terkait. Jangan dipakai untuk data nilai asli.
          </CardDescription>
        </CardHeader>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Pilih kelas</CardTitle>
          <CardDescription>Admin dapat melihat kelas lintas guru, termasuk yang sudah archived.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {kelasQuery.isPending ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              Memuat kelas...
            </div>
          ) : kelasQuery.isError ? (
            <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
              Gagal memuat kelas. Coba refresh halaman.
            </div>
          ) : kelasItems.length === 0 ? (
            <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
              Belum ada kelas.
            </div>
          ) : (
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
              <select
                className="h-10 min-w-0 flex-1 rounded-md border bg-background px-3 text-sm outline-none ring-offset-background focus:ring-2 focus:ring-ring"
                value={kelasID}
                onChange={(event) => setKelasID(event.target.value)}
              >
                {kelasItems.map((kelas: Kelas) => (
                  <option key={kelas.id} value={kelas.id}>
                    {kelas.nama}{kelas.sekolah_nama ? ` - ${kelas.sekolah_nama}` : ''}{kelas.archived_at ? ' (archived)' : ''}
                  </option>
                ))}
              </select>
              <Button
                type="button"
                variant="outline"
                onClick={() => ujianQuery.refetch()}
                disabled={!kelasID || ujianQuery.isFetching}
              >
                {ujianQuery.isFetching ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                Refresh
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Daftar ujian</CardTitle>
          <CardDescription>
            {selectedKelas ? `Kelas: ${selectedKelas.nama}` : 'Pilih kelas untuk melihat ujian.'}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!kelasID ? (
            <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
              Pilih kelas dulu.
            </div>
          ) : ujianQuery.isPending ? (
            <div className="flex items-center justify-center py-12 text-muted-foreground">
              <Loader2 className="size-5 animate-spin" />
            </div>
          ) : ujianQuery.isError ? (
            <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
              Gagal memuat daftar ujian untuk kelas ini.
            </div>
          ) : (ujianQuery.data?.items ?? []).length === 0 ? (
            <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
              Belum ada ujian di kelas ini.
            </div>
          ) : (
            <div className="overflow-x-auto rounded-md border">
              <table className="w-full min-w-[760px] text-sm">
                <thead className="bg-muted/60 text-left text-xs uppercase tracking-wide text-muted-foreground">
                  <tr>
                    <th className="px-4 py-3 font-medium">Judul</th>
                    <th className="px-4 py-3 font-medium">Status</th>
                    <th className="px-4 py-3 font-medium">Mulai</th>
                    <th className="px-4 py-3 font-medium">Selesai</th>
                    <th className="px-4 py-3 text-right font-medium">Aksi</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {(ujianQuery.data?.items ?? []).map((ujian) => (
                    <tr key={ujian.id} className="bg-background">
                      <td className="px-4 py-3">
                        <div className="font-medium">{ujian.judul}</div>
                        <div className="text-xs text-muted-foreground">ID: {ujian.id}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className="rounded-full border px-2 py-1 text-xs text-muted-foreground">
                          {statusLabel(ujian.status)}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-muted-foreground">{formatDate(ujian.waktu_mulai)}</td>
                      <td className="px-4 py-3 text-muted-foreground">{formatDate(ujian.waktu_selesai)}</td>
                      <td className="px-4 py-3 text-right">
                        <Button
                          type="button"
                          variant="destructive"
                          size="sm"
                          onClick={() => onForceDelete(ujian)}
                          disabled={forceDeleteMutation.isPending}
                        >
                          {forceDeleteMutation.isPending ? (
                            <Loader2 className="size-4 animate-spin" />
                          ) : (
                            <Trash2 className="size-4" />
                          )}
                          Hapus data testing
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
