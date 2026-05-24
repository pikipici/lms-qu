'use client';

/**
 * /siswa/kelas/detail?id=:id — siswa kelas detail page (Task 3.E.2).
 *
 * Static export: pakai query string. Mirror /guru/kelas/detail pattern.
 *
 * Layout:
 *   - Header card berwarna deterministik per kelas_id (kelasToneFromId).
 *   - Section "Bab kelas" (materi accent, biru).
 *   - Section "Tugas kelas" (tugas accent, pink).
 *   - Section "Pengumuman" (umum accent, krem).
 */

import * as React from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useQuery, type UseQueryResult } from '@tanstack/react-query';
import {
  ArrowLeft,
  ArrowRight,
  BookOpen,
  Calendar,
  ClipboardList,
  Megaphone,
  RotateCcw,
  Sparkles,
  TrendingUp,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import { listMyKelas, type MyKelasItem } from '@/lib/siswa-api';
import {
  listSiswaBab,
  type SiswaBabItem,
  type SiswaBabListResponse,
} from '@/lib/siswa-bab-api';
import { Button } from '@/components/ui/button';
import { SiswaBabProgressBar } from '@/components/siswa/SiswaBabProgressBar';
import { PengumumanReadList } from '@/components/pengumuman/PengumumanReadList';
import { SiswaTugasList } from '@/components/submission/SiswaTugasList';
import {
  SECTION_META,
  SiswaBadge,
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardDescription,
  SiswaCardHeader,
  SiswaCardTitle,
  kelasToneFromId,
} from '@/components/siswa-ui';

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

function joinedViaLabel(via: 'kode' | 'admin'): string {
  return via === 'kode' ? 'kode invite' : 'admin';
}

function BabCard({ kelasID, bab }: { kelasID: string; bab: SiswaBabItem }) {
  const href = `/siswa/kelas/detail/bab?id=${kelasID}&bid=${bab.id}`;
  return (
    <Link
      href={href}
      className="block rounded-siswa siswa-border bg-siswa-surface siswa-press p-4 focus-visible:outline-none"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex items-center gap-2">
            <SiswaBadge tone="blue">Bab {bab.nomor}</SiswaBadge>
            <h3 className="siswa-display truncate text-base font-bold leading-tight">
              {bab.judul}
            </h3>
          </div>
          {bab.deskripsi ? (
            <p className="line-clamp-2 text-sm text-siswa-text-muted">
              {bab.deskripsi}
            </p>
          ) : null}
          <SiswaBabProgressBar
            persen={bab.progress.persen}
            materiRead={bab.progress.materi_read}
            materiTotal={bab.progress.materi_total}
            babKosong={bab.progress.bab_kosong}
            size="sm"
            variant="siswa"
          />
        </div>
        <ArrowRight
          className="mt-1 size-4 shrink-0 text-siswa-text-muted"
          strokeWidth={2.5}
        />
      </div>
    </Link>
  );
}

function SiswaKelasDetailContent({ kelasID }: { kelasID: string }) {
  const enrollmentQuery = useQuery({
    queryKey: ['siswa', 'kelas', 'list'],
    queryFn: () => listMyKelas({ page: 1, pageSize: 50 }),
    staleTime: 30_000,
  });

  const babQuery = useQuery({
    queryKey: ['siswa', 'kelas', 'bab', kelasID],
    queryFn: () => listSiswaBab(kelasID),
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

  if (enrollmentQuery.isPending) {
    return (
      <div className="space-y-4">
        <div className="h-6 w-32 animate-pulse rounded bg-siswa-text/10" />
        <div className="h-32 animate-pulse rounded-siswa siswa-border bg-siswa-surface" />
        <div className="h-64 animate-pulse rounded-siswa siswa-border bg-siswa-surface" />
      </div>
    );
  }

  if (enrollmentQuery.isError) {
    const err = enrollmentQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>Gagal memuat kelas</SiswaCardTitle>
          <SiswaCardDescription>
            {apiErr?.message ?? 'Terjadi kesalahan tidak terduga.'}
            {apiErr?.requestId ? ` (req: ${apiErr.requestId})` : ''}
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody className="flex flex-wrap gap-3">
          <SiswaButton
            type="button"
            tone="primary"
            size="sm"
            onClick={() => enrollmentQuery.refetch()}
            disabled={enrollmentQuery.isFetching}
          >
            <RotateCcw className="size-4" />
            Coba lagi
          </SiswaButton>
          <SiswaButton asChild tone="surface" size="sm">
            <Link href="/siswa">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </SiswaButton>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  if (!enrollment) {
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>Kelas tidak ditemukan</SiswaCardTitle>
          <SiswaCardDescription>
            Lu belum gabung kelas ini, atau ID kelas tidak valid. Gabung
            kelas baru pakai kode invite di /siswa/gabung.
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          <SiswaButton asChild tone="surface" size="sm">
            <Link href="/siswa">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </SiswaButton>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  const kelas = enrollment.kelas;
  const archived = Boolean(kelas.archived_at);
  const tone = kelasToneFromId(kelas.id);
  const meta = SECTION_META[tone];
  const KelasIcon = meta.Icon;

  return (
    <div className="space-y-6">
      <Button asChild variant="ghost" size="sm" className="-ml-2 text-siswa-text">
        <Link href="/siswa">
          <ArrowLeft className="size-4" />
          Daftar kelas
        </Link>
      </Button>

      {/* Header card berwarna deterministik */}
      <SiswaCard tone={tone} shadow="lg" className="overflow-hidden">
        <div className="flex items-start gap-4 border-b-2 border-siswa-border bg-siswa-surface/70 px-6 py-5">
          <span className="grid size-12 shrink-0 place-items-center rounded-siswa siswa-border bg-siswa-surface siswa-shadow-sm">
            <KelasIcon className="size-6" strokeWidth={2.5} />
          </span>
          <div className="min-w-0 flex-1 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs font-semibold uppercase tracking-[0.18em] text-siswa-text-muted">
                Kelas
              </span>
              {archived ? (
                <SiswaBadge tone="cream">Diarsipkan</SiswaBadge>
              ) : null}
              <SiswaBadge tone="neutral">
                via {joinedViaLabel(enrollment.joined_via)}
              </SiswaBadge>
            </div>
            <h1 className="siswa-display text-2xl font-bold leading-tight sm:text-3xl">
              {kelas.nama}
            </h1>
            <p className="flex items-center gap-1.5 text-xs text-siswa-text-muted">
              <Calendar className="size-3" />
              Bergabung {formatDate(enrollment.joined_at)}
            </p>
          </div>
        </div>
        <div className="space-y-3 px-6 py-5">
          {kelas.deskripsi ? (
            <p className="text-sm text-siswa-text">{kelas.deskripsi}</p>
          ) : (
            <p className="text-sm italic text-siswa-text-muted">
              Belum ada deskripsi dari guru.
            </p>
          )}
          <div className="flex flex-wrap gap-2 pt-1">
            <SiswaButton asChild tone="surface" size="sm">
              <Link href={`/siswa/kelas/detail/nilai?id=${kelasID}`}>
                <TrendingUp className="size-4" strokeWidth={2.5} />
                Lihat nilai kelas ini
              </Link>
            </SiswaButton>
          </div>
        </div>
      </SiswaCard>

      {/* Bab kelas (Materi accent — biru) */}
      <SiswaCard tone="materi" shadow="md">
        <SiswaCardHeader>
          <div className="flex flex-row items-start justify-between gap-3">
            <div className="space-y-1">
              <SiswaCardTitle className="flex items-center gap-2">
                <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                  <BookOpen className="size-4" strokeWidth={2.5} />
                </span>
                Bab kelas
              </SiswaCardTitle>
              <SiswaCardDescription>
                Klik bab buat lihat materi, latihan, ulangan, dan tugas.
                Progress bar nge-track materi yang udah lu baca.
              </SiswaCardDescription>
            </div>
            <SiswaButton
              type="button"
              tone="surface"
              size="sm"
              onClick={() => babQuery.refetch()}
              disabled={babQuery.isFetching}
            >
              <RotateCcw className="size-4" />
              Refresh
            </SiswaButton>
          </div>
        </SiswaCardHeader>
        <SiswaCardBody>
          <BabListBody kelasID={kelasID} query={babQuery} />
        </SiswaCardBody>
      </SiswaCard>

      {/* Tugas kelas (Tugas accent — pink) */}
      <SiswaCard tone="tugas" shadow="md">
        <SiswaCardHeader>
          <div className="flex flex-row items-start justify-between gap-3">
            <div className="space-y-1">
              <SiswaCardTitle className="flex items-center gap-2">
                <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                  <ClipboardList className="size-4" strokeWidth={2.5} />
                </span>
                Tugas kelas
              </SiswaCardTitle>
              <SiswaCardDescription>
                Tugas kelas-wide dari guru. Tugas spesifik bab tersedia di halaman
                bab masing-masing.
              </SiswaCardDescription>
            </div>
            <Sparkles className="size-5 text-siswa-text/40" />
          </div>
        </SiswaCardHeader>
        <SiswaCardBody>
          <SiswaTugasList
            kelasID={kelasID}
            babID={null}
            emptyState="Belum ada tugas kelas-wide."
          />
        </SiswaCardBody>
      </SiswaCard>

      {/* Pengumuman (Umum accent — krem) */}
      <SiswaCard tone="umum" shadow="md">
        <SiswaCardHeader>
          <div className="flex flex-row items-start justify-between gap-3">
            <div className="space-y-1">
              <SiswaCardTitle className="flex items-center gap-2">
                <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                  <Megaphone className="size-4" strokeWidth={2.5} />
                </span>
                Pengumuman kelas
              </SiswaCardTitle>
              <SiswaCardDescription>
                Update terbaru dari guru. Pengumuman bab tersedia di halaman bab
                masing-masing.
              </SiswaCardDescription>
            </div>
          </div>
        </SiswaCardHeader>
        <SiswaCardBody>
          <PengumumanReadList
            kelasID={kelasID}
            babID={null}
            emptyState="Belum ada pengumuman dari guru."
            expandFirst
          />
        </SiswaCardBody>
      </SiswaCard>
    </div>
  );
}

function BabListBody({
  kelasID,
  query,
}: {
  kelasID: string;
  query: UseQueryResult<SiswaBabListResponse, Error>;
}) {
  if (query.isPending) {
    return (
      <div className="space-y-2">
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            className="h-24 animate-pulse rounded-siswa siswa-border bg-siswa-surface/50"
          />
        ))}
      </div>
    );
  }

  if (query.isError) {
    const err = query.error;
    const apiErr = err instanceof ApiError ? err : null;
    const isForbidden = apiErr?.code === 'forbidden';
    return (
      <div className="space-y-3 rounded-siswa border-2 border-siswa-danger bg-siswa-surface p-4 text-sm">
        <p className="font-bold">
          {isForbidden ? 'Akses ditolak' : 'Gagal memuat bab'}
        </p>
        <p className="text-siswa-text-muted">
          {isForbidden
            ? 'Lu tidak terdaftar aktif di kelas ini. Hubungi guru atau admin.'
            : apiErr?.message ?? 'Terjadi kesalahan tidak terduga.'}
          {apiErr?.requestId ? ` (req: ${apiErr.requestId})` : ''}
        </p>
        <SiswaButton
          type="button"
          tone="surface"
          size="sm"
          onClick={() => query.refetch()}
          disabled={query.isFetching}
        >
          <RotateCcw className="size-4" />
          Coba lagi
        </SiswaButton>
      </div>
    );
  }

  const items = query.data?.items ?? [];

  if (items.length === 0) {
    return (
      <div className="rounded-siswa border-2 border-dashed border-siswa-border-soft bg-siswa-surface/60 p-8 text-center">
        <BookOpen className="mx-auto mb-2 size-8 text-siswa-text-muted" strokeWidth={2.5} />
        <p className="text-sm text-siswa-text-muted">
          Belum ada bab yang dipublish di kelas ini. Tunggu guru lu nge-publish bab.
        </p>
      </div>
    );
  }

  return (
    <ul className="space-y-3">
      {items.map((bab) => (
        <li key={bab.id}>
          <BabCard kelasID={kelasID} bab={bab} />
        </li>
      ))}
    </ul>
  );
}

export default function SiswaKelasDetailPage() {
  const searchParams = useSearchParams();
  const id = searchParams?.get('id') ?? '';

  if (!id) {
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>ID kelas tidak ada</SiswaCardTitle>
          <SiswaCardDescription>
            URL ini butuh parameter <code>?id=...</code>. Kembali ke daftar kelas
            untuk pilih satu.
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          <SiswaButton asChild tone="surface" size="sm">
            <Link href="/siswa">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </SiswaButton>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  return <SiswaKelasDetailContent kelasID={id} />;
}
