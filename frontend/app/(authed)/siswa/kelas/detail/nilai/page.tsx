'use client';

/**
 * /siswa/kelas/detail/nilai?id=:id — siswa rekap nilai per-kelas (Task 7.A.2).
 *
 * Visual: neo-brutalism + pastel pop. Header tone deterministic kelas,
 * total card berwarna nilai accent, table breakdown bab + ulangan harian.
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
import { SiswaNilaiBabTable } from '@/components/siswa/SiswaNilaiBabTable';
import { SiswaNilaiUjianList } from '@/components/siswa/SiswaNilaiUjianList';
import {
  SECTION_META,
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardDescription,
  SiswaCardHeader,
  SiswaCardTitle,
  kelasToneFromId,
} from '@/components/siswa-ui';

function totalClass(n: number | null): string {
  if (n === null) return 'text-siswa-text-muted';
  if (n >= 75) return 'text-emerald-700';
  if (n >= 60) return 'text-amber-700';
  return 'text-rose-700';
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
        <div className="h-6 w-32 animate-pulse rounded bg-siswa-text/10" />
        <div className="h-32 animate-pulse rounded-siswa siswa-border bg-siswa-surface" />
        <div className="h-64 animate-pulse rounded-siswa siswa-border bg-siswa-surface" />
      </div>
    );
  }

  if (nilaiQuery.isError) {
    const err = nilaiQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    const isForbidden =
      apiErr?.status === 403 || apiErr?.status === 404 || apiErr?.code === 'forbidden';

    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>
            {isForbidden ? 'Akses ditolak' : 'Gagal memuat nilai'}
          </SiswaCardTitle>
          <SiswaCardDescription>
            {isForbidden
              ? 'Kamu tidak terdaftar aktif di kelas ini, atau kelas tidak ada.'
              : (apiErr?.message ?? 'Terjadi kesalahan tidak terduga.')}
            {apiErr?.requestId ? ` (req: ${apiErr.requestId})` : ''}
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody className="flex flex-wrap gap-2">
          {!isForbidden ? (
            <SiswaButton
              type="button"
              tone="surface"
              size="sm"
              onClick={() => nilaiQuery.refetch()}
              disabled={nilaiQuery.isFetching}
            >
              <RotateCcw className="size-4" />
              Coba lagi
            </SiswaButton>
          ) : null}
          <SiswaButton asChild tone="ghost" size="sm">
            <Link href="/siswa/nilai">
              <ArrowLeft className="size-4" />
              Nilai semua kelas
            </Link>
          </SiswaButton>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  const data: SiswaKelasNilaiResponse = nilaiQuery.data!;
  const kelasName = data.kelas.nama || enrollment?.kelas.nama || 'Kelas';
  const tone = kelasToneFromId(kelasID);
  const meta = SECTION_META[tone];
  const KelasIcon = meta.Icon;

  return (
    <div className="space-y-6">
      <SiswaButton asChild tone="ghost" size="sm" className="-ml-2">
        <Link href={`/siswa/kelas/detail?id=${kelasID}`}>
          <ArrowLeft className="size-4" />
          Kembali ke kelas
        </Link>
      </SiswaButton>

      <SiswaCard tone={tone} shadow="lg" className="overflow-hidden">
        <div className="flex items-start gap-4 border-b-2 border-siswa-border bg-siswa-surface/70 px-6 py-5">
          <span className="grid size-12 shrink-0 place-items-center rounded-siswa siswa-border bg-siswa-surface siswa-shadow-sm">
            <KelasIcon className="size-6" strokeWidth={2.5} />
          </span>
          <div className="min-w-0 flex-1 space-y-1">
            <span className="text-xs font-semibold uppercase tracking-[0.18em] text-siswa-text-muted">
              Rekap Nilai
            </span>
            <h1 className="siswa-display text-2xl font-bold leading-tight sm:text-3xl">
              {kelasName}
            </h1>
            <p className="text-sm text-siswa-text-muted">
              Bobot diatur guru per tugas dan per ujian.
            </p>
          </div>
        </div>
        <div className="px-6 py-5">
          <p className="text-xs font-semibold uppercase tracking-wide text-siswa-text-muted">
            Total nilai kelas
          </p>
          <div className="flex items-baseline gap-3">
            <p
              className={`siswa-display text-5xl font-bold tabular-nums ${totalClass(data.total_kelas)}`}
            >
              {formatNilai(data.total_kelas)}
            </p>
            <span className="flex items-center gap-1 text-xs text-siswa-text-muted">
              <TrendingUp className="size-3" />
              Rata-rata bab terbobot
            </span>
          </div>
        </div>
      </SiswaCard>

      <SiswaCard tone="materi" shadow="md">
        <SiswaCardHeader>
          <div className="flex flex-row items-start justify-between gap-3">
            <div className="space-y-1">
              <SiswaCardTitle>Nilai per bab</SiswaCardTitle>
              <SiswaCardDescription>
                Total bab = rata-rata tertimbang ulangan bab + tugas. Bab
                tanpa nilai ditandai &quot;—&quot;.
              </SiswaCardDescription>
            </div>
            <SiswaButton
              type="button"
              tone="surface"
              size="sm"
              onClick={() => nilaiQuery.refetch()}
              disabled={nilaiQuery.isFetching}
            >
              <RotateCcw className="size-4" />
              Refresh
            </SiswaButton>
          </div>
        </SiswaCardHeader>
        <SiswaCardBody>
          <SiswaNilaiBabTable bab={data.bab} />
        </SiswaCardBody>
      </SiswaCard>

      <SiswaCard tone="ulangan" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>Ulangan harian</SiswaCardTitle>
          <SiswaCardDescription>
            Ulangan harian lintas-bab. Tidak masuk total kelas — berdiri
            sendiri di rapor.
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          <SiswaNilaiUjianList rows={data.ulangan_harian} />
        </SiswaCardBody>
      </SiswaCard>
    </div>
  );
}

export default function SiswaKelasNilaiPage() {
  const searchParams = useSearchParams();
  const id = searchParams?.get('id') ?? '';

  if (!id) {
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>ID kelas tidak ada</SiswaCardTitle>
          <SiswaCardDescription>
            URL ini butuh parameter <code>?id=...</code>. Kembali ke daftar
            kelas atau dashboard nilai.
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody className="flex flex-wrap gap-2">
          <SiswaButton asChild tone="surface" size="sm">
            <Link href="/siswa/nilai">
              <ArrowLeft className="size-4" />
              Nilai semua kelas
            </Link>
          </SiswaButton>
          <SiswaButton asChild tone="ghost" size="sm">
            <Link href="/siswa">Daftar kelas</Link>
          </SiswaButton>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  return <SiswaKelasNilaiContent kelasID={id} />;
}
