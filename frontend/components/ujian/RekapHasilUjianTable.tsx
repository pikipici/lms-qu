'use client';

/**
 * RekapHasilUjianTable — per-siswa aggregate dashboard untuk satu Ujian
 * (Task 6.F.2). Mirror RekapHasilTable Fase 5 SoalBab tapi adapted untuk
 * Ujian: pakai `nilai_terbaik`/`nilai_terakhir` numeric, status_terakhir
 * + cancelled_count badge, tombol Reset attempt → CancelUjianAttemptDialog.
 *
 * Sort backend: nilai_terbaik DESC nulls-last + name ASC tiebreak (locked
 * sort di service Rekap).
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Clock, Loader2, RotateCcw, Trash2 } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type SiswaRekap,
  type UjianSusulan,
  cancelUjianHasil,
  createUjianSusulan,
  deleteUjianSusulan,
  friendlyUjianError,
  getRekapHasilUjian,
  listUjianSusulan,
} from '@/lib/ujian-api';
import { type EnrollmentItem, listKelasEnrollments } from '@/lib/kelas-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';

import { CancelUjianAttemptDialog } from './CancelUjianAttemptDialog';

export interface RekapHasilUjianTableProps {
  ujianID: string;
  kelasID: string;
  /** Disable cancel actions kalau ujian/kelas archived. */
  disabled?: boolean;
}

export function RekapHasilUjianTable({
  ujianID,
  kelasID,
  disabled,
}: RekapHasilUjianTableProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const queryKey = React.useMemo(
    () => ['guru', 'ujian', ujianID, 'rekap'] as const,
    [ujianID],
  );

  const rekapQuery = useQuery({
    queryKey,
    queryFn: () => getRekapHasilUjian(ujianID),
    staleTime: 10_000,
  });

  const [cancelTarget, setCancelTarget] = React.useState<SiswaRekap | null>(
    null,
  );
  const [susulanSiswaID, setSusulanSiswaID] = React.useState('');
  const [susulanMulai, setSusulanMulai] = React.useState('');
  const [susulanSelesai, setSusulanSelesai] = React.useState('');
  const [susulanDurasi, setSusulanDurasi] = React.useState('');
  const [susulanReason, setSusulanReason] = React.useState('');
  const [search, setSearch] = React.useState('');
  const [rombelFilter, setRombelFilter] = React.useState('__all__');

  const enrollmentQuery = useQuery({
    queryKey: ['guru', 'kelas', kelasID, 'enrollments', 'ujian-susulan'],
    queryFn: () => listKelasEnrollments(kelasID, { page: 1, pageSize: 200 }),
    staleTime: 30_000,
  });

  const susulanQuery = useQuery({
    queryKey: ['guru', 'ujian', ujianID, 'susulan'],
    queryFn: () => listUjianSusulan(ujianID),
    staleTime: 10_000,
  });

  const cancelMutation = useMutation({
    mutationFn: (hasilID: string) => cancelUjianHasil(hasilID),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
      toast({
        title: 'Attempt dibatalkan',
        description: 'Siswa boleh start fresh. Slot attempt direset.',
      });
      setCancelTarget(null);
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyUjianError(apiErr, 'cancel')
        : 'Gagal membatalkan attempt.';
      toast({
        title: 'Gagal membatalkan',
        description: apiErr?.requestId
          ? `${message} (req: ${apiErr.requestId})`
          : message,
        variant: 'destructive',
      });
    },
  });

  const createSusulanMutation = useMutation({
    mutationFn: () => {
      if (!susulanSiswaID || !susulanSelesai) {
        throw new Error('Pilih siswa dan waktu selesai susulan.');
      }
      const durasi = susulanDurasi ? Number(susulanDurasi) : undefined;
      return createUjianSusulan(ujianID, {
        siswa_id: susulanSiswaID,
        waktu_mulai: susulanMulai ? new Date(susulanMulai).toISOString() : null,
        waktu_selesai: new Date(susulanSelesai).toISOString(),
        durasi_menit: durasi,
        max_attempt: 1,
        reason: susulanReason,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['guru', 'ujian', ujianID, 'susulan'] });
      toast({ title: 'Susulan dibuka', description: 'Siswa terpilih bisa start di window susulan.' });
      setSusulanReason('');
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr ? friendlyUjianError(apiErr, 'susulan') : err instanceof Error ? err.message : 'Gagal menyimpan susulan.';
      toast({ title: 'Gagal buat susulan', description: message, variant: 'destructive' });
    },
  });

  const deleteSusulanMutation = useMutation({
    mutationFn: (siswaID: string) => deleteUjianSusulan(ujianID, siswaID),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['guru', 'ujian', ujianID, 'susulan'] });
      toast({ title: 'Susulan dicabut' });
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr ? friendlyUjianError(apiErr, 'susulan') : 'Gagal mencabut susulan.';
      toast({ title: 'Gagal cabut susulan', description: message, variant: 'destructive' });
    },
  });

  if (rekapQuery.isPending) {
    return (
      <div className="flex items-center justify-center py-8 text-muted-foreground">
        <Loader2 className="size-5 animate-spin" />
      </div>
    );
  }
  if (rekapQuery.isError) {
    const err = rekapQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    return (
      <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
        Gagal memuat rekap.{' '}
        {apiErr ? friendlyUjianError(apiErr, 'rekap') : 'Coba refresh.'}
      </div>
    );
  }

  const rekap = rekapQuery.data!.rekap;
  const items = rekap.items;
  const enrollments = enrollmentQuery.data?.items ?? [];
  const susulanItems = susulanQuery.data?.items ?? [];
  const normalizedSearch = search.trim().toLowerCase();
  const rombelOptions = Array.from(
    new Map(
      items
        .filter((item) => item.rombel_id && item.rombel_nama)
        .map((item) => [
          item.rombel_id!,
          { id: item.rombel_id!, nama: item.rombel_nama! },
        ]),
    ).values(),
  ).sort((a, b) => a.nama.localeCompare(b.nama, 'id-ID'));
  const filteredItems = items.filter((item) => {
    const matchesRombel =
      rombelFilter === '__all__'
        ? true
        : rombelFilter === '__none__'
          ? !item.rombel_id
          : item.rombel_id === rombelFilter;
    const haystack = `${item.siswa_name} ${item.siswa_email}`.toLowerCase();
    const matchesSearch = normalizedSearch ? haystack.includes(normalizedSearch) : true;
    return matchesRombel && matchesSearch;
  });
  const hasFilter = normalizedSearch.length > 0 || rombelFilter !== '__all__';

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
        <span>
          Total siswa attempt:{' '}
          <span className="font-semibold text-foreground">{rekap.total}</span>
        </span>
        <span>
          Rata-rata:{' '}
          <span className="font-semibold text-foreground">
            {rekap.rata_rata != null
              ? rekap.rata_rata.toFixed(2)
              : '—'}
          </span>
        </span>
        {hasFilter && (
          <span>
            Ditampilkan:{' '}
            <span className="font-semibold text-foreground">{filteredItems.length}</span>
          </span>
        )}
      </div>

      <section className="rounded-md border border-amber-200 bg-amber-50/60 p-3 text-sm dark:border-amber-900 dark:bg-amber-950/30">
        <div className="mb-3 flex items-center gap-2 font-medium text-amber-900 dark:text-amber-200">
          <Clock className="size-4" />
          Susulan per siswa
        </div>
        <div className="grid gap-3 md:grid-cols-5">
          <label className="space-y-1 md:col-span-2">
            <Label>Siswa</Label>
            <select
              value={susulanSiswaID}
              onChange={(e) => setSusulanSiswaID(e.target.value)}
              disabled={disabled || enrollmentQuery.isPending}
              className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
            >
              <option value="">Pilih siswa...</option>
              {enrollments.map((enrollment) => (
                <option key={enrollment.siswa_id} value={enrollment.siswa_id}>
                  {enrollment.nama || enrollment.email || enrollment.siswa_id}
                </option>
              ))}
            </select>
          </label>
          <label className="space-y-1">
            <Label>Mulai</Label>
            <Input
              type="datetime-local"
              value={susulanMulai}
              onChange={(e) => setSusulanMulai(e.target.value)}
              disabled={disabled}
            />
          </label>
          <label className="space-y-1">
            <Label>Selesai</Label>
            <Input
              type="datetime-local"
              value={susulanSelesai}
              onChange={(e) => setSusulanSelesai(e.target.value)}
              disabled={disabled}
            />
          </label>
          <label className="space-y-1">
            <Label>Durasi menit</Label>
            <Input
              type="number"
              min={1}
              value={susulanDurasi}
              onChange={(e) => setSusulanDurasi(e.target.value)}
              placeholder="default"
              disabled={disabled}
            />
          </label>
        </div>
        <div className="mt-3 flex flex-col gap-2 md:flex-row">
          <Input
            value={susulanReason}
            onChange={(e) => setSusulanReason(e.target.value)}
            placeholder="Catatan alasan susulan (opsional)"
            disabled={disabled}
          />
          <Button
            type="button"
            onClick={() => createSusulanMutation.mutate()}
            disabled={disabled || createSusulanMutation.isPending}
          >
            {createSusulanMutation.isPending && <Loader2 className="size-4 animate-spin" />}
            Buka susulan
          </Button>
        </div>
        {susulanItems.length > 0 && (
          <div className="mt-3 space-y-2">
            {susulanItems.map((item) => (
              <SusulanPill
                key={item.id}
                item={item}
                siswa={enrollments.find((e) => e.siswa_id === item.siswa_id)}
                disabled={disabled || deleteSusulanMutation.isPending}
                onDelete={() => deleteSusulanMutation.mutate(item.siswa_id)}
              />
            ))}
          </div>
        )}
      </section>

      {items.length === 0 ? (
        <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
          Belum ada siswa yang attempt ujian ini.
        </div>
      ) : (
        <div className="space-y-3">
          <div className="grid gap-2 rounded-md border bg-muted/20 p-3 md:grid-cols-[minmax(0,1fr)_220px_auto] md:items-end">
            <label className="space-y-1">
              <Label>Cari siswa</Label>
              <Input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="Nama atau email siswa"
              />
            </label>
            <label className="space-y-1">
              <Label>Rombel</Label>
              <select
                value={rombelFilter}
                onChange={(event) => setRombelFilter(event.target.value)}
                className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
              >
                <option value="__all__">Semua rombel</option>
                {rombelOptions.map((rombel) => (
                  <option key={rombel.id} value={rombel.id}>
                    {rombel.nama}
                  </option>
                ))}
                <option value="__none__">Tanpa rombel</option>
              </select>
            </label>
            {hasFilter && (
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setSearch('');
                  setRombelFilter('__all__');
                }}
              >
                Reset filter
              </Button>
            )}
          </div>

          {filteredItems.length === 0 ? (
            <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
              Tidak ada hasil yang cocok dengan filter.
            </div>
          ) : (
            <div className="overflow-x-auto rounded-md border">
              <table className="w-full min-w-[860px] text-sm">
                <thead className="bg-muted/40 text-xs">
                  <tr>
                    <th className="px-3 py-2 text-left">Siswa</th>
                    <th className="px-3 py-2 text-left">Rombel</th>
                    <th className="px-3 py-2 text-right">Attempt</th>
                    <th className="px-3 py-2 text-right">Cancel</th>
                    <th className="px-3 py-2 text-right">Terbaik</th>
                    <th className="px-3 py-2 text-right">Terakhir</th>
                    <th className="px-3 py-2 text-left">Status terakhir</th>
                    <th className="px-3 py-2 text-right" />
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {filteredItems.map((s) => (
                    <RekapRow
                      key={s.siswa_id}
                      siswa={s}
                      onCancel={() => setCancelTarget(s)}
                      disabled={disabled || cancelMutation.isPending}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      <CancelUjianAttemptDialog
        open={!!cancelTarget}
        onOpenChange={(o) => {
          if (!o) setCancelTarget(null);
        }}
        siswa={cancelTarget}
        onConfirm={(hasilID) => cancelMutation.mutate(hasilID)}
        pending={cancelMutation.isPending}
      />
    </div>
  );
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString('id-ID', {
    dateStyle: 'medium',
    timeStyle: 'short',
  });
}

function SusulanPill({
  item,
  siswa,
  disabled,
  onDelete,
}: {
  item: UjianSusulan;
  siswa?: EnrollmentItem;
  disabled?: boolean;
  onDelete: () => void;
}) {
  return (
    <div className="flex flex-col gap-2 rounded-md border bg-background/80 p-2 text-xs md:flex-row md:items-center md:justify-between">
      <div>
        <div className="font-medium text-foreground">
          {siswa?.nama || siswa?.email || item.siswa_id}
        </div>
        <div className="text-muted-foreground">
          {item.waktu_mulai ? `${formatDateTime(item.waktu_mulai)} - ` : ''}
          {formatDateTime(item.waktu_selesai)}
          {item.durasi_menit ? ` · durasi ${item.durasi_menit} menit` : ''}
        </div>
      </div>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="h-7 self-start text-xs text-destructive md:self-auto"
        disabled={disabled}
        onClick={onDelete}
      >
        <Trash2 className="size-3.5" />
        Cabut
      </Button>
    </div>
  );
}

// ---------- Row ----------

function RekapRow({
  siswa,
  onCancel,
  disabled,
}: {
  siswa: SiswaRekap;
  onCancel: () => void;
  disabled?: boolean;
}) {
  const canCancel =
    siswa.status_terakhir === 'berlangsung' ||
    siswa.status_terakhir === 'selesai';

  return (
    <tr className="hover:bg-muted/20">
      <td className="px-3 py-2">
        <div className="font-medium">{siswa.siswa_name || siswa.siswa_id}</div>
        <div className="text-xs text-muted-foreground">{siswa.siswa_email}</div>
      </td>
      <td className="px-3 py-2 text-xs text-muted-foreground">
        {siswa.rombel_nama || 'Tanpa rombel'}
      </td>
      <td className="px-3 py-2 text-right tabular-nums">
        {siswa.attempt_count}
      </td>
      <td className="px-3 py-2 text-right tabular-nums">
        {siswa.cancelled_count > 0 ? (
          <span className="rounded-full border border-rose-300 bg-rose-50 px-2 py-0.5 text-xs text-rose-700 dark:border-rose-800 dark:bg-rose-950 dark:text-rose-300">
            {siswa.cancelled_count}
          </span>
        ) : (
          <span className="text-muted-foreground">0</span>
        )}
      </td>
      <td className="px-3 py-2 text-right tabular-nums">
        {siswa.nilai_terbaik != null
          ? siswa.nilai_terbaik.toFixed(2)
          : '—'}
      </td>
      <td className="px-3 py-2 text-right tabular-nums">
        {siswa.nilai_terakhir != null
          ? siswa.nilai_terakhir.toFixed(2)
          : '—'}
      </td>
      <td className="px-3 py-2">
        <StatusBadge status={siswa.status_terakhir ?? ''} />
      </td>
      <td className="px-3 py-2 text-right">
        {canCancel && siswa.hasil_terakhir_id && (
          <Button
            size="sm"
            variant="ghost"
            type="button"
            onClick={onCancel}
            disabled={disabled}
            className="h-7 text-xs"
          >
            <RotateCcw className="size-3.5" />
            Reset
          </Button>
        )}
      </td>
    </tr>
  );
}

function StatusBadge({ status }: { status: string }) {
  const cls =
    status === 'selesai'
      ? 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
      : status === 'berlangsung'
        ? 'border-sky-300 bg-sky-50 text-sky-700 dark:border-sky-800 dark:bg-sky-950 dark:text-sky-300'
        : status === 'dibatalkan'
          ? 'border-rose-300 bg-rose-50 text-rose-700 dark:border-rose-800 dark:bg-rose-950 dark:text-rose-300'
          : 'border-border text-muted-foreground';
  return (
    <span
      className={cn(
        'inline-flex rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
        cls,
      )}
    >
      {status || '—'}
    </span>
  );
}
