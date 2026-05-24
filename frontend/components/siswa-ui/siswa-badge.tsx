'use client';

import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@/lib/utils';

const badgeVariants = cva(
  'inline-flex items-center gap-1 rounded-full border-2 px-2.5 py-0.5 text-xs font-semibold leading-none',
  {
    variants: {
      tone: {
        neutral: 'bg-siswa-surface border-siswa-border text-siswa-text',
        yellow: 'bg-siswa-yellow border-siswa-border text-siswa-text',
        pink: 'bg-siswa-pink border-siswa-border text-siswa-text',
        blue: 'bg-siswa-blue border-siswa-border text-siswa-text',
        green: 'bg-siswa-green border-siswa-border text-siswa-text',
        lavender: 'bg-siswa-lavender border-siswa-border text-siswa-text',
        cream: 'bg-siswa-cream border-siswa-border text-siswa-text',
        success: 'bg-siswa-success border-siswa-border text-siswa-text',
        warning: 'bg-siswa-warning border-siswa-border text-siswa-text',
        danger: 'bg-siswa-danger border-siswa-border text-white',
      },
    },
    defaultVariants: {
      tone: 'neutral',
    },
  },
);

export interface SiswaBadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

export function SiswaBadge({
  className,
  tone,
  children,
  ...rest
}: SiswaBadgeProps) {
  return (
    <span className={cn(badgeVariants({ tone }), className)} {...rest}>
      {children}
    </span>
  );
}
