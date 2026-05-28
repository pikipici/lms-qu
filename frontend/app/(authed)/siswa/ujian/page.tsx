'use client';

/**
 * /siswa/ujian — siswa lobby page lintas-kelas (Task 6.G.1).
 *
 * Visual: neo-brutalism + pastel pop. Stat tiles 4-up dengan section accent
 * (aktif=ulangan, mendatang=cream, berakhir=surface, terbaik=latihan).
 * Filter pill + dropdown kelas. UjianLobbyCard list.
 */

import * as React from 'react';
import Link from 'next/link';
import { useQuery, useQueries } from '@tanstack/react-query';
import {
  AlertCircle,
  ArrowRight,
  ClipboardList,
  GraduationCap,
  Loader2,
  Trophy,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import { listMyKelas, type MyKelasItem } from '@/lib/siswa-api';
import {
  listSiswaUjianByKelas,
  listSiswaUjianHasil,
  type Ujian,
  type SiswaUjianHasilListResult,
} from '@/lib/siswa-ujian-api';
import { cn } from '@/lib/utils';
import { UjianLobbyCard } from '@/components/siswa-ujian/UjianLobbyCard';
import {
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardDescription,
  SiswaCardHeader,
  SiswaCardTitle,
  SiswaPageHeader,
  SiswaStat,
} from '@/components/siswa-ui';

type WindowFilter = 'all' | 'aktif' | 'mendatang' | 'berakhir';

const WINDOW_TABS: { key: WindowFilter; label: string }[] = [
  { key: 'all', label: 'Semua' },
  { key: 'aktif', label: 'Aktif' },
  { key: 'mendatang', label: 'Mendatang' },
  { key: 'berakhir', label: 'Berakhir' },
];

interface PerKelasData {
  kelasID: string;
  kelasName: string;
  ujian: Ujian[];
  hasil: SiswaUjianHasilListResult | null;
}

type WindowState = 'mendatang' | 'aktif' | 'berakhir' | 'tanpa-window';

function computeWindowState(now: number, ujian: Ujian): WindowState {
  const startMs = ujian.waktu_mulai
    ? new Date(ujian.waktu_mulai).getTime()
    : null;
  const endMs = ujian.waktu_selesai
    ? new Date(ujian.waktu_selesai).getTime()
    : null;
  if (startMs && now < startMs) return 'mendatang';
  if (endMs && now > endMs) return 'berakhir';
  if (startMs || endMs) return 'aktif';
  return 'tanpa-window';
}

function matchesWindowFilter(filter: WindowFilter, state: WindowState): boolean {
  if (filter === 'all') return true;
  if (filter === 'aktif') return state === 'aktif' || state === 'tanpa-window';
  return state === filter;
}

function ujianSortKey(ujian: Ujian, now: number): number {
  const startMs = ujian.waktu_mulai
    ? new Date(ujian.waktu_mulai).getTime()
    : null;
  const endMs = ujian.waktu_selesai
    ? new Date(ujian.waktu_selesai).getTime()
    : null;
  if (startMs && now < startMs) {
    return 100_000_000 + (startMs - now);
  }
  if (endMs && now > endMs) {
    return 1_000_000_000 + (now - endMs);
  }
  if (endMs) {
    return endMs - now;
  }
  return 50_000_000;
}

function formatNilai(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(2);
}

export default function SiswaUjianPage() {
  const [windowFilter, setWindowFilter] = React.useState<WindowFilter>('all');
  const [kelasFilter, setKelasFilter] = React.useState<string>('all');
  const [now, setNow] = React.useState<number>(() => Date.now());

  React.useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(id);
  }, []);

  const enrollmentQuery = useQuery({
    queryKey: ['siswa', 'kelas', 'list'],
    queryFn: () => listMyKelas({ page: 1, pageSize: 50 }),
    staleTime: 30_000,
  });

  const kelasItems = React.useMemo<MyKelasItem[]>(
    () => enrollmentQuery.data?.items ?? [],
    [enrollmentQuery.data?.items],
  );

  const ujianQueries = useQueries({
    queries: kelasItems.map((it) => ({
      queryKey: ['siswa', 'ujian', 'list', it.kelas.id],
      queryFn: () => listSiswaUjianByKelas(it.kelas.id, { limit: 100 }),
      staleTime: 15_000,
      retry: (failureCount: number, err: Error) => {
        if (err instanceof ApiError) {
          if (err.status === 403 || err.status === 404) return false;
        }
        return failureCount < 2;
      },
    })),
  });

  const hasilQueries = useQueries({
    queries: kelasItems.map((it) => ({
      queryKey: ['siswa', 'ujian', 'hasil', it.kelas.id],
      queryFn: () => listSiswaUjianHasil(it.kelas.id),
      staleTime: 15_000,
      retry: (failureCount: number, err: Error) => {
        if (err instanceof ApiError) {
          if (err.status === 403 || err.status === 404) return false;
        }
        return failureCount < 2;
      },
    })),
  });

  const perKelas = React.useMemo<PerKelasData[]>(() => {
    return kelasItems.map((it, idx) => ({
      kelasID: it.kelas.id,
      kelasName: it.kelas.nama,
      ujian: ujianQueries[idx]?.data?.items ?? [],
      hasil: hasilQueries[idx]?.data?.hasil ?? null,
    }));
  }, [kelasItems, ujianQueries, hasilQueries]);

  const flatRows = React.useMemo(() => {
    const rows: { kelas: PerKelasData; ujian: Ujian }[] = [];
    for (const k of perKelas) {
      for (const u of k.ujian) {
        rows.push({ kelas: k, ujian: u });
      }
    }
    return rows;
  }, [perKelas]);

  const filtered = React.useMemo(() => {
    return flatRows
      .filter((r) => {
        if (kelasFilter !== 'all' && r.kelas.kelasID !== kelasFilter)
          return false;
        const state = computeWindowState(now, r.ujian);
        if (!matchesWindowFilter(windowFilter, state)) return false;
        return true;
      })
      .sort((a, b) => ujianSortKey(a.ujian, now) - ujianSortKey(b.ujian, now));
  }, [flatRows, kelasFilter, windowFilter, now]);

  const counts = React.useMemo(() => {
    let aktif = 0;
    let mendatang = 0;
    let berakhir = 0;
    let attemptSelesai = 0;
    let nilaiTerbaik: number | null = null;
    for (const r of flatRows) {
      const state = computeWindowState(now, r.ujian);
      if (state === 'aktif' || state === 'tanpa-window') aktif++;
      else if (state === 'mendatang') mendatang++;
      else if (state === 'berakhir') berakhir++;
    }
    for (const k of perKelas) {
      if (!k.hasil) continue;
      attemptSelesai += k.hasil.attempt_count;
      if (k.hasil.nilai_terbaik != null) {
        if (nilaiTerbaik == null || k.hasil.nilai_terbaik > nilaiTerbaik) {
          nilaiTerbaik = k.hasil.nilai_terbaik;
        }
      }
    }
    return { aktif, mendatang, berakhir, attemptSelesai, nilaiTerbaik };
  }, [flatRows, perKelas, now]);

  const isLoading =
    enrollmentQuery.isPending ||
    ujianQueries.some((q) => q.isPending) ||
    hasilQueries.some((q) => q.isPending);
  const hasError =
    enrollmentQuery.isError ||
    ujianQueries.some((q) => q.isError) ||
    hasilQueries.some((q) => q.isError);

  return (
    <div className="space-y-6">
      <SiswaPageHeader
        eyebrow="Ujian saya"
        title="Lobby ujian"
        description="Daftar ulangan dari semua kelas. Klik kartu untuk mulai atau melanjutkan ujian."
      />

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <SiswaStat
          label="Sedang aktif"
          value={counts.aktif}
          Icon={GraduationCap}
          tone="ulangan"
        />
        <SiswaStat
          label="Mendatang"
          value={counts.mendatang}
          Icon={ClipboardList}
          tone="materi"
        />
        <SiswaStat
          label="Sudah berakhir"
          value={counts.berakhir}
          Icon={ClipboardList}
          tone="surface"
        />
        <SiswaStat
          label="Nilai terbaik"
          value={
            counts.nilaiTerbaik != null
              ? formatNilai(counts.nilaiTerbaik)
              : counts.attemptSelesai > 0
                ? '—'
                : 'Belum ada'
          }
          hint={`${counts.attemptSelesai} kesempatan selesai`}
          Icon={Trophy}
          tone="latihan"
        />
      </div>

      <div className="flex flex-wrap items-center gap-3 rounded-siswa siswa-border bg-siswa-surface p-3 siswa-shadow-sm">
        <div className="flex flex-wrap items-center gap-1">
          {WINDOW_TABS.map((t) => {
            const active = windowFilter === t.key;
            return (
              <button
                key={t.key}
                type="button"
                onClick={() => setWindowFilter(t.key)}
                className={cn(
                  'rounded-[calc(var(--siswa-radius)-4px)] px-3 py-1.5 text-sm font-semibold transition-colors',
                  active
                    ? 'border-2 border-siswa-border bg-siswa-yellow siswa-shadow-sm'
                    : 'border-2 border-transparent text-siswa-text/70 hover:bg-siswa-cream/60',
                )}
              >
                {t.label}
              </button>
            );
          })}
        </div>
        <div className="ml-auto flex items-center gap-2">
          <label
            htmlFor="kelas-filter"
            className="text-xs font-semibold uppercase tracking-wide text-siswa-text-muted"
          >
            Kelas
          </label>
          <select
            id="kelas-filter"
            value={kelasFilter}
            onChange={(e) => setKelasFilter(e.target.value)}
            className="rounded-siswa border-2 border-siswa-border bg-siswa-surface px-3 py-1.5 text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-siswa-yellow"
          >
            <option value="all">Semua kelas</option>
            {kelasItems.map((it) => (
              <option key={it.kelas.id} value={it.kelas.id}>
                {it.kelas.nama}
              </option>
            ))}
          </select>
        </div>
      </div>

      {enrollmentQuery.isPending ? (
        <LobbySkeleton />
      ) : enrollmentQuery.isError ? (
        <ErrorCard
          title="Gagal memuat kelas"
          message={
            enrollmentQuery.error instanceof ApiError
              ? enrollmentQuery.error.message
              : 'Terjadi kesalahan tidak terduga.'
          }
        />
      ) : kelasItems.length === 0 ? (
        <EmptyKelasCard />
      ) : isLoading && filtered.length === 0 ? (
        <LobbySkeleton />
      ) : filtered.length === 0 ? (
        <EmptyUjianCard
          windowFilter={windowFilter}
          kelasFilter={kelasFilter}
          totalRows={flatRows.length}
        />
      ) : (
        <div className="space-y-4">
          {filtered.map(({ kelas, ujian }) => (
            <UjianLobbyCard
              key={ujian.id}
              ujian={ujian}
              kelasName={kelas.kelasName}
              hasilAggregate={kelas.hasil ?? undefined}
            />
          ))}
        </div>
      )}

      {hasError && filtered.length > 0 ? (
        <p className="flex items-center gap-2 text-xs text-siswa-text-muted">
          <AlertCircle className="size-3.5 text-siswa-warning" />
          Sebagian kelas gagal di-fetch — list yang muncul mungkin tidak lengkap.
        </p>
      ) : null}
    </div>
  );
}

function LobbySkeleton() {
  return (
    <div className="space-y-3">
      {Array.from({ length: 3 }).map((_, i) => (
        <div
          key={i}
          className="h-48 animate-pulse rounded-siswa siswa-border bg-siswa-surface/60"
        />
      ))}
    </div>
  );
}

function EmptyKelasCard() {
  return (
    <SiswaCard tone="surface" shadow="md">
      <SiswaCardBody className="p-8 text-center">
        <p className="text-sm text-siswa-text-muted">
          Lu belum gabung kelas mana pun. Gabung dulu pakai kode invite di
          /siswa/gabung supaya bisa lihat ujian.
        </p>
        <div className="mt-4">
          <SiswaButton asChild tone="primary" size="sm">
            <Link href="/siswa/gabung">
              Gabung kelas <ArrowRight className="size-4" strokeWidth={2.5} />
            </Link>
          </SiswaButton>
        </div>
      </SiswaCardBody>
    </SiswaCard>
  );
}

function EmptyUjianCard({
  windowFilter,
  kelasFilter,
  totalRows,
}: {
  windowFilter: WindowFilter;
  kelasFilter: string;
  totalRows: number;
}) {
  const message = (() => {
    if (totalRows === 0) {
      return 'Belum ada ujian dipublish di kelas-kelas lu. Tunggu guru aktivasi ujian.';
    }
    if (windowFilter !== 'all' || kelasFilter !== 'all') {
      return 'Tidak ada ujian yang masuk filter. Coba ubah filter status atau pilih kelas lain.';
    }
    return 'Belum ada ujian.';
  })();
  return (
    <SiswaCard tone="surface" shadow="md">
      <SiswaCardBody className="p-8 text-center">
        <p className="text-sm text-siswa-text-muted">{message}</p>
      </SiswaCardBody>
    </SiswaCard>
  );
}

function ErrorCard({ title, message }: { title: string; message: string }) {
  return (
    <SiswaCard tone="surface" shadow="md">
      <SiswaCardHeader>
        <SiswaCardTitle>{title}</SiswaCardTitle>
        <SiswaCardDescription>{message}</SiswaCardDescription>
      </SiswaCardHeader>
      <SiswaCardBody>
        <SiswaButton
          type="button"
          tone="surface"
          size="sm"
          onClick={() => window.location.reload()}
        >
          <Loader2 className="size-4" />
          Muat ulang halaman
        </SiswaButton>
      </SiswaCardBody>
    </SiswaCard>
  );
}
