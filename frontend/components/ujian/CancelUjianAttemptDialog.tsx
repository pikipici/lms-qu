'use client';

/**
 * CancelUjianAttemptDialog — confirm dialog untuk soft-cancel attempt
 * siswa pada satu Ujian. Mirror SoalBab CancelAttemptConfirmDialog tapi
 * adapted untuk Ujian endpoint POST /hasil-ujian/:id/cancel.
 *
 * Locked #76: dibatalkan tidak count attempt_no — partial-unique slot
 * (ujian_id, siswa_id) WHERE deleted_at IS NULL released, siswa boleh
 * start fresh attempt.
 */

import * as React from 'react';
import { Loader2, RotateCcw } from 'lucide-react';

import { type SiswaRekap } from '@/lib/ujian-api';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

export interface CancelUjianAttemptDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  siswa: SiswaRekap | null;
  onConfirm: (hasilID: string) => void;
  pending?: boolean;
}

export function CancelUjianAttemptDialog({
  open,
  onOpenChange,
  siswa,
  onConfirm,
  pending,
}: CancelUjianAttemptDialogProps) {
  const hasilID = siswa?.hasil_terakhir_id ?? null;
  const statusTerakhir = siswa?.status_terakhir ?? '';

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Reset attempt siswa?</DialogTitle>
          <DialogDescription>
            Soft-cancel attempt terakhir siswa ini supaya bisa start fresh.
          </DialogDescription>
        </DialogHeader>

        {siswa && (
          <div className="space-y-2 rounded-md border bg-muted/30 p-3 text-sm">
            <div>
              <span className="text-muted-foreground">Siswa:</span>{' '}
              <span className="font-medium">
                {siswa.siswa_name || siswa.siswa_id}
              </span>
            </div>
            <div>
              <span className="text-muted-foreground">Status terakhir:</span>{' '}
              <span className="font-medium">{statusTerakhir || '—'}</span>
            </div>
            <div>
              <span className="text-muted-foreground">Attempt count:</span>{' '}
              <span className="font-medium">{siswa.attempt_count}</span>{' '}
              {siswa.cancelled_count > 0 && (
                <span className="text-xs text-rose-700 dark:text-rose-300">
                  · {siswa.cancelled_count} cancelled
                </span>
              )}
            </div>
            {siswa.nilai_terakhir != null && (
              <div>
                <span className="text-muted-foreground">Nilai terakhir:</span>{' '}
                <span className="font-medium">
                  {siswa.nilai_terakhir.toFixed(2)}
                </span>
              </div>
            )}
          </div>
        )}

        <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-xs text-amber-900 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-200">
          <p>
            Attempt akan ditandai <strong>dibatalkan</strong> dan slot single-
            attempt direset. Siswa boleh start ujian dari awal lagi.
          </p>
          <p className="mt-1">
            Hasil cancelled tetap tercatat di event log untuk forensik —
            tidak hilang permanen.
          </p>
        </div>

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
            onClick={() => {
              if (hasilID) onConfirm(hasilID);
            }}
            disabled={pending || !hasilID}
          >
            {pending && <Loader2 className="size-4 animate-spin" />}
            <RotateCcw className="size-4" />
            Reset attempt
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
