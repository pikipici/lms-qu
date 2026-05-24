'use client';

/**
 * /guru/kelas/detail/rekap?id=:id — Guru rekap matrix page (Task 7.B FE).
 *
 * Konsumen GET /kelas/:id/rekap (JSON). Tombol "Download CSV" pakai
 * downloadGuruKelasRekapCSV (auth-aware fetch + Blob save-as).
 *
 * Auth: kelasGroup admin/guru only — siswa kena 403 (ditangani guard route).
 */

import * as React from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, Download, RotateCcw, ScrollText, TrendingUp, Users } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  downloadGuruKelasRekapCSV,
  formatNilai,
  getGuruKelasRekap,
} from '@/lib/nilai-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { GuruRekapMatrix } from '@/components/guru/GuruRekapMatrix';

function totalClass(n: number | null): string {
  if (n === null) return 'text-muted-foreground';
  if (n >= 75) return 'text-emerald-700 dark:text-emerald-400';
  if (n >= 60) return 'text-amber-700 dark:text-amber-400';
  return 'text-rose-700 dark:text-rose-400';
}

function GuruRekapContent({ kelasID }: { kelasID: string }) {
  const { toast } = useToast();
  const [downloading, setDownloading] = React.useState(false);

  const rekapQ = useQuery({
    queryKey: ['guru', 'kelas', 'rekap', kelasID],
    queryFn: () => getGuruKelasRekap(kelasID),
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError) {
        if (err.status === 403 || err.status === 404 || err.status === 400) {
          return false;
        }
      }
      return failureCount < 2;
    },
  });

  const onDownloadCSV = async () => {
    setDownloading(true);
    try {
      await downloadGuruKelasRekapCSV(kelasID);
      toast({ title: 'CSV ter-download' });
    } catch (err) {
      const apiErr = err instanceof ApiError ? err : null;
      toast({
        title: 'Gagal download CSV',
        description: apiErr?.message ?? 'Coba lagi atau refresh halaman.',
        variant: 'destructive',
      });
    } finally {
      setDownloading(false);
    }
  };

  if (rekapQ.isPending && !rekapQ.data) {
    return (
      <div className="space-y-4">
        <div className="h-6 w-32 animate-pulse rounded bg-muted" />
        <div className="h-32 animate-pulse rounded-md border bg-muted/40" />
        <div className="h-64 animate-pulse rounded-md border bg-muted/40" />
      </div>
    );
  }

  if (rekapQ.isError) {
    const err = rekapQ.error;
    const apiErr = err instanceof ApiError ? err : null;
    const isForbidden = apiErr?.status === 403;
    const isNotFound = apiErr?.status === 404;
    const requestId = apiErr?.requestId;
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {isForbidden
              ? 'Akses ditolak'
              : isNotFound
                ? 'Kelas tidak ditemukan'
                : 'Gagal memuat rekap'}
          </CardTitle>
          <CardDescription>
            {isForbidden
              ? 'Lu bukan pemilik kelas ini. Hubungi admin kalau ini error.'
              : isNotFound
                ? 'Kelas mungkin di-archive atau ID tidak valid.'
                : (apiErr?.message ?? 'Terjadi kesalahan tidak terduga.')}
            {requestId ? ` (req: ${requestId})` : ''}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {!isForbidden && !isNotFound && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => rekapQ.refetch()}
              disabled={rekapQ.isFetching}
            >
              <RotateCcw className="size-4" />
              Coba lagi
            </Button>
          )}
          <Button asChild variant="ghost" size="sm">
            <Link href={`/guru/kelas/detail?id=${kelasID}`}>
              <ArrowLeft className="size-4" />
              Kembali ke kelas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  const data = rekapQ.data!;

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-2">
          <Button asChild variant="ghost" size="sm" className="-ml-3">
            <Link href={`/guru/kelas/detail?id=${kelasID}`}>
              <ArrowLeft className="size-4" />
              Kembali ke kelas
            </Link>
          </Button>
          <div className="flex flex-wrap items-center gap-2">
            <ScrollText className="size-5 text-muted-foreground" />
            <h1 className="text-2xl font-semibold tracking-tight">
              Rekap nilai · {data.kelas.nama}
            </h1>
          </div>
          <p className="text-sm text-muted-foreground">
            Matrix nilai siswa x bab + ulangan harian. Bobot diatur per tugas dan per ujian.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => rekapQ.refetch()}
            disabled={rekapQ.isFetching}
          >
            <RotateCcw className="size-4" />
            Refresh
          </Button>
          <Button
            type="button"
            size="sm"
            onClick={onDownloadCSV}
            disabled={downloading || data.rows.length === 0}
          >
            <Download className="size-4" />
            {downloading ? 'Mengunduh…' : 'Download CSV'}
          </Button>
        </div>
      </header>

      <div className="grid gap-4 sm:grid-cols-3">
        <Card>
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-sm">Total siswa</CardTitle>
              <CardDescription>Enrolment aktif</CardDescription>
            </div>
            <Users className="size-5 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <span className="text-2xl font-semibold tabular-nums">
              {data.summary.siswa_count}
            </span>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-sm">Sudah punya nilai</CardTitle>
              <CardDescription>Bab/ujian dinilai</CardDescription>
            </div>
            <TrendingUp className="size-5 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <span className="text-2xl font-semibold tabular-nums">
              {data.summary.siswa_with_nilai}
              <span className="ml-1 text-sm font-normal text-muted-foreground">
                / {data.summary.siswa_count}
              </span>
            </span>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-sm">Rata kelas</CardTitle>
              <CardDescription>
                Mean total siswa yang punya nilai
              </CardDescription>
            </div>
          </CardHeader>
          <CardContent>
            <span
              className={`text-2xl font-semibold tabular-nums ${totalClass(data.summary.kelas_avg)}`}
            >
              {formatNilai(data.summary.kelas_avg)}
            </span>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Matrix nilai</CardTitle>
          <CardDescription>
            Kolom kiri sticky. Sub-kolom per-bab: Total / Ulangan (Ul) / Tugas (Tg).
            Sub-kolom per-ulangan harian: Best / Last / N attempt. Sel kosong (—)
            artinya komponen belum dinilai.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <GuruRekapMatrix data={data} />
        </CardContent>
      </Card>
    </div>
  );
}

export default function GuruKelasRekapPage() {
  const searchParams = useSearchParams();
  const id = searchParams?.get('id') ?? '';

  if (!id) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">ID kelas tidak ada</CardTitle>
          <CardDescription>
            URL ini butuh parameter <code>?id=...</code>. Kembali ke daftar kelas.
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

  return <GuruRekapContent kelasID={id} />;
}
