/**
 * RekapHasilTable — guru/admin dashboard untuk hasil ulangan bab.
 *
 * Backend endpoint (Task 5.E.1):
 *   GET /bab/:id/hasil-rekap → per-siswa aggregate. Sort by nilai_terbaik DESC.
 *   POST /hasil-soal-bab/:id/cancel → soft-cancel attempt terakhir siswa.
 *
 * UI:
 *   - Header: total siswa + rata-rata kelas
 *   - Tabel per siswa: nama/email + attempt_count + cancelled_count +
 *     nilai_terbaik + nilai_terakhir + status_terakhir + tombol "Reset attempt"
 *   - Cancel handler hanya tersedia kalau hasil_terakhir_id ada DAN
 *     status_terakhir == 'selesai' atau 'berlangsung' (latihan tidak punya
 *     entry di rekap karena backend filter Mode=ulangan).
 */

'use client';

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  AlertTriangle,
  ListChecks,
  Loader2,
  RotateCcw,
  Trophy,
  Users,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  cancelHasilAttempt,
  friendlyHasilError,
  getHasilRekap,
  type SiswaRekap,
} from '@/lib/soalbab-hasil-api';
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { cn } from '@/lib/utils';

export interface RekapHasilTableProps {
  babID: string;
  disabled?: boolean;
}

function fmtNilai(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—';
  // Bulatin 2 desimal kalau ada decimal, kalau bulat tampilin polos.
  return Number.isInteger(n) ? String(n) : n.toFixed(2);
}

function fmtDate(iso: string | null | undefined): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString('id-ID', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function statusLabel(s?: string | null): { text: string; className: string } {
  switch (s) {
    case 'selesai':
      return { text: 'Selesai', className: 'bg-emerald-50 text-emerald-700 border-emerald-200' };
    case 'berlangsung':
      return { text: 'Berlangsung', className: 'bg-sky-50 text-sky-700 border-sky-200' };
    case 'dibatalkan':
      return { text: 'Dibatalkan', className: 'bg-rose-50 text-rose-700 border-rose-200' };
    default:
      return { text: '—', className: 'bg-muted text-muted-foreground border-border' };
  }
}

export function RekapHasilTable({ babID, disabled }: RekapHasilTableProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [confirmTarget, setConfirmTarget] = React.useState<SiswaRekap | null>(null);

  const rekapQuery = useQuery({
    queryKey: ['guru', 'bab', 'hasil-rekap', babID],
    queryFn: () => getHasilRekap(babID),
    staleTime: 10_000,
  });

  const cancelMu = useMutation({
    mutationFn: (hasilID: string) => cancelHasilAttempt(hasilID),
    onSuccess: () => {
      toast({
        title: 'Attempt direset',
        description: 'Siswa bisa mulai attempt baru. Cancelled count bertambah.',
      });
      queryClient.invalidateQueries({ queryKey: ['guru', 'bab', 'hasil-rekap', babID] });
      setConfirmTarget(null);
    },
    onError: (err) => {
      const msg = err instanceof ApiError ? friendlyHasilError(err) : 'Gagal mereset attempt.';
      toast({ title: 'Gagal mereset', description: msg, variant: 'destructive' });
    },
  });

  if (rekapQuery.isPending) {
    return (
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <ListChecks className="size-5 text-muted-foreground" />
            <CardTitle className="text-base">Rekap Hasil Ulangan</CardTitle>
          </div>
          <CardDescription>Memuat data hasil…</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="h-32 animate-pulse rounded-md border bg-muted/40" />
        </CardContent>
      </Card>
    );
  }

  if (rekapQuery.isError || !rekapQuery.data) {
    const msg =
      rekapQuery.error instanceof ApiError
        ? friendlyHasilError(rekapQuery.error)
        : 'Gagal memuat rekap.';
    return (
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <ListChecks className="size-5 text-muted-foreground" />
            <CardTitle className="text-base">Rekap Hasil Ulangan</CardTitle>
          </div>
          <CardDescription>Tidak bisa menampilkan rekap saat ini.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="rounded-md border border-destructive/50 bg-destructive/5 p-3 text-sm text-destructive">
            {msg}
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => rekapQuery.refetch()}
          >
            <RotateCcw className="size-4" />
            Coba lagi
          </Button>
        </CardContent>
      </Card>
    );
  }

  const rekap = rekapQuery.data;

  return (
    <>
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <ListChecks className="size-5 text-muted-foreground" />
              <CardTitle className="text-base">Rekap Hasil Ulangan</CardTitle>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => rekapQuery.refetch()}
              disabled={rekapQuery.isFetching}
            >
              <RotateCcw className={cn('size-4', rekapQuery.isFetching && 'animate-spin')} />
              Refresh
            </Button>
          </div>
          <CardDescription>
            Data hanya untuk mode <strong>ulangan</strong>. Latihan formative tidak masuk ke rekap.
          </CardDescription>
          <div className="mt-2 flex flex-wrap gap-x-6 gap-y-1 text-sm">
            <span className="inline-flex items-center gap-1.5 text-muted-foreground">
              <Users className="size-4" />
              <strong className="text-foreground">{rekap.total}</strong> siswa
            </span>
            <span className="inline-flex items-center gap-1.5 text-muted-foreground">
              <Trophy className="size-4" />
              Rata-rata kelas:{' '}
              <strong className="text-foreground">{fmtNilai(rekap.rata_rata)}</strong>
            </span>
          </div>
        </CardHeader>
        <CardContent>
          {rekap.items.length === 0 ? (
            <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
              Belum ada siswa yang mengerjakan ulangan di bab ini.
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-xs uppercase tracking-wide text-muted-foreground">
                    <th className="py-2 pr-3 font-medium">Siswa</th>
                    <th className="py-2 pr-3 font-medium">Attempt</th>
                    <th className="py-2 pr-3 font-medium">Reset</th>
                    <th className="py-2 pr-3 font-medium">Nilai terbaik</th>
                    <th className="py-2 pr-3 font-medium">Nilai terakhir</th>
                    <th className="py-2 pr-3 font-medium">Status terakhir</th>
                    <th className="py-2 pr-3 font-medium">Mulai terakhir</th>
                    <th className="py-2 pr-3 font-medium text-right">Aksi</th>
                  </tr>
                </thead>
                <tbody>
                  {rekap.items.map((row) => {
                    const sl = statusLabel(row.status_terakhir);
                    const canCancel = Boolean(
                      row.hasil_terakhir_id &&
                        (row.status_terakhir === 'selesai' ||
                          row.status_terakhir === 'berlangsung'),
                    );
                    return (
                      <tr key={row.siswa_id} className="border-b last:border-b-0">
                        <td className="py-2 pr-3">
                          <div className="font-medium">{row.siswa_name || row.siswa_id}</div>
                          <div className="text-xs text-muted-foreground">{row.siswa_email || '—'}</div>
                        </td>
                        <td className="py-2 pr-3">{row.attempt_count}</td>
                        <td className="py-2 pr-3">
                          {row.cancelled_count > 0 ? (
                            <span className="inline-flex items-center gap-1 rounded-full bg-rose-50 px-2 py-0.5 text-xs text-rose-700 ring-1 ring-rose-200">
                              <AlertTriangle className="size-3" />
                              {row.cancelled_count}
                            </span>
                          ) : (
                            <span className="text-muted-foreground">0</span>
                          )}
                        </td>
                        <td className="py-2 pr-3 font-medium">
                          {fmtNilai(row.nilai_terbaik)}
                        </td>
                        <td className="py-2 pr-3">{fmtNilai(row.nilai_terakhir)}</td>
                        <td className="py-2 pr-3">
                          <span
                            className={cn(
                              'inline-flex rounded-full border px-2 py-0.5 text-xs',
                              sl.className,
                            )}
                          >
                            {sl.text}
                          </span>
                        </td>
                        <td className="py-2 pr-3 text-xs text-muted-foreground">
                          {fmtDate(row.mulai_terakhir_at)}
                        </td>
                        <td className="py-2 pr-3 text-right">
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            onClick={() => setConfirmTarget(row)}
                            disabled={disabled || !canCancel || cancelMu.isPending}
                            className="text-rose-600 hover:bg-rose-50 hover:text-rose-700"
                          >
                            Reset attempt
                          </Button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      <CancelAttemptConfirmDialog
        target={confirmTarget}
        open={Boolean(confirmTarget)}
        onOpenChange={(o) => {
          if (!o) setConfirmTarget(null);
        }}
        pending={cancelMu.isPending}
        onConfirm={() => {
          if (confirmTarget?.hasil_terakhir_id) {
            cancelMu.mutate(confirmTarget.hasil_terakhir_id);
          }
        }}
      />
    </>
  );
}

interface CancelAttemptConfirmDialogProps {
  target: SiswaRekap | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  onConfirm: () => void;
}

function CancelAttemptConfirmDialog({
  target,
  open,
  onOpenChange,
  pending,
  onConfirm,
}: CancelAttemptConfirmDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Reset attempt ulangan?</DialogTitle>
          <DialogDescription>
            Attempt terakhir {target?.siswa_name || 'siswa'} akan ditandai{' '}
            <strong>dibatalkan</strong>. Siswa boleh memulai attempt baru tanpa terkena batas
            attempt. Aksi ini tercatat di rekap (cancelled count bertambah) dan tidak bisa
            dibatalkan.
          </DialogDescription>
        </DialogHeader>
        {target ? (
          <div className="space-y-1 rounded-md border bg-muted/30 p-3 text-sm">
            <div>
              <span className="text-muted-foreground">Siswa:</span>{' '}
              <strong>{target.siswa_name || target.siswa_id}</strong>
            </div>
            <div>
              <span className="text-muted-foreground">Email:</span> {target.siswa_email || '—'}
            </div>
            <div>
              <span className="text-muted-foreground">Status terakhir:</span>{' '}
              {target.status_terakhir || '—'}
            </div>
            <div>
              <span className="text-muted-foreground">Nilai terakhir:</span>{' '}
              {fmtNilai(target.nilai_terakhir)}
            </div>
          </div>
        ) : null}
        <DialogFooter className="gap-2">
          <Button
            type="button"
            variant="ghost"
            onClick={() => onOpenChange(false)}
            disabled={pending}
          >
            Batal
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={onConfirm}
            disabled={pending || !target?.hasil_terakhir_id}
          >
            {pending ? <Loader2 className="size-4 animate-spin" /> : null}
            Ya, reset attempt
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
