'use client';

/**
 * MateriList — guru-side list materi untuk satu bab (atau "berdiri bebas"
 * = bab_id IS NULL).
 *
 * Fitur:
 *   - GET /kelas/:id/materi?bab_id=<uuid|null> via TanStack Query
 *   - Card per materi: icon-per-tipe + judul + meta (urutan, ukuran utk PDF)
 *   - Aksi per card: Edit (open MateriEditDialog) + Hapus (confirm dialog)
 *   - Tombol "Tambah materi" di header → MateriCreateDialog
 *   - Empty state tertaut info bab/free-floating
 *
 * Pola mirroring BabSortableCard untuk DropdownMenu, hanya di-flatten
 * karena materi tidak butuh archive/duplicate (delete cukup; tipe immutable
 * jadi user delete + create ulang kalau perlu ganti).
 *
 * Dipakai di /guru/kelas/detail/bab page Tab Materi (Task 3.D.1).
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  ExternalLink,
  FileText,
  MoreVertical,
  Plus,
  RotateCcw,
  Trash2,
  Type,
  Youtube,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type Materi,
  type MateriTipe,
  deleteMateri,
  friendlyMateriError,
  getMateriFileURL,
  listMateri,
} from '@/lib/materi-api';
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
import { MateriCreateDialog } from './MateriCreateDialog';
import { MateriEditDialog } from './MateriEditDialog';

interface MateriListProps {
  kelasID: string;
  /** UUID bab tempat materi nempel; null = list materi berdiri bebas (locked #20). */
  babID: string | null;
  /** Title context (bab nomor + judul, atau "Materi berdiri bebas"). */
  contextLabel: string;
  /** Disable mutations + create/edit/delete (mis. kelas/bab archived). */
  disabled?: boolean;
}

function tipeIcon(t: MateriTipe) {
  switch (t) {
    case 'pdf':
      return FileText;
    case 'youtube':
      return Youtube;
    case 'markdown':
      return Type;
  }
}

function tipeBadge(t: MateriTipe) {
  switch (t) {
    case 'pdf':
      return { label: 'PDF', class: 'bg-rose-50 text-rose-700 dark:bg-rose-950 dark:text-rose-300' };
    case 'youtube':
      return { label: 'YouTube', class: 'bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300' };
    case 'markdown':
      return { label: 'Markdown', class: 'bg-sky-50 text-sky-700 dark:bg-sky-950 dark:text-sky-300' };
  }
}

function formatBytes(n: number | null | undefined): string {
  if (!n || n <= 0) return '';
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export function MateriList({
  kelasID,
  babID,
  contextLabel,
  disabled,
}: MateriListProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = React.useState(false);
  const [editTarget, setEditTarget] = React.useState<Materi | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<Materi | null>(null);

  const queryKey = React.useMemo(
    () =>
      ['guru', 'materi', 'list', kelasID, babID ?? 'free'] as const,
    [kelasID, babID],
  );

  const invalidateKeys = React.useMemo(
    () => [queryKey] as const,
    [queryKey],
  );

  const listQuery = useQuery({
    queryKey,
    queryFn: () =>
      listMateri(kelasID, {
        babID: babID ?? null,
      }),
    staleTime: 15_000,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteMateri(id),
    onSuccess: (resp) => {
      queryClient.invalidateQueries({ queryKey });
      const cleanupNote = resp.pending_r2_cleanup
        ? ' File akan dibersihkan oleh sweeper.'
        : '';
      toast({
        title: 'Materi dihapus',
        description: `Materi ${resp.tipe} sudah dihapus.${cleanupNote}`,
      });
      setDeleteTarget(null);
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyMateriError(apiErr, 'delete')
        : 'Gagal menghapus materi.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal menghapus materi',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  async function handleOpenPDF(materi: Materi) {
    try {
      const { url } = await getMateriFileURL(materi.id);
      window.open(url, '_blank', 'noopener');
    } catch (err) {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyMateriError(apiErr, 'file-url')
        : 'Gagal membuat URL PDF.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal membuka PDF',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    }
  }

  const items = listQuery.data?.items ?? [];

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-1">
            <CardTitle className="text-base">Materi</CardTitle>
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
              onClick={() => setCreateOpen(true)}
              disabled={disabled || listQuery.isPending}
              type="button"
            >
              <Plus className="size-4" />
              Tambah materi
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {listQuery.isPending && (
          <div className="space-y-2">
            <div className="h-16 animate-pulse rounded-md border bg-muted/40" />
            <div className="h-16 animate-pulse rounded-md border bg-muted/40" />
          </div>
        )}

        {listQuery.isError && (
          <div className="rounded-md border border-destructive/50 bg-destructive/5 p-3 text-sm text-destructive">
            Gagal memuat daftar materi.{' '}
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
            Belum ada materi. Tambahkan PDF, YouTube, atau markdown untuk
            mulai.
          </div>
        )}

        {listQuery.isSuccess && items.length > 0 && (
          <ul className="space-y-2">
            {items.map((m) => {
              const Icon = tipeIcon(m.tipe);
              const badge = tipeBadge(m.tipe);
              const sizeLabel = m.tipe === 'pdf' ? formatBytes(m.size_bytes) : '';
              return (
                <li
                  key={m.id}
                  className="flex items-start justify-between gap-3 rounded-md border bg-background px-3 py-2.5"
                >
                  <div className="flex min-w-0 items-start gap-3">
                    <div className="mt-0.5">
                      <Icon className="size-5 text-muted-foreground" />
                    </div>
                    <div className="min-w-0 space-y-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <p
                          className="truncate font-medium"
                          title={m.judul}
                        >
                          {m.judul}
                        </p>
                        <span
                          className={cn(
                            'rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
                            badge.class,
                          )}
                        >
                          {badge.label}
                        </span>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        Urutan {m.urutan} · v{m.version}
                        {sizeLabel ? ` · ${sizeLabel}` : ''}
                        {m.tipe === 'youtube' && m.konten
                          ? ` · ID ${m.konten}`
                          : ''}
                      </p>
                    </div>
                  </div>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="size-8 p-0"
                        aria-label={`Aksi materi ${m.judul}`}
                      >
                        <MoreVertical className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-44">
                      {m.tipe === 'pdf' && (
                        <>
                          <DropdownMenuItem
                            onSelect={() => handleOpenPDF(m)}
                          >
                            <ExternalLink className="size-4" />
                            Buka PDF
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                        </>
                      )}
                      <DropdownMenuItem
                        onSelect={() => setEditTarget(m)}
                        disabled={disabled}
                      >
                        Edit
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        className="text-destructive focus:text-destructive"
                        onSelect={() => setDeleteTarget(m)}
                        disabled={disabled}
                      >
                        <Trash2 className="size-4" />
                        Hapus
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </li>
              );
            })}
          </ul>
        )}
      </CardContent>

      <MateriCreateDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        kelasID={kelasID}
        babID={babID}
        invalidateKeys={invalidateKeys}
      />

      {editTarget && (
        <MateriEditDialog
          open={!!editTarget}
          onOpenChange={(open) => {
            if (!open) setEditTarget(null);
          }}
          materi={editTarget}
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
            <DialogTitle>Hapus materi?</DialogTitle>
            <DialogDescription>
              {deleteTarget
                ? `"${deleteTarget.judul}" (${deleteTarget.tipe}) akan dihapus permanen.`
                : ''}
              {deleteTarget?.tipe === 'pdf' &&
                ' File PDF di storage akan ikut dibersihkan oleh sweeper.'}
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
