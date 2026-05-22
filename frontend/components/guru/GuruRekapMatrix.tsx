'use client';

/**
 * GuruRekapMatrix — sticky-header matrix table untuk guru rekap nilai
 * (Task 7.B FE).
 *
 * Layout:
 *   - Header row: Siswa | Total | Bab 1 (T/U/T) | ... | Ujian 1 (Best/Last) | ...
 *   - Group header (2-row): top spans bab/ujian judul, bottom spans sub-cols
 *   - Sticky kolom kiri (Siswa nama + Total) supaya scroll horizontal tidak
 *     hilang context.
 *   - Numeric cells: 1-decimal, color-tier ≥75 / ≥60 / <60 / null=muted
 */

import * as React from 'react';
import { ScrollText } from 'lucide-react';

import { cn } from '@/lib/utils';
import {
  formatNilai,
  type GuruRekapResponse,
} from '@/lib/nilai-api';

interface Props {
  data: GuruRekapResponse;
}

function nilaiClass(n: number | null): string {
  if (n === null) return 'text-muted-foreground';
  if (n >= 75) return 'text-emerald-700 dark:text-emerald-400 font-semibold';
  if (n >= 60) return 'text-amber-700 dark:text-amber-400 font-semibold';
  return 'text-rose-700 dark:text-rose-400 font-semibold';
}

export function GuruRekapMatrix({ data }: Props) {
  const { bab, ujian, rows } = data;

  if (rows.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-8 text-center">
        <ScrollText className="mx-auto mb-2 size-6 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          Belum ada siswa terdaftar di kelas ini. Tambah siswa lewat enrollment
          atau bagikan kode invite.
        </p>
      </div>
    );
  }

  const babColCount = bab.length * 3;
  const ujianColCount = ujian.length * 3;
  const fixedColCount = 2; // siswa + total

  return (
    <div className="overflow-x-auto rounded-md border">
      <table className="w-full border-collapse text-xs">
        <thead className="bg-muted/40">
          {/* Top group row */}
          <tr>
            <th
              rowSpan={2}
              className="sticky left-0 z-20 min-w-[180px] border-b bg-muted/40 px-3 py-2 text-left font-medium"
            >
              Siswa
            </th>
            <th
              rowSpan={2}
              className="sticky left-[180px] z-20 border-b border-l bg-muted/40 px-3 py-2 text-right font-medium"
            >
              Total
            </th>
            {bab.map((b) => (
              <th
                key={b.id}
                colSpan={3}
                className="border-b border-l px-3 py-2 text-center text-muted-foreground"
                title={b.judul}
              >
                <span className="rounded bg-muted px-1.5 py-0.5 tabular-nums">
                  Bab {b.nomor}
                </span>
                <span className="ml-1 max-w-[140px] truncate font-medium text-foreground">
                  {b.judul}
                </span>
              </th>
            ))}
            {ujian.map((u) => (
              <th
                key={u.id}
                colSpan={3}
                className="border-b border-l px-3 py-2 text-center text-muted-foreground"
                title={u.judul}
              >
                <span className="rounded bg-muted px-1.5 py-0.5">UH</span>
                <span className="ml-1 max-w-[140px] truncate font-medium text-foreground">
                  {u.judul}
                </span>
              </th>
            ))}
          </tr>
          {/* Sub header row */}
          <tr className="text-[10px] uppercase tracking-wide text-muted-foreground">
            {bab.map((b) => (
              <React.Fragment key={b.id}>
                <th className="border-b border-l px-2 py-1 text-right font-normal">Total</th>
                <th className="border-b px-2 py-1 text-right font-normal">Ul</th>
                <th className="border-b px-2 py-1 text-right font-normal">Tg</th>
              </React.Fragment>
            ))}
            {ujian.map((u) => (
              <React.Fragment key={u.id}>
                <th className="border-b border-l px-2 py-1 text-right font-normal">Best</th>
                <th className="border-b px-2 py-1 text-right font-normal">Last</th>
                <th className="border-b px-2 py-1 text-right font-normal">N</th>
              </React.Fragment>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y">
          {rows.map((row) => (
            <tr key={row.siswa_id} className="hover:bg-muted/20">
              <td className="sticky left-0 z-10 min-w-[180px] bg-background px-3 py-2 font-medium">
                <div className="truncate" title={row.siswa_nama}>
                  {row.siswa_nama || '—'}
                </div>
              </td>
              <td
                className={cn(
                  'sticky left-[180px] z-10 border-l bg-background px-3 py-2 text-right tabular-nums',
                  nilaiClass(row.total_kelas),
                )}
              >
                {formatNilai(row.total_kelas)}
              </td>
              {row.bab.map((c) => (
                <React.Fragment key={c.bab_id}>
                  <td
                    className={cn(
                      'border-l px-2 py-2 text-right tabular-nums',
                      nilaiClass(c.total),
                    )}
                  >
                    {formatNilai(c.total)}
                  </td>
                  <td
                    className={cn(
                      'px-2 py-2 text-right tabular-nums text-muted-foreground/80',
                      c.ulangan_bab !== null && nilaiClass(c.ulangan_bab),
                    )}
                  >
                    {formatNilai(c.ulangan_bab)}
                  </td>
                  <td
                    className={cn(
                      'px-2 py-2 text-right tabular-nums text-muted-foreground/80',
                      c.tugas !== null && nilaiClass(c.tugas),
                    )}
                  >
                    {formatNilai(c.tugas)}
                  </td>
                </React.Fragment>
              ))}
              {row.ujian.map((c) => (
                <React.Fragment key={c.ujian_id}>
                  <td
                    className={cn(
                      'border-l px-2 py-2 text-right tabular-nums',
                      nilaiClass(c.nilai_terbaik),
                    )}
                  >
                    {formatNilai(c.nilai_terbaik)}
                  </td>
                  <td
                    className={cn(
                      'px-2 py-2 text-right tabular-nums text-muted-foreground/80',
                      c.nilai_terakhir !== null && nilaiClass(c.nilai_terakhir),
                    )}
                  >
                    {formatNilai(c.nilai_terakhir)}
                  </td>
                  <td className="px-2 py-2 text-right tabular-nums text-muted-foreground">
                    {c.attempt_count > 0 ? c.attempt_count : '—'}
                  </td>
                </React.Fragment>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      <div className="border-t bg-muted/20 px-3 py-1.5 text-[10px] text-muted-foreground">
        {rows.length} siswa · {babColCount} kolom bab · {ujianColCount} kolom
        ulangan · {fixedColCount} kolom tetap
      </div>
    </div>
  );
}
