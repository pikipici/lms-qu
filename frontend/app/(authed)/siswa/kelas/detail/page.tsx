'use client';

/**
 * /siswa/kelas/detail?id=:id — siswa kelas detail page (Task 3.E.2).
 *
 * Static export (Next 14 `output: 'export'`) tidak izinkan dynamic route
 * tanpa generateStaticParams. Mirror pola query-string seperti
 * /guru/kelas/detail (Task 2.B.4) + /admin/pengguna/detail.
 *
 * Render:
 *   - Header: nama kelas + back link ke /siswa + meta (gabung-via, joined_at).
 *   - List bab status='published' (filter di backend) dengan progress bar
 *     per bab. Empty state ramah kalau kelas belum ada bab published.
 *
 * Backend dependency:
 *   - GET /siswa/kelas (list enrollment, sudah dipakai di /siswa landing)
 *     dipakai untuk hydrate kelas info kalau user navigasi langsung dari
 *     bookmark — gak ada GET /siswa/kelas/:id detail dedicated, jadi kita
 *     listMyKelas + cari id di items. Kalau gak ketemu, anggap forbidden.
 *   - GET /siswa/kelas/:id/bab (Task 3.E.1) untuk list bab + progress.
 *
 * Klik card bab → push ke /siswa/kelas/detail/bab?id=:kelasID&bid=:babID.
 */

import * as React from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useQuery, type UseQueryResult } from '@tanstack/react-query';
import { ArrowLeft, ArrowRight, BookOpen, RotateCcw } from 'lucide-react';

import { ApiError } from '@/lib/api';
import { listMyKelas, type MyKelasItem } from '@/lib/siswa-api';
import {
  listSiswaBab,
  type SiswaBabItem,
  type SiswaBabListResponse,
} from '@/lib/siswa-bab-api';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { SiswaBabProgressBar } from '@/components/siswa/SiswaBabProgressBar';
import { PengumumanReadList } from '@/components/pengumuman/PengumumanReadList';
import { SiswaTugasList } from '@/components/submission/SiswaTugasList';

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

// ---------- Bab card ----------

function BabCard({
  kelasID,
  bab,
}: {
  kelasID: string;
  bab: SiswaBabItem;
}) {
  return (
    <Link
      href={`/siswa/kelas/detail/bab?id=${kelasID}&bid=${bab.id}`}
      className="group block rounded-lg border bg-card p-4 transition-colors hover:bg-accent/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex items-center gap-2">
            <span className="rounded-md bg-muted px-2 py-0.5 text-xs font-medium tabular-nums text-muted-foreground">
              Bab {bab.nomor}
            </span>
            <h3 className="truncate text-base font-semibold">{bab.judul}</h3>
          </div>
          {bab.deskripsi && (
            <p className="line-clamp-2 text-sm text-muted-foreground">
              {bab.deskripsi}
            </p>
          )}
          <SiswaBabProgressBar
            persen={bab.progress.persen}
            materiRead={bab.progress.materi_read}
            materiTotal={bab.progress.materi_total}
            babKosong={bab.progress.bab_kosong}
            size="sm"
          />
        </div>
        <ArrowRight className="mt-1 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
      </div>
    </Link>
  );
}

// ---------- Page content ----------

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
      // 403/404 → don't retry; user mungkin gak enroll atau kelas bukan miliknya.
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

  // Loading state (header). Bab loading shown separately below.
  if (enrollmentQuery.isPending) {
    return (
      <div className="space-y-4">
        <div className="h-6 w-32 animate-pulse rounded bg-muted" />
        <div className="h-32 animate-pulse rounded-md border bg-muted/40" />
        <div className="h-64 animate-pulse rounded-md border bg-muted/40" />
      </div>
    );
  }

  // Enrollment list error → render error card.
  if (enrollmentQuery.isError) {
    const err = enrollmentQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    const requestId = apiErr?.requestId;
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Gagal memuat kelas</CardTitle>
          <CardDescription>
            {apiErr?.message ?? 'Terjadi kesalahan tidak terduga.'}
            {requestId ? ` (req: ${requestId})` : ''}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => enrollmentQuery.refetch()}
            disabled={enrollmentQuery.isFetching}
          >
            <RotateCcw className="size-4" />
            Coba lagi
          </Button>
          <Button asChild variant="ghost" size="sm">
            <Link href="/siswa">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  // User tidak enroll di kelas ini.
  if (!enrollment) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Kelas tidak ditemukan</CardTitle>
          <CardDescription>
            Lu belum gabung kelas ini, atau ID kelas tidak valid. Gabung
            kelas baru pakai kode invite di /siswa/gabung.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline" size="sm">
            <Link href="/siswa">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  const kelas = enrollment.kelas;
  const archived = Boolean(kelas.archived_at);

  return (
    <div className="space-y-6">
      <header className="space-y-2">
        <Button asChild variant="ghost" size="sm" className="-ml-3">
          <Link href="/siswa">
            <ArrowLeft className="size-4" />
            Daftar kelas
          </Link>
        </Button>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">
            {kelas.nama}
          </h1>
          {archived && (
            <span className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
              Diarsipkan
            </span>
          )}
        </div>
        <p className="text-sm text-muted-foreground">
          Gabung {formatDate(enrollment.joined_at)} via{' '}
          <span className="font-medium">{joinedViaLabel(enrollment.joined_via)}</span>
        </p>
        {kelas.deskripsi && (
          <p className="max-w-3xl text-sm text-muted-foreground">
            {kelas.deskripsi}
          </p>
        )}
      </header>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3">
          <div className="space-y-1.5">
            <CardTitle className="text-base">Bab kelas</CardTitle>
            <CardDescription>
              Klik bab buat lihat materi. Progress bar nge-track materi yang
              udah lu baca.
            </CardDescription>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => babQuery.refetch()}
            disabled={babQuery.isFetching}
          >
            <RotateCcw className="size-4" />
            Refresh
          </Button>
        </CardHeader>
        <CardContent>
          <BabListBody kelasID={kelasID} query={babQuery} />
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3">
          <div className="space-y-1.5">
            <CardTitle className="text-base">Tugas kelas</CardTitle>
            <CardDescription>
              Tugas dari guru. Tugas bab tersedia di halaman bab masing-masing.
            </CardDescription>
          </div>
        </CardHeader>
        <CardContent>
          <SiswaTugasList
            kelasID={kelasID}
            babID={null}
            emptyState="Belum ada tugas kelas-wide."
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3">
          <div className="space-y-1.5">
            <CardTitle className="text-base">Pengumuman kelas</CardTitle>
            <CardDescription>
              Update terbaru dari guru. Pengumuman bab tersedia di halaman bab masing-masing.
            </CardDescription>
          </div>
        </CardHeader>
        <CardContent>
          <PengumumanReadList
            kelasID={kelasID}
            babID={null}
            emptyState="Belum ada pengumuman dari guru."
            expandFirst
          />
        </CardContent>
      </Card>
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
            className="h-24 animate-pulse rounded-md border bg-muted/40"
          />
        ))}
      </div>
    );
  }

  if (query.isError) {
    const err = query.error;
    const apiErr = err instanceof ApiError ? err : null;
    const isForbidden = apiErr?.code === 'forbidden';
    const requestId = apiErr?.requestId;
    return (
      <div className="space-y-2 rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
        <p className="font-medium">
          {isForbidden ? 'Akses ditolak' : 'Gagal memuat bab'}
        </p>
        <p>
          {isForbidden
            ? 'Lu tidak terdaftar aktif di kelas ini. Hubungi guru atau admin.'
            : apiErr?.message ?? 'Terjadi kesalahan tidak terduga.'}
          {requestId ? ` (req: ${requestId})` : ''}
        </p>
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
      </div>
    );
  }

  const items = query.data?.items ?? [];

  if (items.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-8 text-center">
        <BookOpen className="mx-auto mb-2 size-6 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          Belum ada bab yang dipublish di kelas ini. Tunggu guru lu nge-publish bab.
        </p>
      </div>
    );
  }

  return (
    <ul className="space-y-2">
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
      <Card>
        <CardHeader>
          <CardTitle className="text-base">ID kelas tidak ada</CardTitle>
          <CardDescription>
            URL ini butuh parameter <code>?id=...</code>. Kembali ke daftar kelas
            untuk pilih satu.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline" size="sm">
            <Link href="/siswa">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  return <SiswaKelasDetailContent kelasID={id} />;
}
