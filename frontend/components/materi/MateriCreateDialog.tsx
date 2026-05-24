'use client';

/**
 * MateriCreateDialog — dialog 3-tipe (PDF / YouTube / Markdown) untuk
 * bikin materi baru di kelas+bab tertentu (Task 3.D.1).
 *
 * Flow:
 *   1. User pilih tipe via radio group (default: pdf).
 *   2. Field per-tipe muncul:
 *        - pdf      → <PdfUpload>
 *        - youtube  → <YouTubeInput> + live embed preview
 *        - markdown → <MarkdownEditor> split write/preview
 *   3. Submit:
 *        - pdf → multipart POST /kelas/:id/materi/upload
 *        - youtube/markdown → JSON POST /kelas/:id/materi
 *   4. Success → toast + invalidate list query + close dialog.
 *
 * Tipe immutable di backend (locked #63 → tipe_immutable). Untuk ganti
 * tipe, user harus delete + create ulang. Edit dialog hanya bisa update
 * judul + konten (lihat MateriEditDialog).
 */

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { FileText, Type, Youtube } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type CreateMateriBody,
  type Materi,
  type MateriTipe,
  MAX_MARKDOWN_BYTES,
  createMateri,
  friendlyMateriError,
  uploadMateriPDF,
} from '@/lib/materi-api';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { MarkdownEditor } from './MarkdownEditor';
import { PdfUpload } from './PdfUpload';
import { YouTubeInput } from './YouTubeInput';

interface MateriCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kelasID: string;
  /** UUID bab tempat materi nempel; null = berdiri bebas (locked #20). */
  babID: string | null;
  /** Query keys untuk invalidate setelah sukses. */
  invalidateKeys: readonly (readonly unknown[])[];
}

interface CreatableTipeOption {
  value: MateriTipe;
  label: string;
  desc: string;
  Icon: React.ComponentType<{ className?: string }>;
}

const TIPE_OPTIONS: CreatableTipeOption[] = [
  {
    value: 'pdf',
    label: 'PDF',
    desc: 'Upload file PDF (maks 20 MB).',
    Icon: FileText,
  },
  {
    value: 'youtube',
    label: 'YouTube',
    desc: 'Tempel link YouTube — auto-embed.',
    Icon: Youtube,
  },
  {
    value: 'markdown',
    label: 'Markdown',
    desc: 'Tulis teks panjang dengan format.',
    Icon: Type,
  },
];

export function MateriCreateDialog({
  open,
  onOpenChange,
  kelasID,
  babID,
  invalidateKeys,
}: MateriCreateDialogProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const [tipe, setTipe] = React.useState<MateriTipe>('pdf');
  const [judul, setJudul] = React.useState('');
  const [pdfFile, setPdfFile] = React.useState<File | null>(null);
  const [ytURL, setYtURL] = React.useState('');
  const [ytParsed, setYtParsed] = React.useState<string | null>(null);
  const [markdown, setMarkdown] = React.useState('');
  const [judulError, setJudulError] = React.useState<string | null>(null);

  // Reset semua state setiap dialog di-open. Hindari leftover dari sesi sebelumnya.
  React.useEffect(() => {
    if (open) {
      setTipe('pdf');
      setJudul('');
      setPdfFile(null);
      setYtURL('');
      setYtParsed(null);
      setMarkdown('');
      setJudulError(null);
    }
  }, [open]);

  function invalidateAll(created: Materi) {
    for (const key of invalidateKeys) {
      queryClient.invalidateQueries({ queryKey: key });
    }
    return created;
  }

  function handleError(err: unknown, action: 'create' | 'upload') {
    const apiErr = err instanceof ApiError ? err : null;
    const message = apiErr
      ? friendlyMateriError(apiErr, action)
      : 'Gagal menyimpan materi.';
    const requestId = apiErr?.requestId;
    toast({
      title: 'Gagal menyimpan materi',
      description: requestId ? `${message} (req: ${requestId})` : message,
      variant: 'destructive',
    });
  }

  const createMutation = useMutation({
    mutationFn: (body: CreateMateriBody) => createMateri(kelasID, body),
    onSuccess: ({ materi }) => {
      invalidateAll(materi);
      toast({
        title: 'Materi dibuat',
        description: `${materi.judul} (${materi.tipe}) berhasil disimpan.`,
      });
      onOpenChange(false);
    },
    onError: (err) => handleError(err, 'create'),
  });

  const uploadMutation = useMutation({
    mutationFn: (file: File) =>
      uploadMateriPDF({
        kelasID,
        babID,
        judul: judul.trim(),
        file,
      }),
    onSuccess: ({ materi }) => {
      invalidateAll(materi);
      toast({
        title: 'PDF diunggah',
        description: `${materi.judul} berhasil diunggah ke storage.`,
      });
      onOpenChange(false);
    },
    onError: (err) => handleError(err, 'upload'),
  });

  const isPending = createMutation.isPending || uploadMutation.isPending;

  function validate(): boolean {
    const trimmed = judul.trim();
    if (!trimmed) {
      setJudulError('Judul wajib diisi.');
      return false;
    }
    if (trimmed.length > 200) {
      setJudulError('Judul maksimal 200 karakter.');
      return false;
    }
    setJudulError(null);
    return true;
  }

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!validate()) return;
    const trimmedJudul = judul.trim();

    if (tipe === 'pdf') {
      if (!pdfFile) {
        toast({
          title: 'File belum dipilih',
          description: 'Pilih file PDF dulu sebelum simpan.',
          variant: 'destructive',
        });
        return;
      }
      uploadMutation.mutate(pdfFile);
      return;
    }

    if (tipe === 'youtube') {
      if (!ytParsed) {
        toast({
          title: 'URL YouTube belum valid',
          description: 'Tempel URL YouTube yang valid (11-char video ID).',
          variant: 'destructive',
        });
        return;
      }
      createMutation.mutate({
        bab_id: babID,
        judul: trimmedJudul,
        tipe: 'youtube',
        konten: ytURL.trim(),
      });
      return;
    }

    // markdown
    if (!markdown.trim()) {
      toast({
        title: 'Konten kosong',
        description: 'Tulis isi materi markdown sebelum simpan.',
        variant: 'destructive',
      });
      return;
    }
    const sizeBytes = new TextEncoder().encode(markdown).length;
    if (sizeBytes > MAX_MARKDOWN_BYTES) {
      toast({
        title: 'Konten terlalu panjang',
        description: `Isi melebihi batas ${MAX_MARKDOWN_BYTES / 1024} KB.`,
        variant: 'destructive',
      });
      return;
    }
    createMutation.mutate({
      bab_id: babID,
      judul: trimmedJudul,
      tipe: 'markdown',
      konten: markdown,
    });
  }

  // Disable submit kalau preflight tidak siap (cosmetic — final guard di onSubmit).
  const submitDisabled =
    isPending ||
    !judul.trim() ||
    (tipe === 'pdf' && !pdfFile) ||
    (tipe === 'youtube' && !ytParsed) ||
    (tipe === 'markdown' && !markdown.trim());

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Tambah materi baru</DialogTitle>
          <DialogDescription>
            Pilih tipe materi (PDF, YouTube, atau Markdown). Tipe tidak bisa
            diubah setelah dibuat — kalau perlu ganti, hapus lalu buat ulang.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={onSubmit} className="space-y-4">
          {/* Tipe radio cards */}
          <div className="space-y-1.5">
            <Label>Tipe</Label>
            <div className="grid gap-2 sm:grid-cols-3">
              {TIPE_OPTIONS.map((opt) => {
                const active = tipe === opt.value;
                return (
                  <button
                    key={opt.value}
                    type="button"
                    onClick={() => setTipe(opt.value)}
                    disabled={isPending}
                    className={cn(
                      'flex items-start gap-2 rounded-md border p-3 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-60',
                      active
                        ? 'border-primary bg-primary/5 ring-1 ring-primary'
                        : 'border-input bg-background hover:bg-muted/40',
                    )}
                  >
                    <opt.Icon
                      className={cn(
                        'size-5 shrink-0',
                        active ? 'text-primary' : 'text-muted-foreground',
                      )}
                    />
                    <div className="min-w-0">
                      <p className="text-sm font-medium">{opt.label}</p>
                      <p className="text-xs text-muted-foreground">
                        {opt.desc}
                      </p>
                    </div>
                  </button>
                );
              })}
            </div>
          </div>

          {/* Judul */}
          <div className="space-y-1.5">
            <Label htmlFor="materi-judul">Judul</Label>
            <Input
              id="materi-judul"
              value={judul}
              onChange={(e) => {
                setJudul(e.target.value);
                if (judulError) setJudulError(null);
              }}
              disabled={isPending}
              placeholder="cth. Pengantar Aljabar — Bab 1"
              maxLength={200}
              autoFocus
              aria-invalid={!!judulError}
            />
            {judulError && (
              <p className="text-xs text-destructive">{judulError}</p>
            )}
          </div>

          {/* Konten per-tipe */}
          {tipe === 'pdf' && (
            <div className="space-y-1.5">
              <Label>File PDF</Label>
              <PdfUpload
                file={pdfFile}
                onFileChange={setPdfFile}
                disabled={isPending}
              />
            </div>
          )}

          {tipe === 'youtube' && (
            <div className="space-y-1.5">
              <Label htmlFor="materi-yt-url">URL YouTube</Label>
              <YouTubeInput
                id="materi-yt-url"
                value={ytURL}
                onChange={setYtURL}
                onParsedChange={setYtParsed}
                disabled={isPending}
              />
            </div>
          )}

          {tipe === 'markdown' && (
            <div className="space-y-1.5">
              <Label htmlFor="materi-markdown">Konten markdown</Label>
              <MarkdownEditor
                id="materi-markdown"
                value={markdown}
                onChange={setMarkdown}
                disabled={isPending}
              />
            </div>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isPending}
            >
              Batal
            </Button>
            <Button type="submit" disabled={submitDisabled}>
              {isPending
                ? tipe === 'pdf'
                  ? 'Mengunggah…'
                  : 'Menyimpan…'
                : 'Simpan materi'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
