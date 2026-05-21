'use client';

/**
 * PengumumanList — guru-side list pengumuman untuk satu kelas (kelas-wide)
 * atau satu bab (bab-scoped).
 *
 * Fitur:
 *   - GET /kelas/:id/pengumuman?bab_id=null|<uuid>&status=&limit= via TanStack Query
 *   - Card per pengumuman: judul + status badge (published/archived) + meta
 *     (created_at, version, badge "Baru" kalau < 7 hari per locked #66)
 *   - Render isi markdown (collapse ke 6 baris, expand-on-demand)
 *   - Aksi per card: Edit (open PengumumanEditDialog) + Archive/Unarchive
 *     (PATCH status) + Hapus (confirm dialog → DELETE)
 *   - Filter status: All / Published / Archived (default: All)
 *   - Tombol "Buat pengumuman" di header → PengumumanComposer
 *
 * Dipakai di /guru/kelas/detail Tab Pengumuman (kelas-wide, babID=null) dan
 * /guru/kelas/detail/bab Tab Pengumuman (bab-scoped, babID=<uuid>).
 */

import * as React from 'react';
import {
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {
  Archive,
  ArchiveRestore,
  ChevronDown,
  ChevronRight,
  Megaphone,
  MoreVertical,
  Plus,
  RotateCcw,
  Sparkles,
  Trash2,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type Pengumuman,
  type PengumumanStatus,
  deletePengumuman,
  friendlyPengumumanError,
  isPengumumanNew,
  listPengumuman,
  updatePengumuman,
} from '@/lib/pengumuman-api';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { PengumumanComposer } from './PengumumanComposer';
import { PengumumanEditDialog } from './PengumumanEditDialog';

export interface PengumumanListProps {
  kelasID: string;
  /** UUID bab tempat pengumuman nempel; null = kelas-wide. */
  babID: string | null;
  /** Title context (mis. "Bab 1 — Pengantar" atau "Pengumuman kelas"). */
  contextLabel: string;
  /** Disable mutations + create/edit/delete (mis. kelas archived). */
  disabled?: boolean;
}

type StatusFilter = 'all' | 'published' | 'archived';

const STATUS_FILTERS: { key: StatusFilter; label: string }[] = [
  { key: 'all', label: 'Semua' },
  { key: 'published', label: 'Published' },
  { key: 'archived', label: 'Archived' },
];

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

function statusBadgeClass(status: PengumumanStatus): string {
  return status === 'published'
    ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300'
    : 'bg-muted text-muted-foreground';
}

export function PengumumanList({
  kelasID,
  babID,
  contextLabel,
  disabled,
}: PengumumanListProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [composerOpen, setComposerOpen] = React.useState(false);
  const [editTarget, setEditTarget] = React.useState<Pengumuman | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<Pengumuman | null>(
    null,
  );
  const [statusFilter, setStatusFilter] = React.useState<StatusFilter>('all');
  const [expanded, setExpanded] = React.useState<Set<string>>(() => new Set());

  const queryKey = React.useMemo(
    () =>
      [
        'guru',
        'pengumuman',
        'list',
        kelasID,
        babID ?? 'kelas-wide',
        statusFilter,
      ] as const,
    [kelasID, babID, statusFilter],
  );

  // Invalidate ALL filter variants pas mutation (status flip pindah bucket).
  const invalidateKeys = React.useMemo(
    () =>
      [
        ['guru', 'pengumuman', 'list', kelasID, babID ?? 'kelas-wide', 'all'] as const,
        ['guru', 'pengumuman', 'list', kelasID, babID ?? 'kelas-wide', 'published'] as const,
        ['guru', 'pengumuman', 'list', kelasID, babID ?? 'kelas-wide', 'archived'] as const,
      ] as const,
    [kelasID, babID],
  );

  const listQuery = useQuery({
    queryKey,
    queryFn: () =>
      listPengumuman(kelasID, {
        babID,
        status: statusFilter === 'all' ? undefined : statusFilter,
        limit: 100,
      }),
    staleTime: 15_000,
  });

  const archiveMutation = useMutation({
    mutationFn: (target: { id: string; version: number; nextStatus: PengumumanStatus }) =>
      updatePengumuman(target.id, {
        version: target.version,
        status: target.nextStatus,
      }),
    onSuccess: ({ pengumuman }) => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title:
          pengumuman.status === 'archived'
            ? 'Pengumuman diarsipkan'
            : 'Pengumuman diaktifkan kembali',
        description:
          pengumuman.status === 'archived'
            ? `"${pengumuman.judul}" disembunyiin dari siswa.`
            : `"${pengumuman.judul}" tampil lagi ke siswa.`,
      });
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      if (apiErr?.code === 'version_conflict') {
        for (const key of invalidateKeys) {
          queryClient.invalidateQueries({ queryKey: key });
        }
      }
      const message = apiErr
        ? friendlyPengumumanError(apiErr, 'archive')
        : 'Gagal mengubah status pengumuman.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal mengubah status',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deletePengumuman(id),
    onSuccess: () => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: 'Pengumuman dihapus',
        description: 'Pengumuman sudah dihapus permanen.',
      });
      setDeleteTarget(null);
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyPengumumanError(apiErr, 'delete')
        : 'Gagal menghapus pengumuman.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal menghapus pengumuman',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  function toggleExpanded(id: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  const items = listQuery.data?.items ?? [];
  const now = React.useMemo(() => new Date(), [listQuery.dataUpdatedAt]);

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-1">
            <CardTitle className="text-base">Pengumuman</CardTitle>
            <CardDescription>{contextLabel}</CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => listQuery.refetch()}
              disabled={listQuery.isFetching}
              type="button"
            >
              <RotateCcw className="size-4" />
              Refresh
            </Button>
            <Button
              size="sm"
              onClick={() => setComposerOpen(true)}
              disabled={disabled || listQuery.isPending}
              type="button"
            >
              <Plus className="size-4" />
              Buat pengumuman
            </Button>
          </div>
        </div>
        <div className="flex items-center gap-1 rounded-md border bg-muted/40 p-0.5 self-start">
          {STATUS_FILTERS.map((f) => {
            const active = statusFilter === f.key;
            return (
              <button
                key={f.key}
                type="button"
                onClick={() => setStatusFilter(f.key)}
                className={cn(
                  'rounded px-2 py-1 text-xs transition-colors',
                  active
                    ? 'bg-background shadow-sm'
                    : 'text-muted-foreground hover:text-foreground',
                )}
              >
                {f.label}
              </button>
            );
          })}
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {listQuery.isPending && (
          <div className="space-y-2">
            <div className="h-20 animate-pulse rounded-md border bg-muted/40" />
            <div className="h-20 animate-pulse rounded-md border bg-muted/40" />
          </div>
        )}

        {listQuery.isError && (
          <div className="rounded-md border border-destructive/50 bg-destructive/5 p-3 text-sm text-destructive">
            Gagal memuat daftar pengumuman.{' '}
            <Button
              variant="link"
              size="sm"
              className="h-auto p-0 align-baseline"
              onClick={() => listQuery.refetch()}
            >
              Coba lagi
            </Button>
            .
          </div>
        )}

        {listQuery.isSuccess && items.length === 0 && (
          <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
            {statusFilter === 'all'
              ? 'Belum ada pengumuman. Klik "Buat pengumuman" untuk mulai.'
              : statusFilter === 'archived'
                ? 'Tidak ada pengumuman yang diarsipkan.'
                : 'Tidak ada pengumuman aktif untuk filter ini.'}
          </div>
        )}

        {listQuery.isSuccess && items.length > 0 && (
          <ul className="space-y-2">
            {items.map((p) => {
              const isOpen = expanded.has(p.id);
              const isNew =
                p.status === 'published' && isPengumumanNew(p, now);
              const isArchived = p.status === 'archived';
              return (
                <li
                  key={p.id}
                  className="rounded-md border bg-background"
                >
                  <div className="flex items-start justify-between gap-3 px-3 py-2.5">
                    <button
                      type="button"
                      onClick={() => toggleExpanded(p.id)}
                      className="flex min-w-0 flex-1 items-start gap-3 text-left"
                      aria-expanded={isOpen}
                    >
                      <span className="mt-0.5">
                        {isOpen ? (
                          <ChevronDown className="size-4 text-muted-foreground" />
                        ) : (
                          <ChevronRight className="size-4 text-muted-foreground" />
                        )}
                      </span>
                      <div className="min-w-0 flex-1 space-y-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <Megaphone className="size-4 shrink-0 text-muted-foreground" />
                          <p
                            className={cn(
                              'truncate font-medium',
                              isArchived && 'text-muted-foreground line-through',
                            )}
                            title={p.judul}
                          >
                            {p.judul}
                          </p>
                          <span
                            className={cn(
                              'rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
                              statusBadgeClass(p.status),
                            )}
                          >
                            {p.status}
                          </span>
                          {isNew && (
                            <span className="inline-flex items-center gap-1 rounded-full bg-amber-50 px-2 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-950 dark:text-amber-300">
                              <Sparkles className="size-3" />
                              Baru
                            </span>
                          )}
                        </div>
                        <p className="text-xs text-muted-foreground">
                          {formatDate(p.created_at)} · v{p.version}
                          {p.updated_at !== p.created_at
                            ? ` · diubah ${formatDate(p.updated_at)}`
                            : ''}
                        </p>
                      </div>
                    </button>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="size-8 p-0"
                          aria-label={`Aksi pengumuman ${p.judul}`}
                        >
                          <MoreVertical className="size-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-44">
                        <DropdownMenuItem
                          onSelect={() => setEditTarget(p)}
                          disabled={disabled}
                        >
                          Edit
                        </DropdownMenuItem>
                        {p.status === 'published' ? (
                          <DropdownMenuItem
                            onSelect={() =>
                              archiveMutation.mutate({
                                id: p.id,
                                version: p.version,
                                nextStatus: 'archived',
                              })
                            }
                            disabled={disabled || archiveMutation.isPending}
                          >
                            <Archive className="size-4" />
                            Arsipkan
                          </DropdownMenuItem>
                        ) : (
                          <DropdownMenuItem
                            onSelect={() =>
                              archiveMutation.mutate({
                                id: p.id,
                                version: p.version,
                                nextStatus: 'published',
                              })
                            }
                            disabled={disabled || archiveMutation.isPending}
                          >
                            <ArchiveRestore className="size-4" />
                            Aktifkan
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          className="text-destructive focus:text-destructive"
                          onSelect={() => setDeleteTarget(p)}
                          disabled={disabled}
                        >
                          <Trash2 className="size-4" />
                          Hapus
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                  {isOpen && (
                    <div className="border-t bg-muted/20 px-3 py-3">
                      {p.isi.trim() ? (
                        <div className="prose prose-sm max-w-none dark:prose-invert">
                          <Markdown remarkPlugins={[remarkGfm]}>
                            {p.isi}
                          </Markdown>
                        </div>
                      ) : (
                        <p className="text-xs italic text-muted-foreground">
                          (Tidak ada isi.)
                        </p>
                      )}
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </CardContent>

      <PengumumanComposer
        open={composerOpen}
        onOpenChange={setComposerOpen}
        kelasID={kelasID}
        babID={babID}
        invalidateKeys={invalidateKeys}
        scopeLabel={contextLabel}
      />

      {editTarget && (
        <PengumumanEditDialog
          open={!!editTarget}
          onOpenChange={(open) => {
            if (!open) setEditTarget(null);
          }}
          pengumuman={editTarget}
          invalidateKeys={invalidateKeys}
        />
      )}

      <Dialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open && !deleteMutation.isPending) setDeleteTarget(null);
        }}
      >
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Hapus pengumuman?</DialogTitle>
            <DialogDescription>
              {deleteTarget
                ? `"${deleteTarget.judul}" akan dihapus permanen. Tidak bisa di-undo.`
                : ''}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setDeleteTarget(null)}
              disabled={deleteMutation.isPending}
            >
              Batal
            </Button>
            <Button
              type="button"
              variant="destructive"
              onClick={() => {
                if (deleteTarget) deleteMutation.mutate(deleteTarget.id);
              }}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending ? 'Menghapus…' : 'Hapus'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
