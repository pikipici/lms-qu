'use client';

/**
 * PdfUpload — drag-drop + file picker untuk PDF materi (locked #64 cap 20MB).
 *
 * FE-side validation:
 *   - Accept hanya `application/pdf` + extension `.pdf`
 *   - Cap MAX_PDF_BYTES (20MB)
 *
 * Server tetap re-validate via mime sniff (`http.DetectContentType`) +
 * size cap (locked #46). FE check hanya UX guard untuk hindari upload
 * yang bakal ditolak.
 *
 * Controlled component — parent hold `file` state, dapat callback
 * `onFileChange(file|null)`. Drag-drop highlight via `dragActive` state
 * lokal.
 */

import * as React from 'react';
import { File as FileIcon, UploadCloud, X } from 'lucide-react';

import { MAX_PDF_BYTES } from '@/lib/materi-api';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';

interface PdfUploadProps {
  file: File | null;
  onFileChange: (file: File | null) => void;
  disabled?: boolean;
  errorMessage?: string;
  id?: string;
}

export function PdfUpload({
  file,
  onFileChange,
  disabled,
  errorMessage,
  id,
}: PdfUploadProps) {
  const inputRef = React.useRef<HTMLInputElement | null>(null);
  const [dragActive, setDragActive] = React.useState(false);
  const [localError, setLocalError] = React.useState<string | null>(null);

  function handleFile(f: File | null) {
    if (!f) {
      setLocalError(null);
      onFileChange(null);
      return;
    }
    // FE-side validation. Server tetap re-validate.
    const isPdfMime = f.type === 'application/pdf' || f.type === '';
    const isPdfExt = /\.pdf$/i.test(f.name);
    if (!isPdfMime || !isPdfExt) {
      setLocalError('File harus PDF (.pdf, application/pdf).');
      onFileChange(null);
      return;
    }
    if (f.size === 0) {
      setLocalError('File kosong.');
      onFileChange(null);
      return;
    }
    if (f.size > MAX_PDF_BYTES) {
      setLocalError(
        `File melebihi batas 20 MB (${formatBytes(f.size)}). Pecah jadi beberapa bagian.`,
      );
      onFileChange(null);
      return;
    }
    setLocalError(null);
    onFileChange(f);
  }

  function onInputChange(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0] ?? null;
    handleFile(f);
    // Reset input value sehingga user bisa re-pick same filename setelah remove.
    e.target.value = '';
  }

  function onDragOver(e: React.DragEvent<HTMLLabelElement>) {
    e.preventDefault();
    e.stopPropagation();
    if (!disabled) setDragActive(true);
  }
  function onDragLeave(e: React.DragEvent<HTMLLabelElement>) {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);
  }
  function onDrop(e: React.DragEvent<HTMLLabelElement>) {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);
    if (disabled) return;
    const f = e.dataTransfer.files?.[0] ?? null;
    handleFile(f);
  }

  const errorToShow = errorMessage ?? localError;

  return (
    <div className="space-y-2">
      {!file ? (
        <label
          htmlFor={id ?? 'pdf-upload-input'}
          onDragOver={onDragOver}
          onDragLeave={onDragLeave}
          onDrop={onDrop}
          className={cn(
            'flex cursor-pointer flex-col items-center justify-center gap-2 rounded-md border-2 border-dashed bg-muted/20 px-4 py-8 text-center text-sm transition-colors',
            dragActive
              ? 'border-primary bg-primary/5 text-foreground'
              : 'border-input text-muted-foreground hover:border-primary/50 hover:bg-muted/40',
            disabled && 'cursor-not-allowed opacity-50',
          )}
        >
          <UploadCloud className="size-7" aria-hidden />
          <div className="space-y-0.5">
            <p className="font-medium text-foreground">
              Tarik PDF ke sini atau klik untuk pilih
            </p>
            <p className="text-xs">
              Maksimal {formatBytes(MAX_PDF_BYTES)} per file. Hanya .pdf.
            </p>
          </div>
          <input
            id={id ?? 'pdf-upload-input'}
            ref={inputRef}
            type="file"
            accept="application/pdf,.pdf"
            disabled={disabled}
            onChange={onInputChange}
            className="sr-only"
          />
        </label>
      ) : (
        <div className="flex items-center justify-between gap-3 rounded-md border bg-muted/20 px-3 py-2 text-sm">
          <div className="flex min-w-0 items-center gap-2">
            <FileIcon className="size-4 shrink-0 text-muted-foreground" />
            <div className="min-w-0">
              <p className="truncate font-medium" title={file.name}>
                {file.name}
              </p>
              <p className="text-xs text-muted-foreground">
                {formatBytes(file.size)}
              </p>
            </div>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => handleFile(null)}
            disabled={disabled}
            aria-label="Hapus file"
          >
            <X className="size-4" />
            Ganti
          </Button>
        </div>
      )}

      {errorToShow && (
        <p className="text-xs text-destructive">{errorToShow}</p>
      )}
    </div>
  );
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}
