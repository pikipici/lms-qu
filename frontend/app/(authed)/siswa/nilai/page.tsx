'use client';

/**
 * /siswa/nilai — cross-class rekap nilai dashboard (Task 7.A.2).
 *
 * Sumber: GET /siswa/nilai → SiswaListResponse.items[] = one card per
 * active enrollment with total_kelas + bab_count + ulangan_count.
 *
 * UI:
 *   - Header + summary (total kelas yang punya nilai)
 *   - Grid card: kelas nama, guru, total kelas (warna), bab/ujian count,
 *     CTA "Lihat detail" → /siswa/kelas/detail/nilai?id=:id
 *   - Empty state: belum gabung kelas / belum ada nilai sama sekali
 */

import * as React from 'react';
import Link from 'next/link';
import { useQueries, useQuery } from '@tanstack/react-query';
import { ArrowRight, RotateCcw, TrendingUp, User2 } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  formatNilai,
  getSiswaKelasNilai,
  listSiswaNilai,
  type SiswaKelasNilaiResponse,
  type SiswaKelasSummary,
} from '@/lib/nilai-api';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

function totalClass(n: number | null): string {
  if (n === null) return 'text-muted-foreground';
  if (n >= 75) return 'text-emerald-700 dark:text-emerald-400';
  if (n >= 60) return 'text-amber-700 dark:text-amber-400';
  return 'text-rose-700 dark:text-rose-400';
}

function KelasNilaiCard({
  item,
  detail,
  detailLoading,
}: {
  item: SiswaKelasSummary;
  detail?: SiswaKelasNilaiResponse;
  detailLoading?: boolean;
}) {
  const babRows = detail?.bab ?? [];

  return (
    <Card className="flex h-full flex-col">
      <CardHeader className="space-y-1.5 pb-3">
        <CardTitle className="text-base">{item.kelas_nama}</CardTitle>
        <CardDescription className="flex items-center gap-1 text-xs">
          <User2 className="size-3" />
          {item.guru_nama || 'guru belum diatur'}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-3">
        <div>
          <p className="text-xs text-muted-foreground">Total nilai kelas</p>
          <p
            className={`text-3xl font-bold tabular-nums ${totalClass(item.total_kelas)}`}
          >
            {formatNilai(item.total_kelas)}
          </p>
          {item.total_kelas === null && (
            <p className="text-xs text-muted-foreground">
              Belum ada bab dengan nilai.
            </p>
          )}
        </div>
        <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
          <span className="rounded bg-muted px-2 py-0.5 tabular-nums">
            {item.bab_count} bab
          </span>
          <span className="rounded bg-muted px-2 py-0.5 tabular-nums">
            {item.ulangan_count} ulangan harian
          </span>
        </div>
        <div className="rounded-md border bg-muted/20">
          <div className="border-b px-3 py-2 text-xs font-medium text-muted-foreground">
            Breakdown nilai per bab
          </div>
          {detailLoading ? (
            <div className="space-y-2 p-3">
              <div className="h-4 animate-pulse rounded bg-muted" />
              <div className="h-4 animate-pulse rounded bg-muted" />
            </div>
          ) : babRows.length === 0 ? (
            <p className="p-3 text-xs text-muted-foreground">
              Belum ada bab yang punya data nilai.
            </p>
          ) : (
            <div className="divide-y">
              {babRows.map((bab) => (
                <div key={bab.bab_id} className="grid grid-cols-[1fr_auto] gap-3 px-3 py-2 text-xs">
                  <div className="min-w-0">
                    <p className="truncate font-medium text-foreground">
                      Bab {bab.nomor}. {bab.judul}
                    </p>
                    <p className="text-muted-foreground">
                      Tugas {formatNilai(bab.nilai_tugas_bab)} · Ulangan {formatNilai(bab.nilai_ulangan_bab)}
                    </p>
                  </div>
                  <div className={`self-center text-right font-semibold tabular-nums ${totalClass(bab.total)}`}>
                    {formatNilai(bab.total)}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
        <div className="mt-auto pt-2">
          <Button asChild variant="outline" size="sm" className="w-full">
            <Link href={`/siswa/kelas/detail/nilai?id=${item.kelas_id}`}>
              Lihat detail
              <ArrowRight className="size-4" />
            </Link>
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export default function SiswaNilaiPage() {
  const nilaiQ = useQuery({
    queryKey: ['siswa', 'nilai', 'list'],
    queryFn: () => listSiswaNilai(),
    staleTime: 15_000,
  });

  const items = React.useMemo(() => nilaiQ.data?.items ?? [], [nilaiQ.data?.items]);

  const detailQueries = useQueries({
    queries: items.map((item) => ({
      queryKey: ['siswa', 'kelas', 'nilai', item.kelas_id],
      queryFn: () => getSiswaKelasNilai(item.kelas_id),
      staleTime: 15_000,
      enabled: items.length > 0,
    })),
  });

  const detailByKelas = React.useMemo(() => {
    const map = new Map<string, SiswaKelasNilaiResponse>();
    detailQueries.forEach((query, index) => {
      const kelasID = items[index]?.kelas_id;
      if (kelasID && query.data) map.set(kelasID, query.data);
    });
    return map;
  }, [detailQueries, items]);

  const counts = React.useMemo(() => {
    let withNilai = 0;
    let avgSum = 0;
    let avgN = 0;
    for (const it of items) {
      if (it.total_kelas !== null) {
        withNilai++;
        avgSum += it.total_kelas;
        avgN++;
      }
    }
    const avgKelas = avgN > 0 ? avgSum / avgN : null;
    return { withNilai, total: items.length, avgKelas };
  }, [items]);

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">Nilai saya</h1>
          <p className="text-sm text-muted-foreground">
            Rekap total nilai lu di semua kelas. Klik salah satu kelas buat
            buka breakdown bab + ulangan harian.
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => nilaiQ.refetch()}
          disabled={nilaiQ.isFetching}
        >
          <RotateCcw className="size-4" />
          Refresh
        </Button>
      </header>

      <div className="grid gap-4 sm:grid-cols-3">
        <Card>
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-sm">Kelas dengan nilai</CardTitle>
              <CardDescription>Sudah ada bab dinilai</CardDescription>
            </div>
            <TrendingUp className="size-5 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <span className="text-2xl font-semibold tabular-nums">
              {counts.withNilai}
              <span className="ml-1 text-sm font-normal text-muted-foreground">
                / {counts.total}
              </span>
            </span>
          </CardContent>
        </Card>

        <Card className="sm:col-span-2">
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-sm">Rata-rata lintas kelas</CardTitle>
              <CardDescription>
                Mean dari total kelas yang sudah punya nilai. Bukan nilai resmi
                — sekadar gambaran.
              </CardDescription>
            </div>
          </CardHeader>
          <CardContent>
            <span
              className={`text-2xl font-semibold tabular-nums ${totalClass(counts.avgKelas)}`}
            >
              {formatNilai(counts.avgKelas)}
            </span>
          </CardContent>
        </Card>
      </div>

      {nilaiQ.isPending ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <div
              key={i}
              className="h-48 animate-pulse rounded-md border bg-muted/40"
            />
          ))}
        </div>
      ) : nilaiQ.isError ? (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Gagal memuat nilai</CardTitle>
            <CardDescription>
              {nilaiQ.error instanceof ApiError
                ? `${nilaiQ.error.message}${nilaiQ.error.requestId ? ` (req: ${nilaiQ.error.requestId})` : ''}`
                : 'Terjadi kesalahan tidak terduga.'}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => nilaiQ.refetch()}
              disabled={nilaiQ.isFetching}
            >
              <RotateCcw className="size-4" />
              Coba lagi
            </Button>
          </CardContent>
        </Card>
      ) : items.length === 0 ? (
        <Card>
          <CardContent className="p-8 text-center">
            <p className="text-sm text-muted-foreground">
              Lu belum gabung kelas apapun. Gabung kelas dulu pakai kode invite
              di /siswa/gabung — nilai bakal muncul di sini begitu ada bab atau
              tugas yang dinilai.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {items.map((it, index) => (
            <KelasNilaiCard
              key={it.kelas_id}
              item={it}
              detail={detailByKelas.get(it.kelas_id)}
              detailLoading={detailQueries[index]?.isPending}
            />
          ))}
        </div>
      )}
    </div>
  );
}
