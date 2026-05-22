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
import { Loader2, RotateCcw } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type SiswaRekap,
  cancelUjianHasil,
  friendlyUjianError,
  getRekapHasilUjian,
} from '@/lib/ujian-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

import { CancelUjianAttemptDialog } from './CancelUjianAttemptDialog';

export interface RekapHasilUjianTableProps {
  ujianID: string;
  /** Disable cancel actions kalau ujian/kelas archived. */
  disabled?: boolean;
}

export function RekapHasilUjianTable({
  ujianID,
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
      </div>

      {items.length === 0 ? (
        <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
          Belum ada siswa yang attempt ujian ini.
        </div>
      ) : (
        <div className="overflow-x-auto rounded-md border">
          <table className="w-full text-sm">
            <thead className="bg-muted/40 text-xs">
              <tr>
                <th className="px-3 py-2 text-left">Siswa</th>
                <th className="px-3 py-2 text-right">Attempt</th>
                <th className="px-3 py-2 text-right">Cancel</th>
                <th className="px-3 py-2 text-right">Terbaik</th>
                <th className="px-3 py-2 text-right">Terakhir</th>
                <th className="px-3 py-2 text-left">Status terakhir</th>
                <th className="px-3 py-2 text-right" />
              </tr>
            </thead>
            <tbody className="divide-y">
              {items.map((s) => (
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
