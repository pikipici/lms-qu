'use client';

/**
 * PengumumanReadList — siswa-side read-only list pengumuman.
 *
 * Visual: neo-brutalism + pastel pop. Card per pengumuman dengan badge
 * "Baru" pop kuning kalau < 7 hari (locked #66). Body markdown via
 * react-markdown, expand-on-demand.
 */

import * as React from 'react';
import { useQuery } from '@tanstack/react-query';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {
  ChevronDown,
  ChevronRight,
  Megaphone,
  Paperclip,
  RotateCcw,
  Sparkles,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type Pengumuman,
  getPengumumanAttachmentURL,
  isPengumumanNew,
  listSiswaPengumuman,
  pengumumanAttachments,
} from '@/lib/pengumuman-api';
import { cn } from '@/lib/utils';
import { SiswaBadge, SiswaButton } from '@/components/siswa-ui';

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

  const items = React.useMemo(
    () => listQuery.data?.items ?? [],
    [listQuery.data?.items],
  );
  const now = React.useMemo(
    () =>
      listQuery.dataUpdatedAt ? new Date(listQuery.dataUpdatedAt) : new Date(),
    [listQuery.dataUpdatedAt],
  );

  const [expanded, setExpanded] = React.useState<Set<string>>(() => new Set());
  const wasInitialized = React.useRef(false);

  React.useEffect(() => {
    if (expandFirst && !wasInitialized.current && items.length > 0) {
      const first = items[0];
      if (first) {
        wasInitialized.current = true;
        setExpanded(new Set([first.id]));
      }
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
      <div className="space-y-3">
        {[0, 1].map((i) => (
          <div
            key={i}
            className="h-20 animate-pulse rounded-siswa siswa-border bg-siswa-surface/60"
          />
        ))}
      </div>
    );
  }

  if (listQuery.isError) {
    const err = listQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    const isForbidden = apiErr?.code === 'forbidden';
    return (
      <div className="space-y-3 rounded-siswa border-2 border-siswa-danger bg-siswa-surface p-4 text-sm">
        <p className="font-bold">
          {isForbidden ? 'Akses ditolak' : 'Gagal memuat pengumuman'}
        </p>
        <p className="text-siswa-text-muted">
          {isForbidden
            ? 'Lu tidak terdaftar aktif di kelas ini.'
            : apiErr?.message ?? 'Terjadi kesalahan tidak terduga.'}
          {apiErr?.requestId ? ` (req: ${apiErr.requestId})` : ''}
        </p>
        <SiswaButton
          type="button"
          tone="surface"
          size="sm"
          onClick={() => listQuery.refetch()}
          disabled={listQuery.isFetching}
        >
          <RotateCcw className="size-4" />
          Coba lagi
        </SiswaButton>
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div className="rounded-siswa border-2 border-dashed border-siswa-border-soft bg-siswa-surface/60 p-6 text-center">
        <Megaphone
          className="mx-auto mb-2 size-8 text-siswa-text-muted"
          strokeWidth={2.5}
        />
        <p className="text-sm text-siswa-text-muted">
          {emptyState ?? 'Belum ada pengumuman.'}
        </p>
      </div>
    );
  }

  return (
    <ul className="space-y-3">
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
  const attachments = pengumumanAttachments(pengumuman);

  async function openAttachment(attachmentID: string) {
    try {
      const { url } = await getPengumumanAttachmentURL(pengumuman.id, attachmentID);
      window.open(url, '_blank', 'noopener,noreferrer');
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Lampiran tidak bisa dibuka.';
      window.alert(message);
    }
  }

  return (
    <li className="overflow-hidden rounded-siswa siswa-border bg-siswa-surface siswa-shadow-sm">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        className="flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-siswa-cream/60"
      >
        <span className="grid size-8 shrink-0 place-items-center rounded-siswa siswa-border bg-siswa-cream/70">
          <Megaphone className="size-4" strokeWidth={2.5} />
        </span>
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <p
              className={cn('siswa-display truncate text-sm font-bold')}
              title={pengumuman.judul}
            >
              {pengumuman.judul}
            </p>
            {isNew ? (
              <SiswaBadge tone="yellow">
                <Sparkles className="size-3" strokeWidth={2.5} />
                Baru
              </SiswaBadge>
            ) : null}
          </div>
          <p className="text-xs text-siswa-text-muted">{createdAt}</p>
        </div>
        {expanded ? (
          <ChevronDown
            className="mt-1 size-4 shrink-0 text-siswa-text-muted"
            strokeWidth={2.5}
          />
        ) : (
          <ChevronRight
            className="mt-1 size-4 shrink-0 text-siswa-text-muted"
            strokeWidth={2.5}
          />
        )}
      </button>
      {expanded ? (
        <div className="border-t-2 border-siswa-border bg-siswa-bg px-4 py-3">
          {pengumuman.isi.trim() ? (
            <div className="prose prose-sm max-w-none text-siswa-text">
              <Markdown remarkPlugins={[remarkGfm]}>{pengumuman.isi}</Markdown>
            </div>
          ) : (
            <p className="text-xs italic text-siswa-text-muted">
              Pengumuman ini tidak punya isi.
            </p>
          )}
          {attachments.length > 0 ? (
            <div className="mt-3 flex flex-wrap gap-2">
              {attachments.map((attachment) => (
                <SiswaButton key={attachment.id} type="button" tone="surface" size="sm" onClick={() => openAttachment(attachment.id)}>
                  <Paperclip className="mr-2 size-4" />
                  {attachment.original_filename}
                </SiswaButton>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}
    </li>
  );
}
