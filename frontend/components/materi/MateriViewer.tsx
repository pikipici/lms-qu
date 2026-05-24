'use client';

/**
 * MateriViewer — dispatcher per-tipe untuk siswa render materi.
 *
 * Visual: neo-brutalism + pastel pop. Header dengan icon block + tipe
 * badge berwarna pop.
 *
 * Switch by `materi.tipe`:
 *   - pdf      → <PdfViewer>
 *   - youtube  → <YouTubeEmbed>
 *   - markdown → <MarkdownView>
 */

import * as React from 'react';
import { FileText, Type, Youtube } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

import type { Materi, MateriTipe } from '@/lib/materi-api';
import { SiswaBadge } from '@/components/siswa-ui';
import { MarkdownView } from './MarkdownView';
import { PdfViewer } from './PdfViewer';
import { YouTubeEmbed } from './YouTubeEmbed';

interface MateriViewerProps {
  materi: Materi;
  /** Hide header card (judul + badge) kalau parent udah render-nya. */
  hideHeader?: boolean;
}

function tipeIcon(t: MateriTipe): LucideIcon {
  switch (t) {
    case 'pdf':
      return FileText;
    case 'youtube':
      return Youtube;
    case 'markdown':
      return Type;
  }
}

function tipeBadgeTone(
  t: MateriTipe,
): React.ComponentProps<typeof SiswaBadge>['tone'] {
  switch (t) {
    case 'pdf':
      return 'pink';
    case 'youtube':
      return 'danger';
    case 'markdown':
      return 'blue';
  }
}

function tipeBadgeLabel(t: MateriTipe): string {
  switch (t) {
    case 'pdf':
      return 'PDF';
    case 'youtube':
      return 'YouTube';
    case 'markdown':
      return 'Markdown';
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
  const tone = tipeBadgeTone(materi.tipe);
  const label = tipeBadgeLabel(materi.tipe);

  return (
    <article className="space-y-3">
      {!hideHeader ? (
        <header className="flex flex-wrap items-start justify-between gap-3 border-b-2 border-siswa-border-soft pb-3">
          <div className="flex min-w-0 items-start gap-3">
            <span className="grid size-10 shrink-0 place-items-center rounded-siswa siswa-border bg-siswa-surface">
              <Icon className="size-5" strokeWidth={2.5} />
            </span>
            <div className="min-w-0 space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="siswa-display truncate text-base font-bold">
                  {materi.judul}
                </h3>
                <SiswaBadge tone={tone}>{label}</SiswaBadge>
              </div>
              <p className="text-xs text-siswa-text-muted">
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
      ) : null}

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
