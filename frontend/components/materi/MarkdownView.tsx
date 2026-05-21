'use client';

/**
 * MarkdownView — render materi tipe='markdown' read-only untuk siswa.
 *
 * Pakai react-markdown + remark-gfm. Auto mark-as-read on mount (no
 * debounce — view dianggap intensional dari klik card materi).
 *
 * Markdown body sudah di-cap 50KB di create/update (locked #63), jadi
 * gak perlu virtualisasi. Render langsung di DOM.
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
    // markRead is stable per (materiID); fire once per materi mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [materiID]);

  if (!konten.trim()) {
    return (
      <p className="rounded-md border border-dashed p-4 text-sm italic text-muted-foreground">
        Materi ini belum berisi konten.
      </p>
    );
  }

  return (
    <div className="prose prose-sm max-w-none rounded-md border bg-background p-4 dark:prose-invert">
      <Markdown remarkPlugins={[remarkGfm]}>{konten}</Markdown>
    </div>
  );
}
