'use client';

/**
 * Tab "Bab" di kelas detail page guru. Membungkus list query +
 * orchestration semua dialog (create/edit/archive/duplicate) + DnD
 * reorder via @dnd-kit.
 *
 * Dipakai dari `frontend/app/(authed)/guru/kelas/detail/page.tsx` saat
 * `tab === 'bab'`. Saat kelas archived, manajemen bab dimatikan
 * (read-only), tapi list tetap ditampilkan.
 */

import * as React from 'react';
import { useQuery } from '@tanstack/react-query';
import { Plus, RotateCcw } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type Bab,
  type BabListResponse,
  listBab,
} from '@/lib/bab-api';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

import { ArchiveBabDialog } from './ArchiveBabDialog';
import { BabFormDialog } from './BabFormDialog';
import { BabReorderList } from './BabReorderList';
import { DuplicateBabDialog } from './DuplicateBabDialog';

interface BabListSectionProps {
  kelasID: string;
  archived: boolean;
}

export function BabListSection({ kelasID, archived }: BabListSectionProps) {
  const [includeArchived, setIncludeArchived] = React.useState(false);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [editTarget, setEditTarget] = React.useState<Bab | null>(null);
  const [archiveTarget, setArchiveTarget] = React.useState<Bab | null>(null);
  const [duplicateTarget, setDuplicateTarget] = React.useState<Bab | null>(
    null,
  );

  const queryKey = React.useMemo(
    () => ['guru', 'kelas', 'bab', kelasID, includeArchived] as const,
    [kelasID, includeArchived],
  );

  const query = useQuery<BabListResponse>({
    queryKey,
    queryFn: () => listBab(kelasID, { includeArchived }),
    staleTime: 15_000,
  });

  const invalidateKeys = React.useMemo(
    () =>
      [
        ['guru', 'kelas', 'bab', kelasID] as const,
      ] as const,
    [kelasID],
  );

  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

  const nextNomor = React.useMemo(() => {
    if (items.length === 0) return 1;
    return Math.max(...items.map((b) => b.nomor)) + 1;
  }, [items]);

  if (query.isPending) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Bab</CardTitle>
          <CardDescription>Memuat daftar bab…</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {[0, 1, 2].map((i) => (
              <div
                key={i}
                className="h-12 animate-pulse rounded-md border bg-muted/40"
              />
            ))}
          </div>
        </CardContent>
      </Card>
    );
  }

  if (query.isError) {
    const err = query.error;
    const apiErr = err instanceof ApiError ? err : null;
    const isForbidden = apiErr?.code === 'forbidden';
    const requestId = apiErr?.requestId;
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Gagal memuat daftar bab</CardTitle>
          <CardDescription>
            {isForbidden
              ? 'Lu hanya bisa lihat bab di kelas yang lu kelola.'
              : apiErr
                ? apiErr.message
                : 'Terjadi kesalahan tidak terduga.'}
            {requestId ? ` (req: ${requestId})` : ''}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => query.refetch()}
            disabled={query.isFetching}
          >
            <RotateCcw className="size-4" />
            Coba lagi
          </Button>
        </CardContent>
      </Card>
    );
  }

  const showActions = !archived;

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div className="space-y-1.5">
          <CardTitle className="text-base">Bab</CardTitle>
          <CardDescription>
            {total === 0
              ? 'Belum ada bab di kelas ini.'
              : `Total ${total} bab${
                  includeArchived ? ' (termasuk yang diarsipkan)' : ' aktif'
                }. Geser untuk mengurutkan ulang; status diatur lewat menu aksi atau dialog edit.`}
          </CardDescription>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <input
              type="checkbox"
              className="size-3.5 rounded border-input"
              checked={includeArchived}
              onChange={(e) => setIncludeArchived(e.target.checked)}
            />
            Tampilkan yang diarsipkan
          </label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => query.refetch()}
            disabled={query.isFetching}
          >
            <RotateCcw className="size-4" />
            Refresh
          </Button>
          {showActions && (
            <Button
              type="button"
              size="sm"
              onClick={() => setCreateOpen(true)}
            >
              <Plus className="size-4" />
              Tambah bab
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {archived && (
          <p className="rounded-md border border-dashed bg-muted/30 p-3 text-xs text-muted-foreground">
            Kelas ini sudah diarsipkan. Manajemen bab dinonaktifkan; daftar
            ditampilkan read-only.
          </p>
        )}

        {items.length === 0 ? (
          <div className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">
            {archived
              ? 'Belum ada bab saat kelas diarsipkan.'
              : 'Belum ada bab. Tambah bab pertama untuk mulai mengisi materi.'}
          </div>
        ) : (
          <BabReorderList
            items={items}
            kelasID={kelasID}
            queryKey={queryKey}
            disabled={archived}
            onEdit={setEditTarget}
            onDuplicate={setDuplicateTarget}
            onArchive={setArchiveTarget}
          />
        )}
      </CardContent>

      <BabFormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        mode="create"
        kelasID={kelasID}
        defaultNomor={nextNomor}
        invalidateKeys={invalidateKeys}
      />
      <BabFormDialog
        open={!!editTarget}
        onOpenChange={(open) => {
          if (!open) setEditTarget(null);
        }}
        mode="edit"
        kelasID={kelasID}
        bab={editTarget}
        invalidateKeys={invalidateKeys}
      />
      <ArchiveBabDialog
        open={!!archiveTarget}
        onOpenChange={(open) => {
          if (!open) setArchiveTarget(null);
        }}
        bab={archiveTarget}
        invalidateKeys={invalidateKeys}
      />
      <DuplicateBabDialog
        open={!!duplicateTarget}
        onOpenChange={(open) => {
          if (!open) setDuplicateTarget(null);
        }}
        bab={duplicateTarget}
        invalidateKeys={invalidateKeys}
      />
    </Card>
  );
}
