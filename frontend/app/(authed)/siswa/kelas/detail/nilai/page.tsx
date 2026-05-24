'use client';

/**
 * /siswa/kelas/detail/nilai?id=:id — siswa rekap nilai per-kelas (Task 7.A.2).
 *
 * Static export pattern: query-string id mirror /siswa/kelas/detail dst.
 *
 * Sumber data:
 *   - GET /siswa/kelas (hydrate kelas info untuk header + back link)
 *   - GET /siswa/kelas/:id/nilai (rekap detail bab + ulangan harian)
 *
 * Render:
 *   - Header: nama kelas + back ke /siswa/kelas/detail?id=:id + total kelas
 *   - SiswaNilaiBabTable (bab breakdown ulangan/tugas/total)
 *   - SiswaNilaiUjianList (ulangan harian aggregate)
 *
 * Error handling:
 *   - 403/404/400 → render forbidden card (siswa belum enrol / kelas tidak ada)
 *   - 5xx → retry button + request_id
 */

import * as React from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, RotateCcw, TrendingUp } from 'lucide-react';

import { ApiError } from '@/lib/api';
import { listMyKelas, type MyKelasItem } from '@/lib/siswa-api';
import {
  formatNilai,
  getSiswaKelasNilai,
  type SiswaKelasNilaiResponse,
} from '@/lib/nilai-api';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { SiswaNilaiBabTable } from '@/components/siswa/SiswaNilaiBabTable';
import { SiswaNilaiUjianList } from '@/components/siswa/SiswaNilaiUjianList';

function totalClass(n: number | null): string {
  if (n === null) return 'text-muted-foreground';
  if (n >= 75) return 'text-emerald-700 dark:text-emerald-400';
  if (n >= 60) return 'text-amber-700 dark:text-amber-400';
  return 'text-rose-700 dark:text-rose-400';
}

function SiswaKelasNilaiContent({ kelasID }: { kelasID: string }) {
  const enrollmentQuery = useQuery({
    queryKey: ['siswa', 'kelas', 'list'],
    queryFn: () => listMyKelas({ page: 1, pageSize: 50 }),
    staleTime: 30_000,
  });

  const nilaiQuery = useQuery({
    queryKey: ['siswa', 'kelas', 'nilai', kelasID],
    queryFn: () => getSiswaKelasNilai(kelasID),
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

  const enrollment: MyKelasItem | undefined = React.useMemo(() => {
    return enrollmentQuery.data?.items.find((it) => it.kelas.id === kelasID);
  }, [enrollmentQuery.data, kelasID]);

  if (nilaiQuery.isPending && !nilaiQuery.data) {
    return (
      <div className="space-y-4">
        <div className="h-6 w-32 animate-pulse rounded bg-muted" />
        <div className="h-32 animate-pulse rounded-md border bg-muted/40" />
        <div className="h-64 animate-pulse rounded-md border bg-muted/40" />
      </div>
    );
  }

  if (nilaiQuery.isError) {
    const err = nilaiQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    const isForbidden =
      apiErr?.status === 403 || apiErr?.status === 404 || apiErr?.code === 'forbidden';
    const requestId = apiErr?.requestId;

    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {isForbidden ? 'Akses ditolak' : 'Gagal memuat nilai'}
          </CardTitle>
          <CardDescription>
            {isForbidden
              ? 'Lu tidak terdaftar aktif di kelas ini, atau kelas tidak ada.'
              : (apiErr?.message ?? 'Terjadi kesalahan tidak terduga.')}
            {requestId ? ` (req: ${requestId})` : ''}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {!isForbidden && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => nilaiQuery.refetch()}
              disabled={nilaiQuery.isFetching}
            >
              <RotateCcw className="size-4" />
              Coba lagi
            </Button>
          )}
          <Button asChild variant="ghost" size="sm">
            <Link href="/siswa/nilai">
              <ArrowLeft className="size-4" />
              Nilai semua kelas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  const data: SiswaKelasNilaiResponse = nilaiQuery.data!;
  const kelasName = data.kelas.nama || enrollment?.kelas.nama || 'Kelas';

  return (
    <div className="space-y-6">
      <header className="space-y-2">
        <Button asChild variant="ghost" size="sm" className="-ml-3">
          <Link href={`/siswa/kelas/detail?id=${kelasID}`}>
            <ArrowLeft className="size-4" />
            Kembali ke kelas
          </Link>
        </Button>
        <div className="flex flex-wrap items-center gap-2">
          <TrendingUp className="size-5 text-muted-foreground" />
          <h1 className="text-2xl font-semibold tracking-tight">
            Nilai · {kelasName}
          </h1>
        </div>
        <p className="text-sm text-muted-foreground">
          Rekap nilai bab (ulangan + tugas) dan ulangan harian lu di kelas ini.
          Bobot diatur guru per tugas dan per ujian.
        </p>
      </header>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <div className="space-y-1">
            <CardTitle className="text-sm">Total nilai kelas</CardTitle>
            <CardDescription>
              Rata-rata dari nilai bab (bab tanpa nilai di-skip).
            </CardDescription>
          </div>
        </CardHeader>
        <CardContent>
          <div className={`text-3xl font-bold tabular-nums ${totalClass(data.total_kelas)}`}>
            {formatNilai(data.total_kelas)}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3">
          <div className="space-y-1.5">
            <CardTitle className="text-base">Nilai per bab</CardTitle>
            <CardDescription>
              Total bab = rata-rata tertimbang ulangan bab + tugas. Bab yang
              belum ada komponen nilai ditandai &quot;—&quot;.
            </CardDescription>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => nilaiQuery.refetch()}
            disabled={nilaiQuery.isFetching}
          >
            <RotateCcw className="size-4" />
            Refresh
          </Button>
        </CardHeader>
        <CardContent>
          <SiswaNilaiBabTable bab={data.bab} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Ulangan harian</CardTitle>
          <CardDescription>
            Ulangan harian lintas-bab. Tidak masuk total kelas — berdiri sendiri
            di rapor.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <SiswaNilaiUjianList rows={data.ulangan_harian} />
        </CardContent>
      </Card>
    </div>
  );
}

export default function SiswaKelasNilaiPage() {
  const searchParams = useSearchParams();
  const id = searchParams?.get('id') ?? '';

  if (!id) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">ID kelas tidak ada</CardTitle>
          <CardDescription>
            URL ini butuh parameter <code>?id=...</code>. Kembali ke daftar
            kelas atau dashboard nilai.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          <Button asChild variant="outline" size="sm">
            <Link href="/siswa/nilai">
              <ArrowLeft className="size-4" />
              Nilai semua kelas
            </Link>
          </Button>
          <Button asChild variant="ghost" size="sm">
            <Link href="/siswa">Daftar kelas</Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  return <SiswaKelasNilaiContent kelasID={id} />;
}
