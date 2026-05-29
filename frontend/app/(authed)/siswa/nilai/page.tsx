'use client';

/**
 * /siswa/nilai — cross-class rekap nilai dashboard (Task 7.A.2).
 *
 * Visual: neo-brutalism + pastel pop. Stat header + KelasNilaiCard dengan
 * kelas tone deterministic + breakdown bab inline.
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
import {
  SECTION_META,
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardDescription,
  SiswaCardHeader,
  SiswaCardTitle,
  SiswaPageHeader,
  SiswaStat,
  kelasToneFromId,
} from '@/components/siswa-ui';

function totalClass(n: number | null): string {
  if (n === null) return 'text-siswa-text-muted';
  if (n >= 75) return 'text-emerald-700';
  if (n >= 60) return 'text-amber-700';
  return 'text-rose-700';
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
  const tone = kelasToneFromId(item.kelas_id);
  const meta = SECTION_META[tone];
  const KelasIcon = meta.Icon;

  return (
    <SiswaCard tone={tone} shadow="md" className="flex h-full flex-col overflow-hidden">
      <div className="flex items-start gap-3 border-b-2 border-siswa-border bg-siswa-surface/70 px-5 py-4">
        <span className="grid size-10 shrink-0 place-items-center rounded-siswa siswa-border bg-siswa-surface">
          <KelasIcon className="size-5" strokeWidth={2.5} />
        </span>
        <div className="min-w-0 flex-1 space-y-0.5">
          <h3 className="siswa-display truncate text-base font-bold leading-tight">
            {item.kelas_nama}
          </h3>
          <p className="flex items-center gap-1 text-xs text-siswa-text-muted">
            <User2 className="size-3" />
            {item.guru_nama || 'guru belum diatur'}
          </p>
        </div>
      </div>
      <div className="flex flex-1 flex-col gap-4 p-5">
        <div>
          <p className="text-xs font-semibold uppercase tracking-wide text-siswa-text-muted">
            Total nilai kelas
          </p>
          <p
            className={`siswa-display text-4xl font-bold tabular-nums ${totalClass(item.total_kelas)}`}
          >
            {formatNilai(item.total_kelas)}
          </p>
          {item.total_kelas === null ? (
            <p className="text-xs text-siswa-text-muted">
              Belum ada bab dengan nilai.
            </p>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2 text-xs font-semibold">
          <span className="rounded-full border-2 border-siswa-border bg-siswa-surface px-2 py-0.5 tabular-nums">
            {item.bab_count} bab
          </span>
          <span className="rounded-full border-2 border-siswa-border bg-siswa-surface px-2 py-0.5 tabular-nums">
            {item.ulangan_count} ulangan harian
          </span>
        </div>
        <div className="rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface">
          <div className="border-b-2 border-siswa-border-soft px-3 py-2 text-xs font-bold uppercase tracking-wide text-siswa-text-muted">
            Breakdown nilai per bab
          </div>
          {detailLoading ? (
            <div className="space-y-2 p-3">
              <div className="h-4 animate-pulse rounded bg-siswa-text/10" />
              <div className="h-4 animate-pulse rounded bg-siswa-text/10" />
            </div>
          ) : babRows.length === 0 ? (
            <p className="p-3 text-xs text-siswa-text-muted">
              Belum ada bab yang punya data nilai.
            </p>
          ) : (
            <div className="divide-y-2 divide-siswa-border-soft">
              {babRows.map((bab) => (
                <div
                  key={bab.bab_id}
                  className="grid grid-cols-[1fr_auto] gap-3 px-3 py-2 text-xs"
                >
                  <div className="min-w-0">
                    <p className="truncate font-semibold">
                      Bab {bab.nomor}. {bab.judul}
                    </p>
                    <p className="text-siswa-text-muted">
                      Tugas {formatNilai(bab.nilai_tugas_bab)} · Ulangan{' '}
                      {formatNilai(bab.nilai_ulangan_bab)}
                    </p>
                  </div>
                  <div
                    className={`self-center text-right font-bold tabular-nums ${totalClass(bab.total)}`}
                  >
                    {formatNilai(bab.total)}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
        <div className="mt-auto pt-2">
          <SiswaButton asChild tone="surface" size="sm" className="w-full">
            <Link href={`/siswa/kelas/detail/nilai?id=${item.kelas_id}`}>
              Lihat detail
              <ArrowRight className="size-4" strokeWidth={2.5} />
            </Link>
          </SiswaButton>
        </div>
      </div>
    </SiswaCard>
  );
}

export default function SiswaNilaiPage() {
  const nilaiQ = useQuery({
    queryKey: ['siswa', 'nilai', 'list'],
    queryFn: () => listSiswaNilai(),
    staleTime: 15_000,
  });

  const items = React.useMemo(
    () => nilaiQ.data?.items ?? [],
    [nilaiQ.data?.items],
  );

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
      <SiswaPageHeader
        eyebrow="Nilai saya"
        title="Rekap nilai"
        description="Total nilai kamu di semua kelas. Klik salah satu kelas buat buka breakdown bab + ulangan harian."
        actions={
          <SiswaButton
            type="button"
            tone="surface"
            size="sm"
            onClick={() => nilaiQ.refetch()}
            disabled={nilaiQ.isFetching}
          >
            <RotateCcw className="size-4" />
            Refresh
          </SiswaButton>
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <SiswaStat
          label="Kelas dengan nilai"
          value={
            <>
              {counts.withNilai}
              <span className="ml-1 text-base font-semibold text-siswa-text-muted">
                / {counts.total}
              </span>
            </>
          }
          hint="Sudah ada bab dinilai"
          Icon={TrendingUp}
          tone="nilai"
        />
        <SiswaStat
          label="Rata-rata kelas"
          value={formatNilai(counts.avgKelas)}
          hint="Mean dari kelas berdata"
          Icon={TrendingUp}
          tone="latihan"
          className={counts.avgKelas != null ? '' : ''}
        />
        <SiswaCard tone="umum" shadow="sm" className="p-5">
          <p className="text-xs font-semibold uppercase tracking-wide text-siswa-text-muted">
            Catatan
          </p>
          <p className="mt-1 text-sm text-siswa-text-muted">
            Mean lintas-kelas bukan nilai resmi. Setiap kelas punya bobot
            sendiri yang diatur guru.
          </p>
        </SiswaCard>
      </div>

      {nilaiQ.isPending ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <div
              key={i}
              className="h-48 animate-pulse rounded-siswa siswa-border bg-siswa-surface/60"
            />
          ))}
        </div>
      ) : nilaiQ.isError ? (
        <SiswaCard tone="surface" shadow="md">
          <SiswaCardHeader>
            <SiswaCardTitle>Gagal memuat nilai</SiswaCardTitle>
            <SiswaCardDescription>
              {nilaiQ.error instanceof ApiError
                ? `${nilaiQ.error.message}${nilaiQ.error.requestId ? ` (req: ${nilaiQ.error.requestId})` : ''}`
                : 'Terjadi kesalahan tidak terduga.'}
            </SiswaCardDescription>
          </SiswaCardHeader>
          <SiswaCardBody>
            <SiswaButton
              type="button"
              tone="surface"
              size="sm"
              onClick={() => nilaiQ.refetch()}
              disabled={nilaiQ.isFetching}
            >
              <RotateCcw className="size-4" />
              Coba lagi
            </SiswaButton>
          </SiswaCardBody>
        </SiswaCard>
      ) : items.length === 0 ? (
        <SiswaCard tone="surface" shadow="md">
          <SiswaCardBody className="p-8 text-center">
            <p className="text-sm text-siswa-text-muted">
              Kamu belum gabung kelas apapun. Gabung kelas dulu pakai kode invite
              di /siswa/gabung — nilai bakal muncul di sini begitu ada bab atau
              tugas yang dinilai.
            </p>
          </SiswaCardBody>
        </SiswaCard>
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
