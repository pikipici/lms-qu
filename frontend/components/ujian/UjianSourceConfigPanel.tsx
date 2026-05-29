'use client';

/**
 * UjianSourceConfigPanel — discriminated source selector untuk Ujian
 * (locked #85). Mode `manual` = pilih soal_ids[] explicit dari BankSoal,
 * mode `random` = filter mapel/tingkat/topik + jumlah_soal.
 *
 * Komponen self-contained dengan TanStack Query — fetch BankSoal pribadi
 * caller untuk pool. Manual mode pakai checkbox grid + count terpilih.
 * Random mode pakai dropdown chip dari tag yang ada + input jumlah_soal
 * + Live Preview button (POST /ujian/:id/source/preview) untuk verify
 * pool size sebelum save.
 *
 * Used by UjianFormDialog + standalone UjianSourceTab di edit Ujian.
 *
 * Beda dari SoalBabSettingForm: discriminated source (bukan jumlah_soal
 * saja) + cross-bab pool (BankSoal pribadi) bukan per-bab.
 */

import * as React from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Filter, ListChecks, Loader2, Search, Wand2 } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type BankSoal,
  listBankSoal,
} from '@/lib/banksoal-api';
import {
  type UjianSourceConfig,
  type UjianSourceMode,
  type UjianSourcePreview,
  friendlyUjianError,
  previewUjianSource,
} from '@/lib/ujian-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';

const MODE_OPTIONS: {
  value: UjianSourceMode;
  label: string;
  hint: string;
  Icon: React.ComponentType<{ className?: string }>;
}[] = [
  {
    value: 'manual',
    label: 'Manual',
    hint: 'Pilih soal satu per satu dari Bank Soal',
    Icon: ListChecks,
  },
  {
    value: 'random',
    label: 'Random',
    hint: 'Filter tag + jumlah, sistem acak per siswa',
    Icon: Wand2,
  },
];

export interface UjianSourceConfigPanelProps {
  /**
   * Saat dialog "create": ujianID null → preview disabled (preview butuh
   * existing ujian id untuk auth scoping).
   */
  ujianID: string | null;
  value: UjianSourceConfig | null;
  onChange: (next: UjianSourceConfig) => void;
  disabled?: boolean;
}

export function UjianSourceConfigPanel({
  ujianID,
  value,
  onChange,
  disabled,
}: UjianSourceConfigPanelProps) {
  const { toast } = useToast();
  const mode: UjianSourceMode =
    value?.mode === 'random' ? 'random' : 'manual';

  // Master query — fetch up to 200 soal milik guru, derive tag pools.
  // Cap 200 cukup untuk MVP UX; guru dengan > 200 soal pakai filter tag.
  const masterQuery = useQuery({
    queryKey: ['guru', 'ujian-source', 'bank-master'],
    queryFn: () => listBankSoal({ limit: 200 }),
    staleTime: 30_000,
  });

  const allSoal = React.useMemo(
    () => masterQuery.data?.items ?? [],
    [masterQuery.data?.items],
  );

  const allMapel = React.useMemo(() => {
    const set = new Set<string>();
    for (const s of allSoal) if (s.mapel) set.add(s.mapel);
    return Array.from(set).sort((a, b) => a.localeCompare(b, 'id'));
  }, [allSoal]);
  const allTingkat = React.useMemo(() => {
    const set = new Set<string>();
    for (const s of allSoal) if (s.tingkat) set.add(s.tingkat);
    return Array.from(set).sort((a, b) => a.localeCompare(b, 'id'));
  }, [allSoal]);

  const previewMutation = useMutation({
    mutationFn: (cfg: UjianSourceConfig) => {
      if (!ujianID) throw new Error('preview_requires_ujian_id');
      return previewUjianSource(ujianID, cfg);
    },
    onError: (err) => {
      if (err instanceof Error && err.message === 'preview_requires_ujian_id') {
        toast({
          title: 'Preview butuh ujian tersimpan',
          description:
            'Simpan ujian sebagai draft dulu, baru preview sumber soal bisa dijalanin.',
        });
        return;
      }
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyUjianError(apiErr, 'preview')
        : 'Gagal memuat preview.';
      toast({
        title: 'Preview gagal',
        description: apiErr?.requestId
          ? `${message} (req: ${apiErr.requestId})`
          : message,
        variant: 'destructive',
      });
    },
  });

  const previewData: UjianSourcePreview | null =
    previewMutation.data?.preview ?? null;

  function setMode(next: UjianSourceMode) {
    if (next === 'manual') {
      onChange({
        mode: 'manual',
        soal_ids: value?.mode === 'manual' ? value.soal_ids : [],
      });
    } else {
      onChange({
        mode: 'random',
        filter:
          value?.mode === 'random' ? value.filter ?? {} : {},
        jumlah_soal:
          value?.mode === 'random' ? value.jumlah_soal : 10,
      });
    }
  }

  return (
    <div className="space-y-4">
      <div>
        <Label className="text-sm">Sumber soal</Label>
        <p className="text-xs text-muted-foreground">
          Pilih cara sumber soal di-isi: pilih manual dari Bank Soal atau
          biarkan sistem acak dari filter tag.
        </p>
        <div className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
          {MODE_OPTIONS.map((m) => {
            const active = mode === m.value;
            return (
              <button
                key={m.value}
                type="button"
                onClick={() => setMode(m.value)}
                disabled={disabled}
                className={cn(
                  'flex min-w-0 items-start gap-2 rounded-md border p-3 text-left transition-colors',
                  active
                    ? 'border-primary bg-primary/5'
                    : 'border-border hover:border-foreground/40',
                  disabled && 'pointer-events-none opacity-60',
                )}
              >
                <m.Icon className="mt-0.5 size-4 text-muted-foreground" />
                <div className="min-w-0 space-y-0.5">
                  <div className="text-sm font-medium">{m.label}</div>
                  <div className="break-words text-xs text-muted-foreground">
                    {m.hint}
                  </div>
                </div>
              </button>
            );
          })}
        </div>
      </div>

      {mode === 'manual' ? (
        <ManualSourcePanel
          allSoal={allSoal}
          allMapel={allMapel}
          allTingkat={allTingkat}
          masterLoading={masterQuery.isPending}
          masterError={masterQuery.isError}
          selectedIDs={
            value?.mode === 'manual' ? value.soal_ids : []
          }
          onChangeSelected={(ids) =>
            onChange({ mode: 'manual', soal_ids: ids })
          }
          disabled={disabled}
        />
      ) : (
        <RandomSourcePanel
          allMapel={allMapel}
          allTingkat={allTingkat}
          masterLoading={masterQuery.isPending}
          value={
            value?.mode === 'random'
              ? value
              : { mode: 'random', filter: {}, jumlah_soal: 10 }
          }
          onChange={(cfg) => onChange(cfg)}
          disabled={disabled}
        />
      )}

      <div className="flex flex-col gap-2 rounded-md border bg-muted/30 px-3 py-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0 break-words text-xs text-muted-foreground">
          {previewData ? (
            <>
              Pool: <span className="font-semibold text-foreground">
                {previewData.pool_size}
              </span>{' '}
              soal · jumlah_soal:{' '}
              <span className="font-semibold text-foreground">
                {previewData.jumlah_soal}
              </span>
              {previewData.pool_size === 0 && (
                <span className="ml-1 text-destructive">
                  (kosong — siswa tidak akan dapat soal)
                </span>
              )}
              {previewData.pool_size > 0 &&
                previewData.pool_size < previewData.jumlah_soal && (
                  <span className="ml-1 text-amber-600 dark:text-amber-400">
                    (kurang — siswa hanya dapat {previewData.pool_size} soal)
                  </span>
                )}
            </>
          ) : (
            <>Klik Preview untuk verifikasi pool size sebelum save.</>
          )}
        </div>
        <Button
          size="sm"
          variant="outline"
          type="button"
          className="w-full sm:w-auto"
          onClick={() => {
            if (!value) return;
            previewMutation.mutate(value);
          }}
          disabled={
            disabled ||
            !value ||
            previewMutation.isPending ||
            !ujianID
          }
        >
          {previewMutation.isPending && (
            <Loader2 className="size-3.5 animate-spin" />
          )}
          <Search className="size-3.5" />
          Preview
        </Button>
      </div>
    </div>
  );
}

// ---------- Manual mode ----------

function ManualSourcePanel({
  allSoal,
  allMapel,
  allTingkat,
  masterLoading,
  masterError,
  selectedIDs,
  onChangeSelected,
  disabled,
}: {
  allSoal: BankSoal[];
  allMapel: string[];
  allTingkat: string[];
  masterLoading: boolean;
  masterError: boolean;
  selectedIDs: string[];
  onChangeSelected: (ids: string[]) => void;
  disabled?: boolean;
}) {
  const [mapelFilter, setMapelFilter] = React.useState('');
  const [tingkatFilter, setTingkatFilter] = React.useState('');
  const [topikQuery, setTopikQuery] = React.useState('');

  const selectedSet = React.useMemo(
    () => new Set(selectedIDs),
    [selectedIDs],
  );

  const filtered = React.useMemo(() => {
    return allSoal.filter((s) => {
      if (mapelFilter && s.mapel !== mapelFilter) return false;
      if (tingkatFilter && s.tingkat !== tingkatFilter) return false;
      if (topikQuery) {
        const q = topikQuery.toLowerCase();
        if (!s.topik?.toLowerCase().includes(q)) return false;
      }
      return true;
    });
  }, [allSoal, mapelFilter, tingkatFilter, topikQuery]);

  function toggle(id: string) {
    if (disabled) return;
    if (selectedSet.has(id)) {
      onChangeSelected(selectedIDs.filter((x) => x !== id));
    } else {
      onChangeSelected([...selectedIDs, id]);
    }
  }

  function selectAllVisible() {
    if (disabled) return;
    const next = new Set(selectedIDs);
    for (const s of filtered) next.add(s.id);
    onChangeSelected(Array.from(next));
  }

  function clearAll() {
    if (disabled) return;
    onChangeSelected([]);
  }

  if (masterError) {
    return (
      <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
        Gagal memuat Bank Soal. Refresh halaman untuk coba lagi.
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {/* Filter */}
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,10rem)_auto] lg:items-center">
        <select
          className="h-8 w-full rounded-md border border-input bg-background px-2 text-xs"
          value={mapelFilter}
          onChange={(e) => setMapelFilter(e.target.value)}
          disabled={disabled || masterLoading}
        >
          <option value="">Semua mapel</option>
          {allMapel.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </select>
        <select
          className="h-8 w-full rounded-md border border-input bg-background px-2 text-xs"
          value={tingkatFilter}
          onChange={(e) => setTingkatFilter(e.target.value)}
          disabled={disabled || masterLoading}
        >
          <option value="">Semua tingkat</option>
          {allTingkat.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
        <Input
          placeholder="Cari topik…"
          value={topikQuery}
          onChange={(e) => setTopikQuery(e.target.value)}
          disabled={disabled || masterLoading}
          className="h-8 w-full text-xs"
        />
        <span className="text-xs text-muted-foreground lg:ml-auto lg:whitespace-nowrap">
          {selectedIDs.length} dipilih · {filtered.length} terlihat
        </span>
      </div>

      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        <Button
          size="sm"
          variant="outline"
          type="button"
          onClick={selectAllVisible}
          disabled={disabled || filtered.length === 0}
          className="h-8 w-full text-xs"
        >
          Pilih semua terlihat
        </Button>
        <Button
          size="sm"
          variant="ghost"
          type="button"
          onClick={clearAll}
          disabled={disabled || selectedIDs.length === 0}
          className="h-8 w-full text-xs"
        >
          Kosongkan
        </Button>
      </div>

      {/* List */}
      <div className="max-h-60 overflow-y-auto rounded-md border">
        {masterLoading ? (
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
          </div>
        ) : filtered.length === 0 ? (
          <div className="p-4 text-center text-xs text-muted-foreground">
            {allSoal.length === 0
              ? 'Bank Soal kosong. Tambah soal di halaman Bank Soal dulu.'
              : 'Tidak ada soal sesuai filter. Reset filter.'}
          </div>
        ) : (
          <ul className="divide-y">
            {filtered.map((s) => {
              const checked = selectedSet.has(s.id);
              return (
                <li key={s.id} className="flex min-w-0 items-start gap-2 p-2">
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() => toggle(s.id)}
                    disabled={disabled}
                    className="mt-1 size-4"
                  />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5 break-words text-[10px] uppercase tracking-wide text-muted-foreground">
                      {s.mapel && <span>Mapel: {s.mapel}</span>}
                      {s.tingkat && <span>· Tingkat: {s.tingkat}</span>}
                      {s.topik && <span>· Topik: {s.topik}</span>}
                      <span>· Jawaban: {s.jawaban.toUpperCase()}</span>
                      <span>· Poin: {s.poin}</span>
                    </div>
                    <p className="line-clamp-2 break-words text-xs">
                      {s.pertanyaan.trim() || '(soal hanya gambar)'}
                    </p>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}

// ---------- Random mode ----------

function RandomSourcePanel({
  allMapel,
  allTingkat,
  masterLoading,
  value,
  onChange,
  disabled,
}: {
  allMapel: string[];
  allTingkat: string[];
  masterLoading: boolean;
  value: {
    mode: 'random';
    filter?: { mapel?: string; tingkat?: string; topik?: string };
    jumlah_soal: number;
  };
  onChange: (cfg: UjianSourceConfig) => void;
  disabled?: boolean;
}) {
  const filter = value.filter ?? {};

  function patchFilter(patch: Partial<{ mapel?: string; tingkat?: string; topik?: string }>) {
    onChange({
      mode: 'random',
      filter: { ...filter, ...patch },
      jumlah_soal: value.jumlah_soal,
    });
  }

  function setJumlah(n: number) {
    onChange({
      mode: 'random',
      filter,
      jumlah_soal: Math.max(1, Math.min(200, n || 1)),
    });
  }

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
        <div className="space-y-1">
          <Label className="text-xs">Mapel</Label>
          <select
            className="h-8 w-full rounded-md border border-input bg-background px-2 text-xs"
            value={filter.mapel ?? ''}
            onChange={(e) =>
              patchFilter({ mapel: e.target.value || undefined })
            }
            disabled={disabled || masterLoading}
          >
            <option value="">(semua)</option>
            {allMapel.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <Label className="text-xs">Tingkat</Label>
          <select
            className="h-8 w-full rounded-md border border-input bg-background px-2 text-xs"
            value={filter.tingkat ?? ''}
            onChange={(e) =>
              patchFilter({ tingkat: e.target.value || undefined })
            }
            disabled={disabled || masterLoading}
          >
            <option value="">(semua)</option>
            {allTingkat.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <Label className="text-xs">Topik (substring)</Label>
          <Input
            placeholder="aljabar"
            value={filter.topik ?? ''}
            onChange={(e) =>
              patchFilter({ topik: e.target.value || undefined })
            }
            disabled={disabled}
            className="h-8 text-xs"
          />
        </div>
      </div>
      <div className="space-y-1">
        <Label className="text-xs" htmlFor="jumlahSoal">
          Jumlah soal per attempt
        </Label>
        <Input
          id="jumlahSoal"
          type="number"
          min={1}
          max={200}
          value={value.jumlah_soal}
          onChange={(e) => setJumlah(Number(e.target.value))}
          disabled={disabled}
          className="h-8 w-full text-xs sm:max-w-[10rem]"
        />
        <p className="text-xs text-muted-foreground">
          Sistem akan mengacak {value.jumlah_soal} soal dari pool yang
          cocok dengan filter di atas, deterministik per siswa (locked
          #86 — refresh siswa tidak ganti soal).
        </p>
      </div>
    </div>
  );
}
