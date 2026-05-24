'use client';

import * as React from 'react';
import { cn } from '@/lib/utils';

interface SiswaProgressProps {
  /** 0..100 percent. Values outside are clamped. */
  value: number;
  /** Tailwind background utility for the fill color. */
  fillClassName?: string;
  /** Show numeric label inline. */
  label?: React.ReactNode;
  className?: string;
  size?: 'sm' | 'md';
  ariaLabel?: string;
}

export function SiswaProgress({
  value,
  fillClassName = 'bg-siswa-yellow',
  label,
  className,
  size = 'md',
  ariaLabel,
}: SiswaProgressProps) {
  const pct = Math.max(0, Math.min(100, Math.round(value)));
  const height = size === 'sm' ? 'h-2.5' : 'h-3.5';
  return (
    <div className={cn('w-full space-y-1', className)}>
      {label !== undefined ? (
        <div className="flex items-center justify-between text-xs font-semibold">
          <span className="text-siswa-text-muted">{label}</span>
          <span>{pct}%</span>
        </div>
      ) : null}
      <div
        role="progressbar"
        aria-valuenow={pct}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={ariaLabel ?? (typeof label === 'string' ? label : 'Progress')}
        className={cn(
          'relative w-full overflow-hidden rounded-full siswa-border bg-siswa-surface',
          height,
        )}
      >
        <div
          className={cn('h-full transition-[width] duration-300', fillClassName)}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  );
}
