'use client';

/**
 * MarkdownEditor — textarea + live preview pakai react-markdown + remark-gfm.
 *
 * Dipakai di MateriCreateDialog (tipe='markdown') + MateriEditDialog (mode
 * edit markdown). Cap 50KB sesuai locked #63 (`MAX_MARKDOWN_BYTES`).
 *
 * Layout: split-view 2 kolom di lebar >=640px (md+), stacked di mobile.
 * Tab toggle (Tulis / Pratinjau) untuk mobile UX. Char counter live.
 *
 * Keep no internal state untuk konten — controlled component (caller-owned).
 */

import * as React from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Bold, Italic, Link, List, ListOrdered } from 'lucide-react';

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
type FormatKind = 'bold' | 'italic' | 'bullet' | 'ordered' | 'link';

export function MarkdownEditor({
  value,
  onChange,
  disabled,
  errorMessage,
  rows = 12,
  id,
}: MarkdownEditorProps) {
  const [view, setView] = React.useState<ViewMode>('write');
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);
  const sizeBytes = React.useMemo(
    () => new TextEncoder().encode(value).length,
    [value],
  );
  const overLimit = sizeBytes > MAX_MARKDOWN_BYTES;

  function insertFormat(kind: FormatKind) {
    const textarea = textareaRef.current;
    if (!textarea || disabled) return;

    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const selected = value.slice(start, end);
    const next = formatSelection(kind, selected);

    onChange(`${value.slice(0, start)}${next}${value.slice(end)}`);
    window.requestAnimationFrame(() => {
      textarea.focus();
      textarea.setSelectionRange(start, start + next.length);
    });
  }

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

      <div className="flex flex-wrap items-center gap-1 rounded-md border bg-muted/25 p-1">
        <ToolbarButton label="Tebal" disabled={disabled} onClick={() => insertFormat('bold')}>
          <Bold className="size-3.5" />
        </ToolbarButton>
        <ToolbarButton label="Miring" disabled={disabled} onClick={() => insertFormat('italic')}>
          <Italic className="size-3.5" />
        </ToolbarButton>
        <ToolbarButton label="Daftar bullet" disabled={disabled} onClick={() => insertFormat('bullet')}>
          <List className="size-3.5" />
        </ToolbarButton>
        <ToolbarButton label="Daftar angka" disabled={disabled} onClick={() => insertFormat('ordered')}>
          <ListOrdered className="size-3.5" />
        </ToolbarButton>
        <ToolbarButton label="Link" disabled={disabled} onClick={() => insertFormat('link')}>
          <Link className="size-3.5" />
        </ToolbarButton>
        <span className="ml-1 text-xs text-muted-foreground">
          Pilih teks lalu klik tombol format.
        </span>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        {/* Editor */}
        <div className={cn(view === 'preview' && 'hidden md:block')}>
          <textarea
            ref={textareaRef}
            id={id}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            disabled={disabled}
            rows={rows}
            className={cn(
              'flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
              overLimit && 'border-destructive focus-visible:ring-destructive',
            )}
            placeholder={'Tulis isi di sini. Contoh:\n\nJudul bagian\n\n- Poin penting pertama\n- Poin penting kedua\n\nPakai tombol format di atas kalau perlu tebal, miring, list, atau link.'}
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
                Tulis konten di kolom sebelah kiri, lalu hasil bacanya muncul di sini.
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

function ToolbarButton({
  children,
  label,
  disabled,
  onClick,
}: {
  children: React.ReactNode;
  label: string;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="h-7 px-2"
      disabled={disabled}
      onClick={onClick}
      aria-label={label}
      title={label}
    >
      {children}
    </Button>
  );
}

function formatSelection(kind: FormatKind, selected: string): string {
  const text = selected || 'teks';
  switch (kind) {
    case 'bold':
      return `**${text}**`;
    case 'italic':
      return `_${text}_`;
    case 'bullet':
      return text
        .split('\n')
        .map((line) => `- ${line || 'poin'}`)
        .join('\n');
    case 'ordered':
      return text
        .split('\n')
        .map((line, i) => `${i + 1}. ${line || 'poin'}`)
        .join('\n');
    case 'link':
      return `[${text}](https://contoh.com)`;
  }
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}
