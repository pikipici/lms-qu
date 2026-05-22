'use client';

/**
 * SiswaNilaiUjianList — list ulangan harian (cross-bab) per kelas.
 *
 * Setiap row: judul ujian, nilai terbaik, nilai terakhir, attempt_count.
 * Kalau attempt_count = 0 → empty placeholder dengan label "belum dicoba".
 */

import * as React from 'react';
import { GraduationCap } from 'lucide-react';

import { formatNilai, type NilaiUjianRow } from '@/lib/nilai-api';
import { cn } from '@/lib/utils';

interface Props {
  rows: NilaiUjianRow[];
}

function nilaiClass(n: number | null): string {
  if (n === null) return 'text-muted-foreground';
  if (n >= 75) return 'text-emerald-700 dark:text-emerald-400 font-semibold';
  if (n >= 60) return 'text-amber-700 dark:text-amber-400 font-semibold';
  return 'text-rose-700 dark:text-rose-400 font-semibold';
}

export function SiswaNilaiUjianList({ rows }: Props) {
  if (rows.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-8 text-center">
        <GraduationCap className="mx-auto mb-2 size-6 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          Belum ada ulangan harian yang dipublish di kelas ini.
        </p>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-md border">
      <table className="w-full text-sm">
        <thead className="bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
          <tr>
            <th className="px-3 py-2 text-left">Ulangan</th>
            <th className="px-3 py-2 text-right">Terbaik</th>
            <th className="px-3 py-2 text-right">Terakhir</th>
            <th className="px-3 py-2 text-right">Attempt</th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {rows.map((row) => (
            <tr key={row.ujian_id} className="align-top">
              <td className="px-3 py-3 font-medium">{row.judul}</td>
              <td className="px-3 py-3 text-right">
                <span
                  className={cn('tabular-nums', nilaiClass(row.nilai_terbaik))}
                >
                  {formatNilai(row.nilai_terbaik)}
                </span>
              </td>
              <td className="px-3 py-3 text-right">
                <span
                  className={cn('tabular-nums', nilaiClass(row.nilai_terakhir))}
                >
                  {formatNilai(row.nilai_terakhir)}
                </span>
              </td>
              <td className="px-3 py-3 text-right tabular-nums text-muted-foreground">
                {row.attempt_count > 0 ? row.attempt_count : '—'}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
