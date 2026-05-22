'use client';

/**
 * SoalBabList — list semua soal di bab dengan filter mode + actions
 * (create / edit / delete / bulk paste). Dipakai di tab "Soal" di bab
 * detail page guru.
 *
 * Action map:
 *   - Tombol "Tambah soal" → SoalBabEditDialog mode create
 *   - Tombol "Bulk paste"  → BulkPasteDialog
 *   - Card per soal: dropdown Edit / Hapus
 *   - Filter chip: semua | latihan | ulangan | keduanya
 *
 * Pool counter: tampilin total per mode supaya guru aware sebelum set
 * UlanganBabSetting jumlah_soal.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Eye,
  ImageIcon,
  Loader2,
  MoreVertical,
  Pencil,
  Plus,
  Trash2,
  Upload,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type SoalBab,
  type SoalMode,
  deleteSoal,
  friendlySoalError,
  listSoal,
} from '@/lib/soalbab-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';

import { BulkPasteDialog } from './BulkPasteDialog';
import { SoalBabEditDialog } from './SoalBabEditDialog';
import { SoalPreviewDialog } from './SoalPreviewDialog';

type ModeFilter = 'semua' | SoalMode;

const FILTER_CHIPS: { key: ModeFilter; label: string }[] = [
  { key: 'semua', label: 'Semua' },
  { key: 'latihan', label: 'Latihan' },
  { key: 'ulangan', label: 'Ulangan' },
  { key: 'keduanya', label: 'Keduanya' },
];

export interface SoalBabListProps {
  babID: string;
  contextLabel?: string;
  disabled?: boolean;
}

export function SoalBabList({ babID, contextLabel, disabled }: SoalBabListProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [filter, setFilter] = React.useState<ModeFilter>('semua');
  const [editing, setEditing] = React.useState<SoalBab | null>(null);
  const [previewing, setPreviewing] = React.useState<SoalBab | null>(null);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [bulkOpen, setBulkOpen] = React.useState(false);

  const queryKey = React.useMemo(
    () => ['guru', 'bab', babID, 'soal'] as const,
    [babID],
  );
  const invalidateKeys = React.useMemo(
    () => [queryKey] as const,
    [queryKey],
  );

  const listQuery = useQuery({
    queryKey,
    queryFn: () => listSoal(babID),
    staleTime: 10_000,
  });

  const items = React.useMemo(() => {
    const all = listQuery.data?.items ?? [];
    if (filter === 'semua') return all;
    return all.filter((s) => s.mode === filter);
  }, [listQuery.data, filter]);

  const counts = React.useMemo(() => {
    const all = listQuery.data?.items ?? [];
    return {
      total: all.length,
      latihan: all.filter((s) => s.mode === 'latihan' || s.mode === 'keduanya').length,
      ulangan: all.filter((s) => s.mode === 'ulangan' || s.mode === 'keduanya').length,
    };
  }, [listQuery.data]);

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteSoal(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
      toast({ title: 'Soal dihapus' });
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr ? friendlySoalError(apiErr, 'delete') : 'Gagal menghapus soal.';
      toast({
        title: 'Gagal menghapus soal',
        description: apiErr?.requestId ? `${message} (req: ${apiErr.requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <CardTitle className="text-base">Soal</CardTitle>
            <CardDescription>
              {contextLabel ?? 'Bank soal per bab untuk latihan dan ulangan.'}
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              type="button"
              onClick={() => setBulkOpen(true)}
              disabled={disabled}
            >
              <Upload className="size-4" />
              Bulk paste
            </Button>
            <Button
              size="sm"
              type="button"
              onClick={() => setCreateOpen(true)}
              disabled={disabled}
            >
              <Plus className="size-4" />
              Tambah soal
            </Button>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2 pt-2">
          {FILTER_CHIPS.map((chip) => {
            const active = filter === chip.key;
            const c =
              chip.key === 'semua'
                ? counts.total
                : chip.key === 'latihan'
                  ? counts.latihan
                  : chip.key === 'ulangan'
                    ? counts.ulangan
                    : (listQuery.data?.items ?? []).filter((s) => s.mode === 'keduanya').length;
            return (
              <button
                key={chip.key}
                type="button"
                onClick={() => setFilter(chip.key)}
                className={cn(
                  'rounded-full border px-3 py-1 text-xs transition-colors',
                  active
                    ? 'border-primary bg-primary/10 font-medium text-foreground'
                    : 'border-border text-muted-foreground hover:border-foreground/40 hover:text-foreground',
                )}
              >
                {chip.label} <span className="ml-1 opacity-70">({c})</span>
              </button>
            );
          })}
        </div>
      </CardHeader>

      <CardContent>
        {listQuery.isPending ? (
          <div className="flex items-center justify-center py-12 text-muted-foreground">
            <Loader2 className="size-5 animate-spin" />
          </div>
        ) : listQuery.isError ? (
          <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
            Gagal memuat daftar soal. Refresh halaman atau coba lagi.
          </div>
        ) : items.length === 0 ? (
          <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
            {counts.total === 0
              ? 'Belum ada soal. Mulai dengan tombol Tambah soal atau Bulk paste.'
              : `Tidak ada soal di filter "${filter}". Coba filter lain.`}
          </div>
        ) : (
          <ul className="space-y-2">
            {items.map((soal) => (
              <SoalRow
                key={soal.id}
                soal={soal}
                onEdit={() => setEditing(soal)}
                onPreview={() => setPreviewing(soal)}
                onDelete={() => {
                  if (confirm(`Hapus soal "${soal.pertanyaan.slice(0, 50) || '(gambar)'}"?`)) {
                    deleteMutation.mutate(soal.id);
                  }
                }}
                disabled={disabled || deleteMutation.isPending}
              />
            ))}
          </ul>
        )}
      </CardContent>

      <SoalBabEditDialog
        key={`create-${createOpen}`}
        open={createOpen}
        onOpenChange={setCreateOpen}
        babID={babID}
        soal={null}
        invalidateKeys={invalidateKeys}
      />
      {editing && (
        <SoalBabEditDialog
          key={`edit-${editing.id}-${editing.version}`}
          open={!!editing}
          onOpenChange={(o) => {
            if (!o) setEditing(null);
          }}
          babID={babID}
          soal={editing}
          invalidateKeys={invalidateKeys}
        />
      )}
      <BulkPasteDialog
        open={bulkOpen}
        onOpenChange={setBulkOpen}
        babID={babID}
        invalidateKeys={invalidateKeys}
      />
      <SoalPreviewDialog
        open={!!previewing}
        onOpenChange={(o) => {
          if (!o) setPreviewing(null);
        }}
        soal={previewing}
      />
    </Card>
  );
}

function modeBadgeClass(mode: SoalMode): string {
  switch (mode) {
    case 'latihan':
      return 'border-blue-300 bg-blue-50 text-blue-700 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300';
    case 'ulangan':
      return 'border-amber-300 bg-amber-50 text-amber-700 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-300';
    case 'keduanya':
      return 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-300';
  }
}

function SoalRow({
  soal,
  onEdit,
  onPreview,
  onDelete,
  disabled,
}: {
  soal: SoalBab;
  onEdit: () => void;
  onPreview: () => void;
  onDelete: () => void;
  disabled?: boolean;
}) {
  const hasImage =
    !!soal.pertanyaan_object_key ||
    !!soal.opsi_a_object_key ||
    !!soal.opsi_b_object_key ||
    !!soal.opsi_c_object_key ||
    !!soal.opsi_d_object_key ||
    !!soal.opsi_e_object_key;
  const preview = soal.pertanyaan.trim() || '(soal hanya gambar)';

  return (
    <li className="rounded-md border bg-background p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span
              className={cn(
                'rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
                modeBadgeClass(soal.mode),
              )}
            >
              {soal.mode}
            </span>
            <span className="text-xs text-muted-foreground">
              Jawaban: <span className="font-semibold uppercase">{soal.jawaban}</span> · Poin: {soal.poin} · v{soal.version}
            </span>
            {hasImage && (
              <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                <ImageIcon className="size-3" />
                Ada gambar
              </span>
            )}
          </div>
          <p className="mt-1 line-clamp-2 text-sm">{preview}</p>
          <ul className="mt-1.5 grid grid-cols-1 gap-1 sm:grid-cols-2 text-xs text-muted-foreground">
            {(['a', 'b', 'c', 'd', 'e'] as const).map((letter) => {
              const text = soal[`opsi_${letter}` as `opsi_${typeof letter}`];
              const correct = soal.jawaban === letter;
              const truncated = text.length > 60 ? `${text.slice(0, 60)}…` : text;
              return (
                <li
                  key={letter}
                  className={cn(
                    'rounded border px-2 py-1',
                    correct ? 'border-emerald-300 bg-emerald-50/50 dark:border-emerald-800 dark:bg-emerald-950/30' : 'border-border',
                  )}
                >
                  <span className="font-mono uppercase">{letter}.</span>{' '}
                  {truncated || <span className="italic">(kosong)</span>}
                </li>
              );
            })}
          </ul>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="sm" variant="ghost" type="button" disabled={disabled}>
              <MoreVertical className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={onPreview}>
              <Eye className="size-4" />
              Preview
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onEdit}>
              <Pencil className="size-4" />
              Edit
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={onDelete}
              className="text-destructive focus:text-destructive"
            >
              <Trash2 className="size-4" />
              Hapus
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </li>
  );
}
