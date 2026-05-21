'use client';

/**
 * MateriViewer — dispatcher per-tipe untuk siswa render materi.
 *
 * Switch by `materi.tipe`:
 *   - pdf      → <PdfViewer> (presigned URL + iframe + debounced mark-read)
 *   - youtube  → <YouTubeEmbed> (nocookie iframe + mark-read on mount)
 *   - markdown → <MarkdownView> (react-markdown + mark-read on mount)
 *
 * Dipakai di Task 3.E.2 page `/siswa/kelas/detail/bab` Tab Materi —
 * expand-on-click pakai komponen ini sebagai body.
 *
 * Header standar: judul + badge tipe + meta (size_bytes utk pdf,
 * video_id utk yt). Body delegasi ke viewer per-tipe.
 */

import * as React from 'react';
import { FileText, Type, Youtube } from 'lucide-react';

import type { Materi, MateriTipe } from '@/lib/materi-api';
import { cn } from '@/lib/utils';
import { MarkdownView } from './MarkdownView';
import { PdfViewer } from './PdfViewer';
import { YouTubeEmbed } from './YouTubeEmbed';

interface MateriViewerProps {
  materi: Materi;
  /** Hide header card (judul + badge) kalau parent udah render-nya. */
  hideHeader?: boolean;
}

function tipeIcon(t: MateriTipe) {
  switch (t) {
    case 'pdf':
      return FileText;
    case 'youtube':
      return Youtube;
    case 'markdown':
      return Type;
  }
}

function tipeBadge(t: MateriTipe) {
  switch (t) {
    case 'pdf':
      return {
        label: 'PDF',
        class:
          'bg-rose-50 text-rose-700 dark:bg-rose-950 dark:text-rose-300',
      };
    case 'youtube':
      return {
        label: 'YouTube',
        class: 'bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300',
      };
    case 'markdown':
      return {
        label: 'Markdown',
        class: 'bg-sky-50 text-sky-700 dark:bg-sky-950 dark:text-sky-300',
      };
  }
}

function formatBytes(n: number | null | undefined): string {
  if (!n || n <= 0) return '';
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export function MateriViewer({ materi, hideHeader }: MateriViewerProps) {
  const Icon = tipeIcon(materi.tipe);
  const badge = tipeBadge(materi.tipe);

  return (
    <article className="space-y-3">
      {!hideHeader && (
        <header className="flex flex-wrap items-start justify-between gap-3 border-b pb-2">
          <div className="flex min-w-0 items-start gap-3">
            <Icon className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
            <div className="min-w-0 space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="truncate text-base font-semibold">
                  {materi.judul}
                </h3>
                <span
                  className={cn(
                    'rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
                    badge.class,
                  )}
                >
                  {badge.label}
                </span>
              </div>
              <p className="text-xs text-muted-foreground">
                {materi.tipe === 'pdf' && materi.size_bytes
                  ? formatBytes(materi.size_bytes)
                  : null}
                {materi.tipe === 'youtube' && materi.konten
                  ? `Video ID ${materi.konten}`
                  : null}
                {materi.tipe === 'markdown'
                  ? `${formatBytes(
                      new TextEncoder().encode(materi.konten).length,
                    )} markdown`
                  : null}
              </p>
            </div>
          </div>
        </header>
      )}

      <div>
        {materi.tipe === 'pdf' && (
          <PdfViewer
            materiID={materi.id}
            originalFilename={materi.original_filename}
          />
        )}
        {materi.tipe === 'youtube' && (
          <YouTubeEmbed
            materiID={materi.id}
            videoID={materi.konten}
            title={materi.judul}
          />
        )}
        {materi.tipe === 'markdown' && (
          <MarkdownView materiID={materi.id} konten={materi.konten} />
        )}
      </div>
    </article>
  );
}
