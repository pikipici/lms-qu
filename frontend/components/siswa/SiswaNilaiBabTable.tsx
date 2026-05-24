'use client';

/**
 * SiswaNilaiBabTable — per-bab breakdown table for siswa rekap nilai page.
 *
 * Renders one row per bab with:
 *   - Nomor + judul
 *   - Ulangan bab pct (with bobot label) + jumlah soal
 *   - Tugas pct (with bobot label) + ratio dinilai/total
 *   - Total per bab (weighted avg, NULL-aware)
 *
 * Empty state when bab[] kosong (kelas belum punya bab published).
 */

import * as React from 'react';
import { BookOpen } from 'lucide-react';

import {
  bobotLabel,
  formatNilai,
  type NilaiBabRow,
  type NilaiKelasInfo,
} from '@/lib/nilai-api';
import { cn } from '@/lib/utils';

interface Props {
  kelas: NilaiKelasInfo;
  bab: NilaiBabRow[];
}

function nilaiClass(n: number | null): string {
  if (n === null) return 'text-muted-foreground';
  if (n >= 75) return 'text-emerald-700 dark:text-emerald-400 font-semibold';
  if (n >= 60) return 'text-amber-700 dark:text-amber-400 font-semibold';
  return 'text-rose-700 dark:text-rose-400 font-semibold';
}

export function SiswaNilaiBabTable({ kelas, bab }: Props) {
  if (bab.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-8 text-center">
        <BookOpen className="mx-auto mb-2 size-6 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          Belum ada bab yang dipublish di kelas ini, jadi belum ada nilai bab
          yang bisa direkap.
        </p>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-md border">
      <table className="w-full text-sm">
        <thead className="bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
          <tr>
            <th className="px-3 py-2 text-left">Bab</th>
            <th className="px-3 py-2 text-right">
              Ulangan Bab
              <span className="ml-1 font-normal normal-case text-muted-foreground/70">
                ({bobotLabel(kelas.bobot_soal_ulangan)})
              </span>
            </th>
            <th className="px-3 py-2 text-right">
              Tugas
              <span className="ml-1 font-normal normal-case text-muted-foreground/70">
                ({bobotLabel(kelas.bobot_tugas)})
              </span>
            </th>
            <th className="px-3 py-2 text-right">Total</th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {bab.map((row) => (
            <tr key={row.bab_id} className="align-top">
              <td className="px-3 py-3">
                <div className="flex items-start gap-2">
                  <span className="rounded-md bg-muted px-2 py-0.5 text-xs font-medium tabular-nums text-muted-foreground">
                    {row.nomor}
                  </span>
                  <span className="min-w-0 break-words font-medium">
                    {row.judul}
                  </span>
                </div>
              </td>
              <td className="px-3 py-3 text-right">
                <div
                  className={cn('tabular-nums', nilaiClass(row.nilai_ulangan_bab))}
                >
                  {formatNilai(row.nilai_ulangan_bab)}
                </div>
                <div className="text-xs text-muted-foreground">
                  {row.jumlah_soal_ulangan_bab > 0
                    ? `${row.jumlah_soal_ulangan_bab} soal`
                    : 'tanpa soal'}
                </div>
              </td>
              <td className="px-3 py-3 text-right">
                <div
                  className={cn('tabular-nums', nilaiClass(row.nilai_tugas_bab))}
                >
                  {formatNilai(row.nilai_tugas_bab)}
                </div>
                <div className="text-xs text-muted-foreground">
                  {row.jumlah_tugas > 0
                    ? `${row.jumlah_tugas_dinilai}/${row.jumlah_tugas} dinilai`
                    : 'tanpa tugas'}
                </div>
                {row.bobot_tugas_total > 0 && (
                  <div className="text-xs text-muted-foreground/80">
                    Bobot item total {row.bobot_tugas_total}
                  </div>
                )}
              </td>
              <td className="px-3 py-3 text-right">
                <div
                  className={cn(
                    'text-base tabular-nums',
                    nilaiClass(row.total),
                  )}
                >
                  {formatNilai(row.total)}
                </div>
                {row.total === null && (
                  <div className="text-xs text-muted-foreground">
                    belum ada nilai
                  </div>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
