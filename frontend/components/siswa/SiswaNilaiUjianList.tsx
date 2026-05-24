'use client';

/**
 * SiswaNilaiUjianList — list ulangan harian (cross-bab) per kelas.
 *
 * Visual: neo-brutalism + pastel pop. Setiap row: judul ujian, nilai
 * terbaik, nilai terakhir, attempt_count.
 */

import * as React from 'react';
import { GraduationCap } from 'lucide-react';

import { formatNilai, type NilaiUjianRow } from '@/lib/nilai-api';
import { cn } from '@/lib/utils';

interface Props {
  rows: NilaiUjianRow[];
}

function nilaiClass(n: number | null): string {
  if (n === null) return 'text-siswa-text-muted';
  if (n >= 75) return 'text-emerald-700 font-bold';
  if (n >= 60) return 'text-amber-700 font-bold';
  return 'text-rose-700 font-bold';
}

export function SiswaNilaiUjianList({ rows }: Props) {
  if (rows.length === 0) {
    return (
      <div className="rounded-siswa border-2 border-dashed border-siswa-border-soft bg-siswa-surface/60 p-8 text-center">
        <GraduationCap
          className="mx-auto mb-2 size-8 text-siswa-text-muted"
          strokeWidth={2.5}
        />
        <p className="text-sm text-siswa-text-muted">
          Belum ada ulangan harian yang dipublish di kelas ini.
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
              Ulangan
            </th>
            <th className="border-b-2 border-siswa-border px-3 py-2 text-right">
              Terbaik
            </th>
            <th className="border-b-2 border-siswa-border px-3 py-2 text-right">
              Terakhir
            </th>
            <th className="border-b-2 border-siswa-border px-3 py-2 text-right">
              Attempt
            </th>
          </tr>
        </thead>
        <tbody className="divide-y-2 divide-siswa-border-soft bg-siswa-surface">
          {rows.map((row) => (
            <tr key={row.ujian_id} className="align-top">
              <td className="px-3 py-3">
                <div className="font-semibold">{row.judul}</div>
                <div className="text-xs text-siswa-text-muted">
                  Bobot {row.bobot ?? 100}
                </div>
              </td>
              <td className="px-3 py-3 text-right">
                <span
                  className={cn(
                    'tabular-nums',
                    nilaiClass(row.nilai_terbaik),
                  )}
                >
                  {formatNilai(row.nilai_terbaik)}
                </span>
              </td>
              <td className="px-3 py-3 text-right">
                <span
                  className={cn(
                    'tabular-nums',
                    nilaiClass(row.nilai_terakhir),
                  )}
                >
                  {formatNilai(row.nilai_terakhir)}
                </span>
              </td>
              <td className="px-3 py-3 text-right tabular-nums text-siswa-text-muted">
                {row.attempt_count > 0 ? row.attempt_count : '—'}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
