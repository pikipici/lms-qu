'use client';

/**
 * MarkdownEditor — textarea + live preview pakai react-markdown + remark-gfm.
 *
 * Dipakai di MateriCreateDialog (tipe='markdown') + MateriEditDialog (mode
 * edit markdown). Cap 50KB sesuai locked #63 (`MAX_MARKDOWN_BYTES`).
 *
 * Layout: split-view 2 kolom di lebar ≥640px (md+), stacked di mobile.
 * Tab toggle (Tulis / Pratinjau) untuk mobile UX. Char counter live.
 *
 * Keep no internal state untuk konten — controlled component (caller-owned).
 */

import * as React from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

import { MAX_MARKDOWN_BYTES } from '@/lib/materi-api';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';

interface MarkdownEditorProps {
  value: string;
  onChange: (value: string) => void;
  /** Disable input — biasanya saat mutation pending. */
  disabled?: boolean;
  /** Inline error dari parent (mis. "melebihi batas"). */
  errorMessage?: string;
  /** Min rows untuk textarea (default 12). */
  rows?: number;
  /** ID untuk wiring label/aria. */
  id?: string;
}

type ViewMode = 'write' | 'preview';

export function MarkdownEditor({
  value,
  onChange,
  disabled,
  errorMessage,
  rows = 12,
  id,
}: MarkdownEditorProps) {
  const [view, setView] = React.useState<ViewMode>('write');
  const sizeBytes = React.useMemo(
    () => new TextEncoder().encode(value).length,
    [value],
  );
  const overLimit = sizeBytes > MAX_MARKDOWN_BYTES;

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-1 rounded-md border bg-muted/40 p-0.5">
          <button
            type="button"
            onClick={() => setView('write')}
            className={cn(
              'rounded px-2 py-1 text-xs transition-colors',
              view === 'write'
                ? 'bg-background shadow-sm'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            Tulis
          </button>
          <button
            type="button"
            onClick={() => setView('preview')}
            className={cn(
              'rounded px-2 py-1 text-xs transition-colors',
              view === 'preview'
                ? 'bg-background shadow-sm'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            Pratinjau
          </button>
        </div>
        <span
          className={cn(
            'text-xs tabular-nums text-muted-foreground',
            overLimit && 'font-medium text-destructive',
          )}
        >
          {formatBytes(sizeBytes)} / {formatBytes(MAX_MARKDOWN_BYTES)}
        </span>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        {/* Editor */}
        <div className={cn(view === 'preview' && 'hidden md:block')}>
          <textarea
            id={id}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            disabled={disabled}
            rows={rows}
            className={cn(
              'flex w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
              overLimit && 'border-destructive focus-visible:ring-destructive',
            )}
            placeholder={'# Judul materi\n\nTulis isi materi pakai markdown.\n\n- Bullet list\n- **Bold**, _italic_, `code`'}
            aria-invalid={Boolean(errorMessage) || overLimit}
            spellCheck={false}
          />
        </div>

        {/* Preview */}
        <div className={cn(view === 'write' && 'hidden md:block')}>
          <div className="prose prose-sm max-w-none rounded-md border border-dashed bg-muted/20 p-3 dark:prose-invert">
            {value.trim() ? (
              <Markdown remarkPlugins={[remarkGfm]}>{value}</Markdown>
            ) : (
              <p className="text-xs italic text-muted-foreground">
                Tulis di kolom kiri — pratinjau muncul di sini.
              </p>
            )}
          </div>
        </div>
      </div>

      {errorMessage && (
        <p className="text-xs text-destructive">{errorMessage}</p>
      )}
      {!errorMessage && overLimit && (
        <p className="text-xs text-destructive">
          Konten melebihi batas {formatBytes(MAX_MARKDOWN_BYTES)}. Pendekkan
          dulu sebelum simpan.
        </p>
      )}

      {/* Mobile: tombol switch untuk preview di bawah */}
      <div className="flex justify-end md:hidden">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setView(view === 'write' ? 'preview' : 'write')}
        >
          {view === 'write' ? 'Lihat pratinjau' : 'Kembali tulis'}
        </Button>
      </div>
    </div>
  );
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}
