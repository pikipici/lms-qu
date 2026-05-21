'use client';

/**
 * TugasList — guru-side list tugas untuk satu kelas (kelas-wide) atau
 * satu bab (bab-scoped).
 *
 * Fitur:
 *   - GET /kelas/:id/tugas?bab_id=null|<uuid>&status=&limit= via TanStack Query
 *   - Card per tugas: judul + status badge (draft/published/archived) + meta
 *     (deadline, attachment count, version) + badge "Telat" kalau overdue
 *   - Render deskripsi markdown (collapse, expand-on-demand)
 *   - Aksi per card: Edit (open TugasEditDialog) + Publish/Archive/Reactivate
 *     + Hapus (confirm dialog → DELETE)
 *   - Filter status: All / Draft / Published / Archived (default: All)
 *   - Tombol "Buat tugas" di header → TugasComposer (auto-open Edit setelah
 *     create supaya guru langsung bisa upload attachment)
 *
 * Dipakai di /guru/kelas/detail Tab Tugas (kelas-wide, babID=null) dan
 * /guru/kelas/detail/bab Tab Tugas (bab-scoped, babID=<uuid>).
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
  ClipboardList,
  Clock,
  Eye,
  FileText,
  MoreVertical,
  Paperclip,
  Plus,
  Rocket,
  RotateCcw,
  Trash2,
} from 'lucide-react';
import Link from 'next/link';

import { ApiError } from '@/lib/api';
import {
  type Tugas,
  type TugasStatus,
  deleteTugas,
  formatDeadline,
  friendlyTugasError,
  isOverdue,
  listTugas,
  updateTugas,
} from '@/lib/tugas-api';
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
import { TugasComposer } from './TugasComposer';
import { TugasEditDialog } from './TugasEditDialog';

export interface TugasListProps {
  kelasID: string;
  /** UUID bab tempat tugas nempel; null = kelas-wide. */
  babID: string | null;
  /** Title context (mis. "Bab 1 — Pengantar" atau "Tugas kelas"). */
  contextLabel: string;
  /** Disable mutations + create/edit/delete (mis. kelas archived). */
  disabled?: boolean;
}

type StatusFilter = 'all' | 'draft' | 'published' | 'archived';

const STATUS_FILTERS: { key: StatusFilter; label: string }[] = [
  { key: 'all', label: 'Semua' },
  { key: 'draft', label: 'Draft' },
  { key: 'published', label: 'Published' },
  { key: 'archived', label: 'Archived' },
];

function statusBadgeClass(status: TugasStatus): string {
  switch (status) {
    case 'published':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300';
    case 'draft':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-950 dark:text-amber-300';
    case 'archived':
    default:
      return 'bg-muted text-muted-foreground';
  }
}

export function TugasList({
  kelasID,
  babID,
  contextLabel,
  disabled,
}: TugasListProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [composerOpen, setComposerOpen] = React.useState(false);
  const [editTarget, setEditTarget] = React.useState<Tugas | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<Tugas | null>(null);
  const [statusFilter, setStatusFilter] = React.useState<StatusFilter>('all');
  const [expanded, setExpanded] = React.useState<Set<string>>(() => new Set());

  const queryKey = React.useMemo(
    () =>
      [
        'guru',
        'tugas',
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
        ['guru', 'tugas', 'list', kelasID, babID ?? 'kelas-wide', 'all'] as const,
        ['guru', 'tugas', 'list', kelasID, babID ?? 'kelas-wide', 'draft'] as const,
        ['guru', 'tugas', 'list', kelasID, babID ?? 'kelas-wide', 'published'] as const,
        ['guru', 'tugas', 'list', kelasID, babID ?? 'kelas-wide', 'archived'] as const,
      ] as const,
    [kelasID, babID],
  );

  const listQuery = useQuery({
    queryKey,
    queryFn: () =>
      listTugas(kelasID, {
        babID,
        status: statusFilter === 'all' ? undefined : statusFilter,
        limit: 100,
      }),
    staleTime: 15_000,
  });

  const statusMutation = useMutation({
    mutationFn: (target: {
      id: string;
      version: number;
      nextStatus: TugasStatus;
    }) =>
      updateTugas(target.id, {
        version: target.version,
        status: target.nextStatus,
      }),
    onSuccess: ({ tugas }) => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      const titles: Record<TugasStatus, string> = {
        published: 'Tugas dipublish',
        archived: 'Tugas diarsipkan',
        draft: 'Tugas dikembalikan ke draft',
      };
      const desc: Record<TugasStatus, string> = {
        published: `"${tugas.judul}" terbit ke siswa enrolled.`,
        archived: `"${tugas.judul}" disembunyiin dari siswa.`,
        draft: `"${tugas.judul}" disembunyiin dari siswa.`,
      };
      toast({
        title: titles[tugas.status],
        description: desc[tugas.status],
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
        ? friendlyTugasError(apiErr, 'archive')
        : 'Gagal mengubah status tugas.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal mengubah status',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteTugas(id),
    onSuccess: () => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: 'Tugas dihapus',
        description: 'Tugas dan lampirannya sudah dihapus permanen.',
      });
      setDeleteTarget(null);
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyTugasError(apiErr, 'delete')
        : 'Gagal menghapus tugas.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal menghapus tugas',
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
  const now = React.useMemo(
    () =>
      listQuery.dataUpdatedAt
        ? new Date(listQuery.dataUpdatedAt)
        : new Date(),
    [listQuery.dataUpdatedAt],
  );

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-1">
            <CardTitle className="text-base">Tugas</CardTitle>
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
              Buat tugas
            </Button>
          </div>
        </div>
        <div className="flex items-center gap-1 self-start rounded-md border bg-muted/40 p-0.5">
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
            Gagal memuat daftar tugas.{' '}
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
              ? 'Belum ada tugas. Klik "Buat tugas" untuk mulai.'
              : statusFilter === 'archived'
                ? 'Tidak ada tugas yang diarsipkan.'
                : statusFilter === 'draft'
                  ? 'Tidak ada draft tersimpan.'
                  : 'Tidak ada tugas yang dipublish.'}
          </div>
        )}

        {listQuery.isSuccess && items.length > 0 && (
          <ul className="space-y-2">
            {items.map((t) => {
              const isOpen = expanded.has(t.id);
              const overdue = isOverdue(t, now);
              const isArchived = t.status === 'archived';
              const isDraft = t.status === 'draft';
              const attachmentCount = t.attachments?.length ?? 0;
              return (
                <li key={t.id} className="rounded-md border bg-background">
                  <div className="flex items-start justify-between gap-3 px-3 py-2.5">
                    <button
                      type="button"
                      onClick={() => toggleExpanded(t.id)}
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
                          <ClipboardList className="size-4 shrink-0 text-muted-foreground" />
                          <p
                            className={cn(
                              'truncate font-medium',
                              isArchived &&
                                'text-muted-foreground line-through',
                              isDraft && 'text-muted-foreground',
                            )}
                            title={t.judul}
                          >
                            {t.judul}
                          </p>
                          <span
                            className={cn(
                              'rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
                              statusBadgeClass(t.status),
                            )}
                          >
                            {t.status}
                          </span>
                          {overdue && t.status === 'published' && (
                            <span className="inline-flex items-center gap-1 rounded-full bg-rose-50 px-2 py-0.5 text-[10px] font-medium text-rose-700 dark:bg-rose-950 dark:text-rose-300">
                              <Clock className="size-3" />
                              Lewat deadline
                            </span>
                          )}
                          {attachmentCount > 0 && (
                            <span className="inline-flex items-center gap-1 rounded-full bg-muted/60 px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
                              <Paperclip className="size-3" />
                              {attachmentCount}
                            </span>
                          )}
                        </div>
                        <p className="text-xs text-muted-foreground">
                          <Clock className="mr-1 inline size-3" />
                          {formatDeadline(t.deadline)} · v{t.version}
                          {t.izinkan_late && t.deadline
                            ? ` · late penalty ${t.penalty_persen}%`
                            : ''}
                          {t.wajib_attachment ? ' · wajib lampiran' : ''}
                        </p>
                      </div>
                    </button>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="size-8 p-0"
                          aria-label={`Aksi tugas ${t.judul}`}
                        >
                          <MoreVertical className="size-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-44">
                        <DropdownMenuItem asChild>
                          <Link
                            href={`/guru/kelas/detail/tugas?id=${kelasID}&tid=${t.id}`}
                          >
                            <Eye className="size-4" />
                            Lihat Submission
                          </Link>
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onSelect={() => setEditTarget(t)}
                          disabled={disabled}
                        >
                          <FileText className="size-4" />
                          Edit
                        </DropdownMenuItem>
                        {t.status === 'draft' && (
                          <DropdownMenuItem
                            onSelect={() =>
                              statusMutation.mutate({
                                id: t.id,
                                version: t.version,
                                nextStatus: 'published',
                              })
                            }
                            disabled={disabled || statusMutation.isPending}
                          >
                            <Rocket className="size-4" />
                            Publish
                          </DropdownMenuItem>
                        )}
                        {t.status === 'published' && (
                          <DropdownMenuItem
                            onSelect={() =>
                              statusMutation.mutate({
                                id: t.id,
                                version: t.version,
                                nextStatus: 'archived',
                              })
                            }
                            disabled={disabled || statusMutation.isPending}
                          >
                            <Archive className="size-4" />
                            Arsipkan
                          </DropdownMenuItem>
                        )}
                        {t.status === 'archived' && (
                          <DropdownMenuItem
                            onSelect={() =>
                              statusMutation.mutate({
                                id: t.id,
                                version: t.version,
                                nextStatus: 'published',
                              })
                            }
                            disabled={disabled || statusMutation.isPending}
                          >
                            <ArchiveRestore className="size-4" />
                            Aktifkan
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          className="text-destructive focus:text-destructive"
                          onSelect={() => setDeleteTarget(t)}
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
                      {t.deskripsi.trim() ? (
                        <div className="prose prose-sm max-w-none dark:prose-invert">
                          <Markdown remarkPlugins={[remarkGfm]}>
                            {t.deskripsi}
                          </Markdown>
                        </div>
                      ) : (
                        <p className="text-xs italic text-muted-foreground">
                          (Belum ada deskripsi.)
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

      <TugasComposer
        open={composerOpen}
        onOpenChange={setComposerOpen}
        kelasID={kelasID}
        babID={babID}
        invalidateKeys={invalidateKeys}
        scopeLabel={contextLabel}
        onCreated={(t) => setEditTarget(t)}
      />

      {editTarget && (
        <TugasEditDialog
          open={!!editTarget}
          onOpenChange={(open) => {
            if (!open) setEditTarget(null);
          }}
          tugas={editTarget}
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
            <DialogTitle>Hapus tugas?</DialogTitle>
            <DialogDescription>
              {deleteTarget
                ? `"${deleteTarget.judul}" akan dihapus permanen beserta semua lampirannya. Tidak bisa di-undo.`
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
