'use client';

/**
 * SiswaBabProgressBar — visualisasi progress per bab untuk siswa.
 *
 * Inline Tailwind bar (no shadcn Progress yet) — width-driven via inline
 * style supaya gak ke-purge oleh Tailwind JIT. Color tier:
 *   - 0%      → muted (bab belum dimulai / kosong)
 *   - 1-99%   → primary
 *   - 100%    → emerald (bab selesai)
 *
 * Tooltip pakai native `title` attr — shadcn tooltip belum di-install.
 * Dipanggil di /siswa/kelas/detail (list) + /siswa/kelas/detail/bab
 * (header detail).
 */

import * as React from 'react';

import { cn } from '@/lib/utils';

interface SiswaBabProgressBarProps {
  persen: number;
  materiRead: number;
  materiTotal: number;
  babKosong: boolean;
  /** size variant — 'sm' utk list, 'md' utk header detail. */
  size?: 'sm' | 'md';
  className?: string;
}

function pickTrackColor(persen: number, kosong: boolean): string {
  if (kosong) return 'bg-muted-foreground/30';
  if (persen >= 100) return 'bg-emerald-500';
  if (persen > 0) return 'bg-primary';
  return 'bg-muted-foreground/40';
}

export function SiswaBabProgressBar({
  persen,
  materiRead,
  materiTotal,
  babKosong,
  size = 'sm',
  className,
}: SiswaBabProgressBarProps) {
  const pctClamped = Math.max(0, Math.min(100, Number.isFinite(persen) ? persen : 0));
  const heightClass = size === 'md' ? 'h-2.5' : 'h-1.5';
  const trackColor = pickTrackColor(pctClamped, babKosong);

  const tooltip = babKosong
    ? 'Bab ini belum punya materi.'
    : `${materiRead} dari ${materiTotal} materi sudah dibaca (${pctClamped.toFixed(2)}%).`;

  return (
    <div
      className={cn('w-full space-y-1', className)}
      role="progressbar"
      aria-valuenow={pctClamped}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-label={tooltip}
      title={tooltip}
    >
      <div
        className={cn(
          'w-full overflow-hidden rounded-full bg-muted',
          heightClass,
        )}
      >
        <div
          className={cn('h-full transition-all duration-300', trackColor)}
          style={{ width: `${pctClamped}%` }}
        />
      </div>
      <div className="flex items-center justify-between text-[11px] text-muted-foreground">
        <span>
          {babKosong
            ? 'Belum ada materi'
            : `${materiRead}/${materiTotal} materi`}
        </span>
        <span className="font-medium tabular-nums">
          {pctClamped.toFixed(pctClamped % 1 === 0 ? 0 : 2)}%
        </span>
      </div>
    </div>
  );
}
