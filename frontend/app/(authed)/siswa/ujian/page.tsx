'use client';

/**
 * /siswa/ujian — siswa lobby page lintas-kelas (Task 6.G.1).
 *
 * Pola mirror /siswa/tugas (Fase 4.D.2) — top-level page hydrate enrollment
 * via listMyKelas, lalu per-kelas parallel fetch listSiswaUjianByKelas +
 * listSiswaUjianHasil. UjianLobbyCard render per ujian dengan attempt
 * aggregate (filter per ujian_id).
 *
 * Filter:
 *   - Kelas: dropdown (default semua kelas).
 *   - Status window: aktif / mendatang / lewat / semua (computed FE).
 *
 * BE service-level role-branch siswa di GET /kelas/:id/ujian auto-filter
 * status='published' — siswa tidak perlu opt-in di FE. Draft/archived
 * tidak akan muncul di response.
 *
 * Cross-kelas berarti N+1 queries: listKelas + N kelas * (list ujian +
 * list hasil). Dengan limit 50 kelas (cap listMyKelas page size), worst
 * case 1 + 50*2 = 101 queries. Acceptable buat MVP — kalau jadi bottleneck
 * kemudian bisa add backend `/siswa/ujian` flat aggregate endpoint.
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
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { UjianLobbyCard } from '@/components/siswa-ujian/UjianLobbyCard';

// ---------- Filter state ----------

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

// ---------- Helpers (computed window state) ----------

type WindowState = 'mendatang' | 'aktif' | 'berakhir' | 'tanpa-window';

function computeWindowState(now: number, ujian: Ujian): WindowState {
  const startMs = ujian.waktu_mulai ? new Date(ujian.waktu_mulai).getTime() : null;
  const endMs = ujian.waktu_selesai ? new Date(ujian.waktu_selesai).getTime() : null;
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
  // Sort: aktif (closest deadline first) > mendatang (soonest start first) >
  // tanpa-window > berakhir (most recent first).
  const startMs = ujian.waktu_mulai ? new Date(ujian.waktu_mulai).getTime() : null;
  const endMs = ujian.waktu_selesai ? new Date(ujian.waktu_selesai).getTime() : null;
  if (startMs && now < startMs) {
    // mendatang — sort by start ascending (smaller positive offset)
    return 100_000_000 + (startMs - now);
  }
  if (endMs && now > endMs) {
    // berakhir — sort by recency (smaller now-endMs)
    return 1_000_000_000 + (now - endMs);
  }
  if (endMs) {
    return endMs - now; // aktif with deadline
  }
  return 50_000_000; // tanpa-window aktif (between aktif and mendatang)
}

// ---------- Component ----------

export default function SiswaUjianPage() {
  const [windowFilter, setWindowFilter] = React.useState<WindowFilter>('all');
  const [kelasFilter, setKelasFilter] = React.useState<string>('all');
  const [now, setNow] = React.useState<number>(() => Date.now());

  // Tick once a minute for window state filter (per-card sudah ada interval
  // tiap detik untuk countdown; di sini cuma re-derive group state).
  React.useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(id);
  }, []);

  // Step 1: enrollments.
  const enrollmentQuery = useQuery({
    queryKey: ['siswa', 'kelas', 'list'],
    queryFn: () => listMyKelas({ page: 1, pageSize: 50 }),
    staleTime: 30_000,
  });

  const kelasItems = React.useMemo<MyKelasItem[]>(
    () => enrollmentQuery.data?.items ?? [],
    [enrollmentQuery.data?.items],
  );

  // Step 2 (parallel): list ujian per kelas.
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

  // Step 3 (parallel): list siswa hasil aggregate per kelas.
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

  // Compose per-kelas data.
  const perKelas = React.useMemo<PerKelasData[]>(() => {
    return kelasItems.map((it, idx) => ({
      kelasID: it.kelas.id,
      kelasName: it.kelas.nama,
      ujian: ujianQueries[idx]?.data?.items ?? [],
      hasil: hasilQueries[idx]?.data?.hasil ?? null,
    }));
  }, [kelasItems, ujianQueries, hasilQueries]);

  // Flat list buat counts + filtered render.
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
        if (kelasFilter !== 'all' && r.kelas.kelasID !== kelasFilter) return false;
        const state = computeWindowState(now, r.ujian);
        if (!matchesWindowFilter(windowFilter, state)) return false;
        return true;
      })
      .sort((a, b) => ujianSortKey(a.ujian, now) - ujianSortKey(b.ujian, now));
  }, [flatRows, kelasFilter, windowFilter, now]);

  // Aggregate counters (untuk header tile).
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
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Ujian saya</h1>
        <p className="text-sm text-muted-foreground">
          Daftar ulangan dari semua kelas lu. Klik kartu untuk mulai atau
          melanjutkan attempt.
        </p>
      </header>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <SummaryTile
          icon={GraduationCap}
          label="Sedang aktif"
          value={String(counts.aktif)}
          accent="emerald"
        />
        <SummaryTile
          icon={ClipboardList}
          label="Mendatang"
          value={String(counts.mendatang)}
        />
        <SummaryTile
          icon={ClipboardList}
          label="Sudah berakhir"
          value={String(counts.berakhir)}
        />
        <SummaryTile
          icon={Trophy}
          label="Nilai terbaik"
          value={
            counts.nilaiTerbaik != null
              ? formatNilai(counts.nilaiTerbaik)
              : counts.attemptSelesai > 0
                ? '—'
                : 'Belum ada attempt'
          }
          subtle={`${counts.attemptSelesai} attempt selesai`}
        />
      </div>

      <div className="flex flex-wrap items-center gap-3 border-b pb-3">
        <div className="flex flex-wrap items-center gap-1">
          {WINDOW_TABS.map((t) => {
            const active = windowFilter === t.key;
            return (
              <button
                key={t.key}
                type="button"
                onClick={() => setWindowFilter(t.key)}
                className={cn(
                  'rounded-md px-3 py-1.5 text-sm transition-colors',
                  active
                    ? 'bg-accent text-accent-foreground font-medium'
                    : 'text-muted-foreground hover:bg-accent/50',
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
            className="text-xs uppercase tracking-wide text-muted-foreground"
          >
            Kelas
          </label>
          <select
            id="kelas-filter"
            value={kelasFilter}
            onChange={(e) => setKelasFilter(e.target.value)}
            className="rounded-md border bg-background px-2 py-1 text-sm"
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
        <div className="space-y-3">
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
        <p className="flex items-center gap-2 text-xs text-muted-foreground">
          <AlertCircle className="size-3.5 text-amber-500" />
          Sebagian kelas gagal di-fetch — list yang muncul mungkin tidak lengkap.
        </p>
      ) : null}
    </div>
  );
}

// ---------- Sub-components ----------

interface SummaryTileProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
  subtle?: string;
  accent?: 'default' | 'emerald';
}

function SummaryTile({
  icon: Icon,
  label,
  value,
  subtle,
  accent = 'default',
}: SummaryTileProps) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
        <div className="space-y-1">
          <CardTitle className="text-sm">{label}</CardTitle>
          {subtle ? <CardDescription>{subtle}</CardDescription> : null}
        </div>
        <Icon
          className={cn(
            'size-5',
            accent === 'emerald' ? 'text-emerald-500' : 'text-muted-foreground',
          )}
        />
      </CardHeader>
      <CardContent>
        <span className="text-2xl font-semibold tabular-nums">{value}</span>
      </CardContent>
    </Card>
  );
}

function LobbySkeleton() {
  return (
    <div className="space-y-3">
      {Array.from({ length: 3 }).map((_, i) => (
        <div
          key={i}
          className="h-48 animate-pulse rounded-md border bg-muted/40"
        />
      ))}
    </div>
  );
}

function EmptyKelasCard() {
  return (
    <Card>
      <CardContent className="p-8 text-center">
        <p className="text-sm text-muted-foreground">
          Lu belum gabung kelas mana pun. Gabung dulu pakai kode invite di
          /siswa/gabung supaya bisa lihat ujian.
        </p>
        <div className="mt-4">
          <Button asChild size="sm">
            <Link href="/siswa/gabung">
              Gabung kelas <ArrowRight className="size-4" />
            </Link>
          </Button>
        </div>
      </CardContent>
    </Card>
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
    <Card>
      <CardContent className="p-8 text-center">
        <p className="text-sm text-muted-foreground">{message}</p>
      </CardContent>
    </Card>
  );
}

function ErrorCard({ title, message }: { title: string; message: string }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{message}</CardDescription>
      </CardHeader>
      <CardContent>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => window.location.reload()}
        >
          <Loader2 className="size-4" />
          Muat ulang halaman
        </Button>
      </CardContent>
    </Card>
  );
}

function formatNilai(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(2);
}
