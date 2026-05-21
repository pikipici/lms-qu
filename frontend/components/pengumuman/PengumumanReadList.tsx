'use client';

/**
 * PengumumanReadList — siswa-side read-only list pengumuman.
 *
 * Backend pakai endpoint `/siswa/kelas/:id/pengumuman?bab_id=...` yang
 * server-side udah filter status=published + enrollment guard. Siswa
 * gak punya endpoint untuk get archived — kalau guru archive, langsung
 * hilang dari sini.
 *
 * Render:
 *   - Card per pengumuman, sort newest (sudah di-handle BE).
 *   - Badge "Baru" kalau created_at < 7 hari (locked #66, calc client-side).
 *   - Body markdown via react-markdown, expand-on-demand untuk hemat space.
 *   - No mark-read action (locked #66 passive timestamp).
 *
 * Dipakai di /siswa/kelas/detail Tab Pengumuman (kelas-wide, babID=null) dan
 * /siswa/kelas/detail/bab Tab Pengumuman (bab-scoped, babID=<uuid>).
 */

import * as React from 'react';
import { useQuery } from '@tanstack/react-query';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {
  ChevronDown,
  ChevronRight,
  Megaphone,
  RotateCcw,
  Sparkles,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type Pengumuman,
  isPengumumanNew,
  listSiswaPengumuman,
} from '@/lib/pengumuman-api';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

export interface PengumumanReadListProps {
  kelasID: string;
  /** UUID bab tempat pengumuman nempel; null = kelas-wide. */
  babID: string | null;
  /** Optional context label (mis. "Pengumuman bab" / "Pengumuman kelas"). */
  emptyState?: string;
  /** Default semua card collapsed. Set true untuk expand pertama. */
  expandFirst?: boolean;
}

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

export function PengumumanReadList({
  kelasID,
  babID,
  emptyState,
  expandFirst,
}: PengumumanReadListProps) {
  const queryKey = React.useMemo(
    () =>
      ['siswa', 'pengumuman', 'list', kelasID, babID ?? 'kelas-wide'] as const,
    [kelasID, babID],
  );

  const listQuery = useQuery({
    queryKey,
    queryFn: () =>
      listSiswaPengumuman(kelasID, {
        babID,
        limit: 100,
      }),
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

  const items = listQuery.data?.items ?? [];
  const now = React.useMemo(
    () => new Date(),
    [listQuery.dataUpdatedAt],
  );

  const [expanded, setExpanded] = React.useState<Set<string>>(() => new Set());
  const wasInitialized = React.useRef(false);

  // Expand pertama hanya sekali (saat data pertama datang) kalau prop set.
  React.useEffect(() => {
    if (
      expandFirst &&
      !wasInitialized.current &&
      items.length > 0
    ) {
      wasInitialized.current = true;
      setExpanded(new Set([items[0].id]));
    }
  }, [expandFirst, items]);

  function toggleExpanded(id: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  if (listQuery.isPending) {
    return (
      <div className="space-y-2">
        {[0, 1].map((i) => (
          <div
            key={i}
            className="h-20 animate-pulse rounded-md border bg-muted/40"
          />
        ))}
      </div>
    );
  }

  if (listQuery.isError) {
    const err = listQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    const isForbidden = apiErr?.code === 'forbidden';
    const requestId = apiErr?.requestId;
    return (
      <div className="space-y-2 rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
        <p className="font-medium">
          {isForbidden ? 'Akses ditolak' : 'Gagal memuat pengumuman'}
        </p>
        <p>
          {isForbidden
            ? 'Lu tidak terdaftar aktif di kelas ini.'
            : apiErr?.message ?? 'Terjadi kesalahan tidak terduga.'}
          {requestId ? ` (req: ${requestId})` : ''}
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => listQuery.refetch()}
          disabled={listQuery.isFetching}
        >
          <RotateCcw className="size-4" />
          Coba lagi
        </Button>
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-6 text-center">
        <Megaphone className="mx-auto mb-2 size-6 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          {emptyState ?? 'Belum ada pengumuman.'}
        </p>
      </div>
    );
  }

  return (
    <ul className="space-y-2">
      {items.map((p) => (
        <PengumumanReadCard
          key={p.id}
          pengumuman={p}
          expanded={expanded.has(p.id)}
          onToggle={() => toggleExpanded(p.id)}
          isNew={isPengumumanNew(p, now)}
          createdAt={formatDate(p.created_at)}
        />
      ))}
    </ul>
  );
}

interface PengumumanReadCardProps {
  pengumuman: Pengumuman;
  expanded: boolean;
  onToggle: () => void;
  isNew: boolean;
  createdAt: string;
}

function PengumumanReadCard({
  pengumuman,
  expanded,
  onToggle,
  isNew,
  createdAt,
}: PengumumanReadCardProps) {
  return (
    <li className="rounded-md border bg-card">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        className="flex w-full items-start gap-3 px-3 py-2.5 text-left hover:bg-accent/40"
      >
        <span className="mt-0.5">
          {expanded ? (
            <ChevronDown className="size-4 text-muted-foreground" />
          ) : (
            <ChevronRight className="size-4 text-muted-foreground" />
          )}
        </span>
        <Megaphone className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <p
              className={cn('truncate text-sm font-medium')}
              title={pengumuman.judul}
            >
              {pengumuman.judul}
            </p>
            {isNew && (
              <span className="inline-flex items-center gap-1 rounded-full bg-amber-50 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-950 dark:text-amber-300">
                <Sparkles className="size-3" />
                Baru
              </span>
            )}
          </div>
          <p className="text-xs text-muted-foreground">{createdAt}</p>
        </div>
      </button>
      {expanded && (
        <div className="border-t bg-background px-3 py-3">
          {pengumuman.isi.trim() ? (
            <div className="prose prose-sm max-w-none dark:prose-invert">
              <Markdown remarkPlugins={[remarkGfm]}>{pengumuman.isi}</Markdown>
            </div>
          ) : (
            <p className="text-xs italic text-muted-foreground">
              Pengumuman ini tidak punya isi.
            </p>
          )}
        </div>
      )}
    </li>
  );
}
