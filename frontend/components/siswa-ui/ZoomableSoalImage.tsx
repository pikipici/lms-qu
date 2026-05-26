'use client';

import * as React from 'react';
import { AlertCircle, Maximize2, Minus, Plus, RotateCcw, X } from 'lucide-react';

import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';
import { cn } from '@/lib/utils';

interface ZoomableSoalImageProps {
  url: string;
  alt: string;
  className?: string;
}

const ZOOM_STEPS = [1, 1.5, 2, 3] as const;

export function ZoomableSoalImage({ url, alt, className }: ZoomableSoalImageProps) {
  const [open, setOpen] = React.useState(false);
  const [errored, setErrored] = React.useState(false);
  const [zoomIndex, setZoomIndex] = React.useState(0);

  const zoom = ZOOM_STEPS[zoomIndex] ?? 1;

  React.useEffect(() => {
    if (!open) setZoomIndex(0);
  }, [open]);

  if (errored) {
    return (
      <div className="flex h-20 items-center justify-center gap-2 rounded-siswa border-2 border-dashed border-siswa-border-soft text-xs text-siswa-text-muted">
        <AlertCircle className="size-4" />
        Gambar gagal dimuat
      </div>
    );
  }

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="group relative inline-flex max-w-full rounded-siswa focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-siswa-yellow"
        aria-label={`Perbesar ${alt}`}
      >
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={url}
          alt={alt}
          onError={() => setErrored(true)}
          className={cn(
            'max-h-64 rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface object-contain transition-transform group-hover:-translate-y-0.5',
            className,
          )}
        />
        <span className="absolute bottom-2 right-2 inline-flex items-center gap-1 rounded-full border-2 border-siswa-border bg-siswa-surface/95 px-2 py-1 text-[11px] font-black siswa-shadow-sm opacity-90 transition-opacity group-hover:opacity-100">
          <Maximize2 className="size-3" strokeWidth={2.5} />
          Zoom
        </span>
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="siswa-theme flex h-[92vh] max-w-[96vw] grid-rows-none flex-col gap-3 overflow-hidden border-2 border-siswa-border bg-siswa-bg p-3 shadow-none sm:rounded-siswa-lg sm:p-4">
          <div className="flex flex-wrap items-center justify-between gap-2 border-b-2 border-siswa-border-soft pb-3">
            <DialogTitle className="siswa-display text-base font-black">
              Preview gambar soal
            </DialogTitle>
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => setZoomIndex((idx) => Math.max(idx - 1, 0))}
                disabled={zoomIndex === 0}
                className="rounded-siswa border-2 border-siswa-border bg-siswa-surface p-2 disabled:cursor-not-allowed disabled:opacity-40"
                aria-label="Zoom out"
              >
                <Minus className="size-4" />
              </button>
              <span className="min-w-14 rounded-siswa border-2 border-siswa-border bg-siswa-yellow px-2 py-1 text-center text-xs font-black">
                {Math.round(zoom * 100)}%
              </span>
              <button
                type="button"
                onClick={() => setZoomIndex((idx) => Math.min(idx + 1, ZOOM_STEPS.length - 1))}
                disabled={zoomIndex === ZOOM_STEPS.length - 1}
                className="rounded-siswa border-2 border-siswa-border bg-siswa-surface p-2 disabled:cursor-not-allowed disabled:opacity-40"
                aria-label="Zoom in"
              >
                <Plus className="size-4" />
              </button>
              <button
                type="button"
                onClick={() => setZoomIndex(0)}
                className="rounded-siswa border-2 border-siswa-border bg-siswa-surface p-2"
                aria-label="Reset zoom"
              >
                <RotateCcw className="size-4" />
              </button>
              <button
                type="button"
                onClick={() => setOpen(false)}
                className="rounded-siswa border-2 border-siswa-border bg-siswa-pink p-2"
                aria-label="Tutup preview"
              >
                <X className="size-4" />
              </button>
            </div>
          </div>

          <div className="min-h-0 flex-1 overflow-auto rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface p-3">
            <div className="flex min-h-full min-w-full items-center justify-center">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={url}
                alt={alt}
                className="max-h-none max-w-none origin-center rounded-siswa object-contain transition-transform"
                style={{
                  width: `${zoom * 100}%`,
                  maxWidth: zoom === 1 ? '100%' : 'none',
                }}
              />
            </div>
          </div>
          <p className="text-center text-xs font-semibold text-siswa-text-muted">
            Klik tombol + / - untuk zoom. Saat diperbesar, geser area gambar untuk melihat detail.
          </p>
        </DialogContent>
      </Dialog>
    </>
  );
}
