'use client';

/**
 * SiswaNilaiBabTable — per-bab breakdown table for siswa rekap nilai page.
 *
 * Visual: neo-brutalism + pastel pop. Wrapped in siswa-border + hard shadow.
 * Renders one row per bab with:
 *   - Nomor + judul
 *   - Ulangan bab pct + jumlah soal
 *   - Tugas pct + ratio dinilai/total
 *   - Total per bab (weighted avg, NULL-aware)
 */

import * as React from 'react';
import { BookOpen } from 'lucide-react';

import { formatNilai, type NilaiBabRow } from '@/lib/nilai-api';
import { cn } from '@/lib/utils';

interface Props {
  bab: NilaiBabRow[];
}

function nilaiClass(n: number | null): string {
  if (n === null) return 'text-siswa-text-muted';
  if (n >= 75) return 'text-emerald-700 font-bold';
  if (n >= 60) return 'text-amber-700 font-bold';
  return 'text-rose-700 font-bold';
}

export function SiswaNilaiBabTable({ bab }: Props) {
  if (bab.length === 0) {
    return (
      <div className="rounded-siswa border-2 border-dashed border-siswa-border-soft bg-siswa-surface/60 p-8 text-center">
        <BookOpen
          className="mx-auto mb-2 size-8 text-siswa-text-muted"
          strokeWidth={2.5}
        />
        <p className="text-sm text-siswa-text-muted">
          Belum ada bab yang dipublish di kelas ini, jadi belum ada nilai bab
          yang bisa direkap.
        </p>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-siswa siswa-border siswa-shadow-sm">
      <table className="w-full text-sm">
        <thead className="bg-siswa-cream text-xs font-bold uppercase tracking-wide text-siswa-text">
          <tr>
            <th className="border-b-2 border-siswa-border px-3 py-2 text-left">
              Bab
            </th>
            <th className="border-b-2 border-siswa-border px-3 py-2 text-right">
              Ulangan Bab
            </th>
            <th className="border-b-2 border-siswa-border px-3 py-2 text-right">
              Tugas
            </th>
            <th className="border-b-2 border-siswa-border px-3 py-2 text-right">
              Total
            </th>
          </tr>
        </thead>
        <tbody className="divide-y-2 divide-siswa-border-soft bg-siswa-surface">
          {bab.map((row) => (
            <tr key={row.bab_id} className="align-top">
              <td className="px-3 py-3">
                <div className="flex items-start gap-2">
                  <span className="rounded-md border-2 border-siswa-border bg-siswa-cream px-2 py-0.5 text-xs font-bold tabular-nums">
                    {row.nomor}
                  </span>
                  <span className="min-w-0 break-words font-semibold">
                    {row.judul}
                  </span>
                </div>
              </td>
              <td className="px-3 py-3 text-right">
                <div
                  className={cn(
                    'tabular-nums',
                    nilaiClass(row.nilai_ulangan_bab),
                  )}
                >
                  {formatNilai(row.nilai_ulangan_bab)}
                </div>
                <div className="text-xs text-siswa-text-muted">
                  {row.jumlah_soal_ulangan_bab > 0
                    ? `${row.jumlah_soal_ulangan_bab} soal`
                    : 'tanpa soal'}
                </div>
              </td>
              <td className="px-3 py-3 text-right">
                <div
                  className={cn(
                    'tabular-nums',
                    nilaiClass(row.nilai_tugas_bab),
                  )}
                >
                  {formatNilai(row.nilai_tugas_bab)}
                </div>
                <div className="text-xs text-siswa-text-muted">
                  {row.jumlah_tugas > 0
                    ? `${row.jumlah_tugas_dinilai}/${row.jumlah_tugas} dinilai`
                    : 'tanpa tugas'}
                </div>
                {row.bobot_tugas_total > 0 ? (
                  <div className="text-xs text-siswa-text-muted/80">
                    Bobot item total {row.bobot_tugas_total}
                  </div>
                ) : null}
              </td>
              <td className="px-3 py-3 text-right">
                <div
                  className={cn(
                    'siswa-display text-base tabular-nums',
                    nilaiClass(row.total),
                  )}
                >
                  {formatNilai(row.total)}
                </div>
                {row.total === null ? (
                  <div className="text-xs text-siswa-text-muted">
                    belum ada nilai
                  </div>
                ) : null}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
