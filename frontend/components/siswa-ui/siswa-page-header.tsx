'use client';

import * as React from 'react';
import { cn } from '@/lib/utils';

interface SiswaPageHeaderProps {
  /** Small label that sits above the title — typically section / breadcrumb. */
  eyebrow?: React.ReactNode;
  /** Main heading. Span with `text-siswa-yellow` highlight gets a built-in
   *  block-shadow effect via `<mark>` styling. */
  title: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
  className?: string;
}

export function SiswaPageHeader({
  eyebrow,
  title,
  description,
  actions,
  className,
}: SiswaPageHeaderProps) {
  return (
    <header
      className={cn(
        'flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between',
        className,
      )}
    >
      <div className="space-y-2">
        {eyebrow ? (
          <div className="text-xs font-semibold uppercase tracking-[0.18em] text-siswa-text-muted">
            {eyebrow}
          </div>
        ) : null}
        <h1 className="siswa-display text-3xl font-bold leading-tight sm:text-4xl">
          {title}
        </h1>
        {description ? (
          <p className="max-w-2xl text-sm text-siswa-text-muted sm:text-base">
            {description}
          </p>
        ) : null}
      </div>
      {actions ? <div className="flex flex-wrap gap-2">{actions}</div> : null}
    </header>
  );
}

/**
 * Highlight helper — wraps inline text with the signature yellow block.
 * Usage: <SiswaHighlight>Halo, {name}</SiswaHighlight>
 */
export function SiswaHighlight({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        'relative inline-block bg-siswa-yellow px-2 leading-tight',
        className,
      )}
    >
      <span className="relative z-10">{children}</span>
    </span>
  );
}
