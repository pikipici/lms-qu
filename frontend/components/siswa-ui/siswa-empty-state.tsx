'use client';

import * as React from 'react';
import type { LucideIcon } from 'lucide-react';
import { cn } from '@/lib/utils';

interface SiswaEmptyStateProps {
  /** Big visual: emoji string, lucide Icon, or custom node. */
  icon?: React.ReactNode | LucideIcon;
  title: string;
  description?: React.ReactNode;
  action?: React.ReactNode;
  className?: string;
}

function isLucideIcon(x: unknown): x is LucideIcon {
  return typeof x === 'function';
}

export function SiswaEmptyState({
  icon,
  title,
  description,
  action,
  className,
}: SiswaEmptyStateProps) {
  let visual: React.ReactNode = null;
  if (typeof icon === 'string') {
    visual = (
      <span aria-hidden className="text-5xl leading-none">
        {icon}
      </span>
    );
  } else if (typeof icon === 'function') {
    const Icon = icon as LucideIcon;
    visual = <Icon className="size-12" strokeWidth={2.5} />;
  } else if (icon) {
    visual = icon as React.ReactNode;
  }

  return (
    <div
      className={cn(
        'siswa-border rounded-siswa-lg bg-siswa-surface p-8 text-center',
        'flex flex-col items-center gap-3',
        className,
      )}
    >
      {visual ? (
        <div className="grid size-20 place-items-center rounded-siswa siswa-border bg-siswa-cream">
          {visual}
        </div>
      ) : null}
      <h3 className="siswa-display text-xl font-bold">{title}</h3>
      {description ? (
        <div className="max-w-md text-sm text-siswa-text-muted">
          {description}
        </div>
      ) : null}
      {action ? <div className="pt-1">{action}</div> : null}
    </div>
  );
}
