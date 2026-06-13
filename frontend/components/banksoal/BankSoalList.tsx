'use client';

/**
 * BankSoalList — list semua soal Bank Soal pribadi guru dengan filter
 * tag (mapel/tingkat/topik) + pagination + actions (create/edit/delete/
 * bulk paste). Mirror SoalBabList pattern (Task 6.F.1).
 *
 * Action map:
 *   - Tombol "Tambah soal" → BankSoalEditDialog mode create (default tag
 *     dari filter aktif)
 *   - Tombol "Bulk paste"   → BankSoalBulkPasteDialog (default tag dari
 *     filter aktif)
 *   - Card per soal: dropdown Edit / Hapus
 *   - Filter chips: mapel + tingkat (derived dari listing) + topik input
 *
 * Pagination: server limit/offset (cap 200 default 50). FE expose Prev/Next
 * supaya guru bisa browse bank besar.
 *
 * Locked decisions referenced:
 *   - #84 BankSoal scope per-guru pribadi
 *   - #56 optimistic concurrency (delete kirim version)
 *   - #69 soft-delete: hapus aman walau soal sudah dipakai HasilUjian
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  ImageIcon,
  Loader2,
  MoreVertical,
  Pencil,
  Plus,
  Search,
  Trash2,
  Upload,
  X,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type BankSoal,
  deleteBankSoal,
  friendlyBankSoalError,
  listBankSoal,
} from '@/lib/banksoal-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';

import { BankSoalBulkPasteDialog } from './BankSoalBulkPasteDialog';
import { BankSoalEditDialog } from './BankSoalEditDialog';

const PAGE_SIZE = 25;

export interface BankSoalListProps {
  disabled?: boolean;
}

export function BankSoalList({ disabled }: BankSoalListProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  // Filter state
  const [mapelFilter, setMapelFilter] = React.useState<string>('');
  const [tingkatFilter, setTingkatFilter] = React.useState<string>('');
  const [topikFilter, setTopikFilter] = React.useState<string>('');
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [topikInput, setTopikInput] = React.useState<string>('');
  const [page, setPage] = React.useState(0);

  // Reset page tiap filter berubah.
  React.useEffect(() => {
    setPage(0);
  }, [mapelFilter, tingkatFilter, topikFilter, tagFilter]);

  const offset = page * PAGE_SIZE;

  // Dialog state
  const [editing, setEditing] = React.useState<BankSoal | null>(null);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [bulkOpen, setBulkOpen] = React.useState(false);

  // Master query (tanpa filter) — dipake buat derive list mapel/tingkat
  // untuk filter chip. Cap 200 supaya cukup untuk UX, kalau guru punya
  // > 200 soal mereka harus pake topik search.
  const masterQueryKey = React.useMemo(
    () => ['guru', 'bank-soal', 'master'] as const,
    [],
  );
  const masterQuery = useQuery({
    queryKey: masterQueryKey,
    queryFn: () => listBankSoal({ limit: 200 }),
    staleTime: 30_000,
  });

  const allMapel = React.useMemo(() => {
    const set = new Set<string>();
    for (const s of masterQuery.data?.items ?? []) {
      if (s.mapel) set.add(s.mapel);
    }
    return Array.from(set).sort((a, b) => a.localeCompare(b, 'id'));
  }, [masterQuery.data]);

  const allTingkat = React.useMemo(() => {
    const set = new Set<string>();
    for (const s of masterQuery.data?.items ?? []) {
      if (s.tingkat) set.add(s.tingkat);
    }
    return Array.from(set).sort((a, b) => a.localeCompare(b, 'id'));
  }, [masterQuery.data]);

  const allTopik = React.useMemo(() => {
    const set = new Set<string>();
    for (const s of masterQuery.data?.items ?? []) {
      if (s.topik) set.add(s.topik);
    }
    return Array.from(set).sort((a, b) => a.localeCompare(b, 'id'));
  }, [masterQuery.data]);

  const allTags = React.useMemo(() => {
    const set = new Set<string>();
    for (const s of masterQuery.data?.items ?? []) {
      for (const tag of s.tags ?? []) set.add(tag);
    }
    return Array.from(set).sort((a, b) => a.localeCompare(b, 'id'));
  }, [masterQuery.data]);

  // Filtered query — driver for the visible list.
  const queryKey = React.useMemo(
    () =>
      [
        'guru',
        'bank-soal',
        'list',
        { mapel: mapelFilter, tingkat: tingkatFilter, topik: topikFilter, tags: tagFilter.join(','), offset },
      ] as const,
    [mapelFilter, tingkatFilter, topikFilter, tagFilter, offset],
  );

  const invalidateKeys = React.useMemo(
    () =>
      [
        ['guru', 'bank-soal', 'list'] as const,
        masterQueryKey,
      ] as const,
    [masterQueryKey],
  );

  const listQuery = useQuery({
    queryKey,
    queryFn: () =>
      listBankSoal({
        mapel: mapelFilter || undefined,
        tingkat: tingkatFilter || undefined,
        topik: topikFilter || undefined,
        tags: tagFilter.length > 0 ? tagFilter : undefined,
        limit: PAGE_SIZE,
        offset,
      }),
    staleTime: 10_000,
  });

  const items = listQuery.data?.items ?? [];
  const total = listQuery.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const hasPrev = page > 0;
  const hasNext = (page + 1) * PAGE_SIZE < total;

  const deleteMutation = useMutation({
    mutationFn: (s: BankSoal) => deleteBankSoal(s.id, s.version),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['guru', 'bank-soal'] });
      toast({ title: 'Soal dihapus' });
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyBankSoalError(apiErr, 'delete')
        : 'Gagal menghapus soal.';
      toast({
        title: 'Gagal menghapus soal',
        description: apiErr?.requestId
          ? `${message} (req: ${apiErr.requestId})`
          : message,
        variant: 'destructive',
      });
    },
  });

  const hasFilter =
    !!mapelFilter || !!tingkatFilter || !!topikFilter || tagFilter.length > 0;
  const activeTags = [
    mapelFilter ? { label: 'Mapel', value: mapelFilter } : null,
    tingkatFilter ? { label: 'Tingkat', value: tingkatFilter } : null,
    topikFilter ? { label: 'Topik', value: topikFilter } : null,
    ...tagFilter.map((t) => ({ label: 'Tag', value: t })),
  ].filter(Boolean) as { label: string; value: string }[];

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <CardTitle className="text-base">Bank Soal</CardTitle>
            <CardDescription>
              Bank soal pribadi kamu (per-guru). Pakai untuk menyusun Ulangan
              Harian dengan mode manual atau random.
            </CardDescription>
          </div>
          <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto sm:justify-end">
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

        {/* Filter row */}
        <div className="space-y-2 pt-3">
          <FilterChipRow
            label="Mapel"
            options={allMapel}
            active={mapelFilter}
            onChange={setMapelFilter}
            emptyHint="(belum ada tag mapel)"
          />
          <FilterChipRow
            label="Tingkat"
            options={allTingkat}
            active={tingkatFilter}
            onChange={setTingkatFilter}
            emptyHint="(belum ada tag tingkat)"
          />
          <FilterChipRow
            label="Topik"
            options={allTopik.slice(0, 24)}
            active={allTopik.includes(topikFilter) ? topikFilter : ''}
            onChange={(value) => {
              setTopikFilter(value);
              setTopikInput(value);
            }}
            emptyHint="(belum ada tag topik)"
          />
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs font-medium text-muted-foreground w-16 shrink-0">
              Tags
            </span>
            {allTags.length === 0 ? (
              <span className="text-xs text-muted-foreground italic">
                (belum ada tags bebas)
              </span>
            ) : (
              allTags.slice(0, 32).map((opt) => {
                const active = tagFilter.includes(opt);
                return (
                  <button
                    key={opt}
                    type="button"
                    onClick={() =>
                      setTagFilter((prev) =>
                        active ? prev.filter((t) => t !== opt) : [...prev, opt],
                      )
                    }
                    className={cn(
                      'rounded-full border px-3 py-1 text-xs transition-colors',
                      active
                        ? 'border-primary bg-primary/10 font-medium text-foreground'
                        : 'border-border text-muted-foreground hover:border-foreground/40 hover:text-foreground',
                    )}
                  >
                    {opt}
                  </button>
                );
              })
            )}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs font-medium text-muted-foreground w-16 shrink-0">
              Cari
            </span>
            <div className="relative min-w-0 flex-1 sm:min-w-[14rem] sm:max-w-md">
              <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={topikInput}
                onChange={(e) => setTopikInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    setTopikFilter(topikInput.trim());
                  }
                }}
                onBlur={() => setTopikFilter(topikInput.trim())}
                placeholder="Cari topik bebas (mis. aljabar)"
                className="h-8 pl-7 pr-8 text-xs"
              />
              {topikInput && (
                <button
                  type="button"
                  onClick={() => {
                    setTopikInput('');
                    setTopikFilter('');
                  }}
                  className="absolute right-1.5 top-1/2 -translate-y-1/2 rounded p-0.5 text-muted-foreground hover:text-foreground"
                  aria-label="Clear topik"
                >
                  <X className="size-3.5" />
                </button>
              )}
            </div>
            {activeTags.length > 0 && (
              <div className="flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
                Aktif:
                {activeTags.map((tag) => (
                  <span
                    key={`${tag.label}-${tag.value}`}
                    className="rounded-full border bg-primary/5 px-2 py-0.5 font-medium text-foreground"
                  >
                    {tag.label}: {tag.value}
                  </span>
                ))}
              </div>
            )}
            {hasFilter && (
              <Button
                size="sm"
                variant="ghost"
                type="button"
                onClick={() => {
                  setMapelFilter('');
                  setTingkatFilter('');
                  setTopikFilter('');
                  setTagFilter([]);
                  setTopikInput('');
                }}
                className="h-7 text-xs"
              >
                Reset filter
              </Button>
            )}
          </div>
        </div>
      </CardHeader>

      <CardContent>
        {listQuery.isPending ? (
          <div className="flex items-center justify-center py-12 text-muted-foreground">
            <Loader2 className="size-5 animate-spin" />
          </div>
        ) : listQuery.isError ? (
          <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
            Gagal memuat Bank Soal. Refresh halaman atau coba lagi.
          </div>
        ) : items.length === 0 ? (
          <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
            {total === 0 && !hasFilter
              ? 'Belum ada soal. Mulai dengan tombol Tambah soal atau Bulk paste.'
              : 'Tidak ada soal sesuai filter. Coba reset filter.'}
          </div>
        ) : (
          <ul className="space-y-2">
            {items.map((soal) => (
              <SoalRow
                key={soal.id}
                soal={soal}
                onEdit={() => setEditing(soal)}
                onDelete={() => {
                  if (
                    confirm(
                      `Hapus soal "${
                        soal.pertanyaan.slice(0, 50) || '(gambar)'
                      }"?`,
                    )
                  ) {
                    deleteMutation.mutate(soal);
                  }
                }}
                disabled={disabled || deleteMutation.isPending}
              />
            ))}
          </ul>
        )}

        {/* Pagination footer */}
        {total > 0 && (
          <div className="mt-4 flex flex-col gap-2 border-t pt-3 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
            <span>
              Menampilkan {offset + 1}-{Math.min(offset + PAGE_SIZE, total)} dari{' '}
              {total} soal
            </span>
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="outline"
                type="button"
                onClick={() => setPage((p) => Math.max(0, p - 1))}
                disabled={!hasPrev || listQuery.isFetching}
                className="h-7 text-xs"
              >
                Prev
              </Button>
              <span>
                Hal {page + 1} / {totalPages}
              </span>
              <Button
                size="sm"
                variant="outline"
                type="button"
                onClick={() => setPage((p) => p + 1)}
                disabled={!hasNext || listQuery.isFetching}
                className="h-7 text-xs"
              >
                Next
              </Button>
            </div>
          </div>
        )}
      </CardContent>

      <BankSoalEditDialog
        key={`create-${createOpen}`}
        open={createOpen}
        onOpenChange={setCreateOpen}
        soal={null}
        defaultMapel={mapelFilter || undefined}
        defaultTingkat={tingkatFilter || undefined}
        defaultTopik={topikFilter || undefined}
        defaultTags={tagFilter.length > 0 ? tagFilter : undefined}
        invalidateKeys={invalidateKeys}
      />
      {editing && (
        <BankSoalEditDialog
          key={`edit-${editing.id}-${editing.version}`}
          open={!!editing}
          onOpenChange={(o) => {
            if (!o) setEditing(null);
          }}
          soal={editing}
          invalidateKeys={invalidateKeys}
        />
      )}
      <BankSoalBulkPasteDialog
        open={bulkOpen}
        onOpenChange={setBulkOpen}
        defaultMapel={mapelFilter || undefined}
        defaultTingkat={tingkatFilter || undefined}
        defaultTopik={topikFilter || undefined}
        defaultTags={tagFilter.length > 0 ? tagFilter : undefined}
        invalidateKeys={invalidateKeys}
      />
    </Card>
  );
}

// ---------- Subcomponents ----------

function FilterChipRow({
  label,
  options,
  active,
  onChange,
  emptyHint,
}: {
  label: string;
  options: string[];
  active: string;
  onChange: (v: string) => void;
  emptyHint: string;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-xs font-medium text-muted-foreground w-16 shrink-0">
        {label}
      </span>
      {options.length === 0 ? (
        <span className="text-xs text-muted-foreground italic">
          {emptyHint}
        </span>
      ) : (
        <>
          <Chip
            label="Semua"
            active={active === ''}
            onClick={() => onChange('')}
          />
          {options.map((opt) => (
            <Chip
              key={opt}
              label={opt}
              active={active === opt}
              onClick={() => onChange(opt)}
            />
          ))}
        </>
      )}
    </div>
  );
}

function Chip({
  label,
  active,
  onClick,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'rounded-full border px-3 py-1 text-xs transition-colors',
        active
          ? 'border-primary bg-primary/10 font-medium text-foreground'
          : 'border-border text-muted-foreground hover:border-foreground/40 hover:text-foreground',
      )}
    >
      {label}
    </button>
  );
}

function SoalRow({
  soal,
  onEdit,
  onDelete,
  disabled,
}: {
  soal: BankSoal;
  onEdit: () => void;
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

  const tagBadges: { label: string; value: string }[] = [];
  if (soal.mapel) tagBadges.push({ label: 'Mapel', value: soal.mapel });
  if (soal.tingkat) tagBadges.push({ label: 'Tingkat', value: soal.tingkat });
  if (soal.topik) tagBadges.push({ label: 'Topik', value: soal.topik });
  for (const tag of soal.tags ?? []) tagBadges.push({ label: 'Tag', value: tag });

  return (
    <li className="rounded-md border bg-background p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            {tagBadges.length > 0 ? (
              tagBadges.map((t) => (
                <span
                  key={`${t.label}-${t.value}`}
                  className="rounded-full border bg-muted/40 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground"
                >
                  {t.label}: {t.value}
                </span>
              ))
            ) : (
              <span className="text-[10px] uppercase tracking-wide text-muted-foreground/70 italic">
                (no tag)
              </span>
            )}
            <span className="text-xs text-muted-foreground">
              Jawaban:{' '}
              <span className="font-semibold uppercase">{soal.jawaban}</span> ·
              Poin: {soal.poin} · v{soal.version}
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
              const truncated =
                text.length > 60 ? `${text.slice(0, 60)}…` : text;
              return (
                <li
                  key={letter}
                  className={cn(
                    'rounded border px-2 py-1',
                    correct
                      ? 'border-emerald-300 bg-emerald-50/50 dark:border-emerald-800 dark:bg-emerald-950/30'
                      : 'border-border',
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
