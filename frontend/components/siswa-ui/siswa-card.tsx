'use client';

import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@/lib/utils';

/**
 * SiswaCard — neo-brutalist card primitive.
 *
 * Variants:
 *   - tone: surface (white) | section accent (materi/latihan/ulangan/tugas/nilai/umum)
 *   - interactive: adds press feedback (translate + shadow shrink) on hover/active
 *
 * The signature look = thick black border + hard offset shadow with no blur.
 */
const cardVariants = cva(
  'siswa-border rounded-siswa bg-siswa-surface text-siswa-text',
  {
    variants: {
      tone: {
        surface: 'bg-siswa-surface',
        materi: 'bg-siswa-materi/40',
        latihan: 'bg-siswa-latihan/45',
        ulangan: 'bg-siswa-ulangan/40',
        tugas: 'bg-siswa-tugas/35',
        nilai: 'bg-siswa-nilai/45',
        umum: 'bg-siswa-umum/70',
      },
      shadow: {
        none: '',
        sm: 'siswa-shadow-sm',
        md: 'siswa-shadow',
        lg: 'siswa-shadow-lg',
      },
      interactive: {
        true: 'siswa-press cursor-pointer',
        false: '',
      },
    },
    defaultVariants: {
      tone: 'surface',
      shadow: 'md',
      interactive: false,
    },
  },
);

export type SiswaCardTone = NonNullable<VariantProps<typeof cardVariants>['tone']>;

export interface SiswaCardProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof cardVariants> {
  asButton?: boolean;
}

export const SiswaCard = React.forwardRef<HTMLDivElement, SiswaCardProps>(
  function SiswaCard(
    { className, tone, shadow, interactive, asButton, children, ...rest },
    ref,
  ) {
    return (
      <div
        ref={ref}
        role={asButton ? 'button' : undefined}
        tabIndex={asButton ? 0 : undefined}
        className={cn(cardVariants({ tone, shadow, interactive }), className)}
        {...rest}
      >
        {children}
      </div>
    );
  },
);

export function SiswaCardHeader({
  className,
  ...rest
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'flex flex-col gap-1 px-5 pt-5 pb-3 sm:px-6 sm:pt-6',
        className,
      )}
      {...rest}
    />
  );
}

export function SiswaCardTitle({
  className,
  ...rest
}: React.HTMLAttributes<HTMLHeadingElement>) {
  return (
    <h3
      className={cn(
        'siswa-display text-lg font-bold leading-tight sm:text-xl',
        className,
      )}
      {...rest}
    />
  );
}

export function SiswaCardDescription({
  className,
  ...rest
}: React.HTMLAttributes<HTMLParagraphElement>) {
  return (
    <p
      className={cn('text-sm text-siswa-text-muted', className)}
      {...rest}
    />
  );
}

export function SiswaCardBody({
  className,
  ...rest
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn('px-5 pb-5 sm:px-6 sm:pb-6', className)} {...rest} />
  );
}

export function SiswaCardFooter({
  className,
  ...rest
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'flex items-center gap-2 border-t-2 border-siswa-border px-5 py-3 sm:px-6',
        className,
      )}
      {...rest}
    />
  );
}
