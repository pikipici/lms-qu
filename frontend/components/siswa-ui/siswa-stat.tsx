'use client';

import * as React from 'react';
import type { LucideIcon } from 'lucide-react';
import { cn } from '@/lib/utils';
import { SiswaCard, type SiswaCardTone } from './siswa-card';

interface SiswaStatProps {
  label: string;
  value: React.ReactNode;
  hint?: React.ReactNode;
  Icon?: LucideIcon;
  tone?: SiswaCardTone;
  loading?: boolean;
  className?: string;
}

export function SiswaStat({
  label,
  value,
  hint,
  Icon,
  tone = 'surface',
  loading,
  className,
}: SiswaStatProps) {
  return (
    <SiswaCard tone={tone} shadow="md" className={cn('p-5 sm:p-6', className)}>
      <div className="flex items-start justify-between gap-3">
        <div className="space-y-1">
          <p className="text-xs font-semibold uppercase tracking-wider text-siswa-text-muted">
            {label}
          </p>
          {loading ? (
            <div className="h-9 w-20 animate-pulse rounded bg-siswa-text/10" />
          ) : (
            <p className="siswa-display text-3xl font-bold leading-none">
              {value}
            </p>
          )}
          {hint ? (
            <p className="text-xs text-siswa-text-muted">{hint}</p>
          ) : null}
        </div>
        {Icon ? (
          <span className="grid size-10 shrink-0 place-items-center rounded-siswa siswa-border bg-siswa-surface">
            <Icon className="size-5" />
          </span>
        ) : null}
      </div>
    </SiswaCard>
  );
}
