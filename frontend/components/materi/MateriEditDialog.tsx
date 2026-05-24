'use client';

/**
 * MateriEditDialog — edit dialog untuk materi yang sudah ada.
 *
 * Tipe IMMUTABLE (locked #63 → backend reject `tipe_immutable`). User cuma
 * bisa update:
 *   - Judul (semua tipe)
 *   - Konten (youtube: re-paste URL → server re-parse; markdown: edit body)
 *   - PDF: konten tidak editable di sini (replace = delete + create ulang)
 *
 * Server pakai optimistic concurrency (#56) — kirim version dari snapshot.
 * 409 version_conflict → toast + invalidate untuk refetch + form re-sync.
 */

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { FileText, Type, Youtube } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type Materi,
  MAX_MARKDOWN_BYTES,
  friendlyMateriError,
  updateMateri,
} from '@/lib/materi-api';
import {
  tryParseYouTubeID,
  youtubeWatchURL,
} from '@/lib/youtube';
import { useToast } from '@/hooks/use-toast';
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
import { YouTubeInput } from './YouTubeInput';

interface MateriEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  materi: Materi;
  invalidateKeys: readonly (readonly unknown[])[];
}

export function MateriEditDialog({
  open,
  onOpenChange,
  materi,
  invalidateKeys,
}: MateriEditDialogProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  // Initial values: untuk youtube, materi.konten = video_id 11-char.
  // Re-build watch URL supaya user bisa lihat URL aslinya di input;
  // server akan parse ulang.
  const buildInitialKonten = React.useCallback(() => {
    if (materi.tipe === 'youtube' && materi.konten) {
      return youtubeWatchURL(materi.konten);
    }
    return materi.konten ?? '';
  }, [materi.tipe, materi.konten]);

  const [judul, setJudul] = React.useState(materi.judul);
  const [konten, setKonten] = React.useState(buildInitialKonten());
  const [ytParsed, setYtParsed] = React.useState<string | null>(
    tryParseYouTubeID(buildInitialKonten()),
  );
  const [judulError, setJudulError] = React.useState<string | null>(null);

  // Re-sync setiap dialog di-open atau materi berubah (mis. abis 409 refetch).
  React.useEffect(() => {
    if (open) {
      setJudul(materi.judul);
      const initialKonten = buildInitialKonten();
      setKonten(initialKonten);
      setYtParsed(tryParseYouTubeID(initialKonten));
      setJudulError(null);
    }
  }, [open, materi.id, materi.judul, materi.konten, materi.tipe, buildInitialKonten]);

  const mutation = useMutation({
    mutationFn: () =>
      updateMateri(materi.id, {
        version: materi.version,
        judul: judul.trim(),
        konten: materi.tipe === 'pdf' ? undefined : konten,
      }),
    onSuccess: ({ materi: updated }) => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: 'Materi diperbarui',
        description: `${updated.judul} (versi ${updated.version}).`,
      });
      onOpenChange(false);
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      if (apiErr?.code === 'version_conflict') {
        for (const key of invalidateKeys) {
          queryClient.invalidateQueries({ queryKey: key });
        }
      }
      const message = apiErr
        ? friendlyMateriError(apiErr, 'update')
        : 'Gagal menyimpan perubahan.';
      const requestId = apiErr?.requestId;
      toast({
        title:
          apiErr?.code === 'version_conflict'
            ? 'Materi sudah berubah'
            : 'Gagal menyimpan materi',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const isPending = mutation.isPending;

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

    if (materi.tipe === 'youtube' && !ytParsed) {
      toast({
        title: 'URL YouTube belum valid',
        description: 'Pastikan URL bisa di-parse ke 11-char video ID.',
        variant: 'destructive',
      });
      return;
    }

    if (materi.tipe === 'markdown') {
      const sizeBytes = new TextEncoder().encode(konten).length;
      if (sizeBytes > MAX_MARKDOWN_BYTES) {
        toast({
          title: 'Konten terlalu panjang',
          description: `Isi melebihi batas ${MAX_MARKDOWN_BYTES / 1024} KB.`,
          variant: 'destructive',
        });
        return;
      }
    }

    mutation.mutate();
  }

  // Detect dirty supaya tombol Simpan disable kalau belum ada perubahan.
  const initialKonten = buildInitialKonten();
  const dirty =
    judul.trim() !== materi.judul.trim() ||
    (materi.tipe !== 'pdf' && konten !== initialKonten);

  const submitDisabled =
    isPending ||
    !judul.trim() ||
    !dirty ||
    (materi.tipe === 'youtube' && !ytParsed);

  const TipeIcon =
    materi.tipe === 'pdf'
      ? FileText
      : materi.tipe === 'youtube'
        ? Youtube
        : Type;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <TipeIcon className="size-5 text-muted-foreground" aria-hidden />
            Edit materi
          </DialogTitle>
          <DialogDescription>
            Tipe <span className="font-medium">{materi.tipe}</span> tidak bisa
            diubah. Versi saat ini: {materi.version}.
            {materi.tipe === 'pdf' &&
              ' File PDF tidak bisa di-replace di sini — hapus + buat ulang kalau perlu ganti file.'}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="materi-edit-judul">Judul</Label>
            <Input
              id="materi-edit-judul"
              value={judul}
              onChange={(e) => {
                setJudul(e.target.value);
                if (judulError) setJudulError(null);
              }}
              disabled={isPending}
              maxLength={200}
              autoFocus
              aria-invalid={!!judulError}
            />
            {judulError && (
              <p className="text-xs text-destructive">{judulError}</p>
            )}
          </div>

          {materi.tipe === 'youtube' && (
            <div className="space-y-1.5">
              <Label htmlFor="materi-edit-yt-url">URL YouTube</Label>
              <YouTubeInput
                id="materi-edit-yt-url"
                value={konten}
                onChange={setKonten}
                onParsedChange={setYtParsed}
                disabled={isPending}
              />
            </div>
          )}

          {materi.tipe === 'markdown' && (
            <div className="space-y-1.5">
              <Label htmlFor="materi-edit-markdown">Konten markdown</Label>
              <MarkdownEditor
                id="materi-edit-markdown"
                value={konten}
                onChange={setKonten}
                disabled={isPending}
              />
            </div>
          )}

          {materi.tipe === 'pdf' && materi.original_filename && (
            <div className="rounded-md border bg-muted/20 px-3 py-2 text-sm">
              <p className="text-xs uppercase tracking-wide text-muted-foreground">
                File saat ini
              </p>
              <p className="font-medium">{materi.original_filename}</p>
              {materi.size_bytes && (
                <p className="text-xs text-muted-foreground">
                  {formatBytes(materi.size_bytes)}
                </p>
              )}
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
              {isPending ? 'Menyimpan…' : 'Simpan'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}
