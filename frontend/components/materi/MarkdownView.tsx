'use client';

/**
 * MarkdownView — render materi tipe='markdown' read-only untuk siswa.
 *
 * Visual: neo-brutalism + pastel pop. react-markdown body wrapped dalam
 * siswa-border. Auto mark-as-read on mount.
 */

import * as React from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

import { useMarkMateriRead } from './useMarkMateriRead';

interface MarkdownViewProps {
  materiID: string;
  konten: string;
}

export function MarkdownView({ materiID, konten }: MarkdownViewProps) {
  const { markRead } = useMarkMateriRead(materiID);

  React.useEffect(() => {
    markRead();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [materiID]);

  if (!konten.trim()) {
    return (
      <p className="rounded-siswa border-2 border-dashed border-siswa-border-soft bg-siswa-surface/60 p-4 text-sm italic text-siswa-text-muted">
        Materi ini belum berisi konten.
      </p>
    );
  }

  return (
    <div className="prose prose-sm max-w-none rounded-siswa siswa-border bg-siswa-surface p-5 siswa-shadow-sm">
      <Markdown remarkPlugins={[remarkGfm]}>{konten}</Markdown>
    </div>
  );
}
