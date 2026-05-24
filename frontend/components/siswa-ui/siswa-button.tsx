'use client';

import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@/lib/utils';

/**
 * SiswaButton — neo-brutalist button.
 *
 * tone: solid yellow (primary), pink, blue, green, surface (outline), ghost
 * size: sm / md / lg
 *
 * Always has black border + hard shadow + press feedback. Use asChild to
 * render a Link/anchor instead of <button>.
 */
const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 font-semibold siswa-border rounded-siswa siswa-press whitespace-nowrap select-none disabled:pointer-events-none disabled:opacity-50',
  {
    variants: {
      tone: {
        primary:
          'bg-siswa-yellow text-siswa-text siswa-shadow hover:bg-siswa-yellow/95',
        pink: 'bg-siswa-pink text-siswa-text siswa-shadow hover:bg-siswa-pink/95',
        blue: 'bg-siswa-blue text-siswa-text siswa-shadow hover:bg-siswa-blue/95',
        green:
          'bg-siswa-green text-siswa-text siswa-shadow hover:bg-siswa-green/95',
        lavender:
          'bg-siswa-lavender text-siswa-text siswa-shadow hover:bg-siswa-lavender/95',
        surface:
          'bg-siswa-surface text-siswa-text siswa-shadow hover:bg-siswa-cream',
        ghost:
          'bg-transparent border-transparent shadow-none hover:bg-siswa-cream',
        danger:
          'bg-siswa-danger text-white siswa-shadow hover:bg-siswa-danger/90',
      },
      size: {
        sm: 'h-9 px-3 text-sm',
        md: 'h-11 px-4 text-sm',
        lg: 'h-12 px-5 text-base',
        icon: 'h-10 w-10 p-0',
      },
    },
    defaultVariants: {
      tone: 'primary',
      size: 'md',
    },
  },
);

export interface SiswaButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export const SiswaButton = React.forwardRef<HTMLButtonElement, SiswaButtonProps>(
  function SiswaButton(
    { className, tone, size, asChild, children, ...rest },
    ref,
  ) {
    const Comp = asChild ? Slot : 'button';
    return (
      <Comp
        ref={ref as React.Ref<HTMLButtonElement>}
        className={cn(buttonVariants({ tone, size }), className)}
        {...rest}
      >
        {children}
      </Comp>
    );
  },
);
