'use client';

/**
 * GuruFeedList — activity feed card untuk dashboard guru.
 *
 * Polling 30s default (TanStack refetchInterval). Load-more via cursor.
 * 3 event kind:
 *   - submission_baru → "Siswa X submit tugas Y di kelas Z" + late badge
 *   - ulangan_selesai → "Siswa X selesai ulangan Y · nilai N"
 *   - siswa_join      → "Siswa X gabung kelas Z"
 *
 * Setiap row clickable: link langsung ke kelas/tugas detail context-aware.
 */

import * as React from 'react';
import Link from 'next/link';
import { useInfiniteQuery } from '@tanstack/react-query';
import {
  Activity,
  ArrowRight,
  ClipboardList,
  GraduationCap,
  RotateCcw,
  UserPlus,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  listGuruFeed,
  type FeedEvent,
  type FeedListResponse,
} from '@/lib/feed-api';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface Props {
  /** Polling interval ms. 0 → disable polling. */
  pollMs?: number;
  /** Page size. Server clamps 1..50. */
  pageSize?: number;
}

function formatRelative(iso: string): string {
  try {
    const t = new Date(iso).getTime();
    const diff = Date.now() - t;
    const s = Math.floor(diff / 1000);
    if (s < 5) return 'baru saja';
    if (s < 60) return `${s} detik lalu`;
    const m = Math.floor(s / 60);
    if (m < 60) return `${m} menit lalu`;
    const h = Math.floor(m / 60);
    if (h < 24) return `${h} jam lalu`;
    const d = Math.floor(h / 24);
    if (d < 7) return `${d} hari lalu`;
    return new Date(iso).toLocaleDateString('id-ID', {
      day: 'numeric',
      month: 'short',
      year: 'numeric',
    });
  } catch {
    return iso;
  }
}

function formatNilai(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—';
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

function eventLink(ev: FeedEvent): string {
  switch (ev.kind) {
    case 'submission_baru':
      return ev.tugas_id
        ? `/guru/kelas/detail/tugas?id=${ev.kelas_id}&tid=${ev.tugas_id}`
        : `/guru/kelas/detail?id=${ev.kelas_id}`;
    case 'ulangan_selesai':
      return ev.ujian_id
        ? `/guru/kelas/detail?id=${ev.kelas_id}&tab=ujian`
        : `/guru/kelas/detail?id=${ev.kelas_id}`;
    case 'siswa_join':
      return `/guru/kelas/detail?id=${ev.kelas_id}`;
  }
}

function EventRow({ ev }: { ev: FeedEvent }) {
  const Icon =
    ev.kind === 'submission_baru'
      ? ClipboardList
      : ev.kind === 'ulangan_selesai'
        ? GraduationCap
        : UserPlus;
  const iconClass =
    ev.kind === 'submission_baru'
      ? 'text-blue-600 dark:text-blue-400'
      : ev.kind === 'ulangan_selesai'
        ? 'text-emerald-600 dark:text-emerald-400'
        : 'text-amber-600 dark:text-amber-400';

  let body: React.ReactNode;
  if (ev.kind === 'submission_baru') {
    body = (
      <>
        <span className="font-medium">{ev.siswa_nama || 'Siswa'}</span> submit
        tugas{' '}
        <span className="font-medium">{ev.tugas_judul || 'tanpa judul'}</span>
        {ev.is_late && (
          <span className="ml-1 inline-flex items-center rounded bg-rose-500/15 px-1.5 py-0.5 text-[10px] font-medium text-rose-700 dark:text-rose-300">
            LATE
          </span>
        )}
      </>
    );
  } else if (ev.kind === 'ulangan_selesai') {
    body = (
      <>
        <span className="font-medium">{ev.siswa_nama || 'Siswa'}</span>{' '}
        selesai ulangan{' '}
        <span className="font-medium">{ev.ujian_judul || 'tanpa judul'}</span>
        {ev.nilai_total !== null && ev.nilai_total !== undefined && (
          <span className="ml-1 inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium tabular-nums text-foreground">
            nilai {formatNilai(ev.nilai_total)}
          </span>
        )}
      </>
    );
  } else {
    body = (
      <>
        <span className="font-medium">{ev.siswa_nama || 'Siswa'}</span> gabung
        kelas
      </>
    );
  }

  return (
    <li>
      <Link
        href={eventLink(ev)}
        className="flex items-start gap-3 px-3 py-2.5 transition-colors hover:bg-muted/40 focus-visible:outline-none focus-visible:bg-muted/40"
      >
        <Icon className={cn('mt-0.5 size-4 shrink-0', iconClass)} />
        <div className="min-w-0 flex-1 text-sm">
          <p className="text-foreground">{body}</p>
          <p className="text-xs text-muted-foreground">
            <span className="text-foreground/80">{ev.kelas_nama}</span>
            {' · '}
            {formatRelative(ev.at)}
          </p>
        </div>
        <ArrowRight className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
      </Link>
    </li>
  );
}

export function GuruFeedList({ pollMs = 30_000, pageSize = 20 }: Props) {
  const q = useInfiniteQuery<FeedListResponse, Error>({
    queryKey: ['guru', 'feed', pageSize],
    queryFn: ({ pageParam }) =>
      listGuruFeed({ cursor: (pageParam as string) || undefined, limit: pageSize }),
    initialPageParam: '',
    getNextPageParam: (last) => last.next_cursor || undefined,
    staleTime: 15_000,
    refetchInterval: pollMs > 0 ? pollMs : false,
    refetchOnWindowFocus: true,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
        return false;
      }
      return failureCount < 2;
    },
  });

  if (q.isPending) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 3 }).map((_, i) => (
          <div
            key={i}
            className="h-12 animate-pulse rounded-md border bg-muted/40"
          />
        ))}
      </div>
    );
  }

  if (q.isError) {
    const err = q.error;
    const apiErr = err instanceof ApiError ? err : null;
    return (
      <div className="space-y-2 rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
        <p className="font-medium">Gagal memuat aktivitas</p>
        <p>
          {apiErr?.message ?? 'Coba refresh halaman.'}
          {apiErr?.requestId ? ` (req: ${apiErr.requestId})` : ''}
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => q.refetch()}
          disabled={q.isFetching}
        >
          <RotateCcw className="size-4" />
          Coba lagi
        </Button>
      </div>
    );
  }

  const events = q.data.pages.flatMap((p) => p.events);

  if (events.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-8 text-center">
        <Activity className="mx-auto mb-2 size-6 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          Belum ada aktivitas. Submission tugas, ulangan harian selesai, dan
          siswa join bakal muncul di sini.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <ul className="divide-y rounded-md border">
        {events.map((ev) => (
          <EventRow key={`${ev.kind}-${ev.id}`} ev={ev} />
        ))}
      </ul>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">
          {events.length} aktivitas{' '}
          {q.isFetching && <span className="ml-1">· memuat…</span>}
        </p>
        <div className="flex gap-2">
          {q.hasNextPage && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => q.fetchNextPage()}
              disabled={q.isFetchingNextPage}
            >
              Muat lebih banyak
            </Button>
          )}
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => q.refetch()}
            disabled={q.isFetching}
          >
            <RotateCcw className="size-4" />
            Refresh
          </Button>
        </div>
      </div>
    </div>
  );
}
