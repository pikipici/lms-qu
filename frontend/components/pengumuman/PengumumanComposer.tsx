'use client';

/**
 * PengumumanComposer — dialog buat bikin pengumuman baru.
 *
 * Scope auto-derived dari prop `babID`:
 *   - babID === null → pengumuman kelas-wide (di tab Pengumuman /guru/kelas/detail)
 *   - babID === <uuid> → pengumuman bab-scoped (di tab Pengumuman /guru/kelas/detail/bab)
 *
 * Tipe konten: judul + isi markdown (reuse <MarkdownEditor> dari sub-fase 3.D.1).
 * Server cap: judul 200 chars, isi 50KB. FE pre-validate untuk UX cepat.
 *
 * On success: invalidate query keys + toast + close.
 * On 409 version_conflict: tidak relevan untuk create — backend gak return code ini di POST.
 * On `kelas_archived` / `bab_not_in_kelas` → friendly toast.
 */

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';

import { ApiError } from '@/lib/api';
import {
  type CreatePengumumanBody,
  MAX_PENGUMUMAN_ISI_BYTES,
  MAX_PENGUMUMAN_JUDUL_LENGTH,
  createPengumuman,
  friendlyPengumumanError,
} from '@/lib/pengumuman-api';
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
import { MarkdownEditor } from '@/components/materi/MarkdownEditor';

export interface PengumumanComposerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kelasID: string;
  /** UUID bab tempat pengumuman nempel; null = kelas-wide. */
  babID: string | null;
  /** Query keys untuk invalidate setelah sukses. */
  invalidateKeys: readonly (readonly unknown[])[];
  /** Label scope untuk dialog description (mis. "Bab 1 — Pengantar"). */
  scopeLabel: string;
}

export function PengumumanComposer({
  open,
  onOpenChange,
  kelasID,
  babID,
  invalidateKeys,
  scopeLabel,
}: PengumumanComposerProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const [judul, setJudul] = React.useState('');
  const [isi, setIsi] = React.useState('');
  const [judulError, setJudulError] = React.useState<string | null>(null);

  // Reset state setiap dialog di-open. Hindari leftover dari sesi sebelumnya.
  React.useEffect(() => {
    if (open) {
      setJudul('');
      setIsi('');
      setJudulError(null);
    }
  }, [open]);

  const mutation = useMutation({
    mutationFn: (body: CreatePengumumanBody) => createPengumuman(kelasID, body),
    onSuccess: ({ pengumuman }) => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: 'Pengumuman dipublish',
        description: `"${pengumuman.judul}" terbit dan langsung muncul di siswa.`,
      });
      onOpenChange(false);
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyPengumumanError(apiErr, 'create')
        : 'Gagal mempublish pengumuman.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal mempublish pengumuman',
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
    if (trimmed.length > MAX_PENGUMUMAN_JUDUL_LENGTH) {
      setJudulError(
        `Judul maksimal ${MAX_PENGUMUMAN_JUDUL_LENGTH} karakter.`,
      );
      return false;
    }
    setJudulError(null);
    return true;
  }

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!validate()) return;

    const sizeBytes = new TextEncoder().encode(isi).length;
    if (sizeBytes > MAX_PENGUMUMAN_ISI_BYTES) {
      toast({
        title: 'Konten terlalu panjang',
        description: `Markdown melebihi batas ${MAX_PENGUMUMAN_ISI_BYTES / 1024} KB.`,
        variant: 'destructive',
      });
      return;
    }

    mutation.mutate({
      bab_id: babID,
      judul: judul.trim(),
      isi,
    });
  }

  const submitDisabled = isPending || !judul.trim();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Pengumuman baru</DialogTitle>
          <DialogDescription>
            Scope: {scopeLabel}. Pengumuman langsung terbit (status published)
            ke siswa enrolled. Lu bisa archive nanti kalau perlu.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="pengumuman-judul">Judul</Label>
            <Input
              id="pengumuman-judul"
              value={judul}
              onChange={(e) => {
                setJudul(e.target.value);
                if (judulError) setJudulError(null);
              }}
              disabled={isPending}
              placeholder="cth. Ulangan Bab 1 minggu depan"
              maxLength={MAX_PENGUMUMAN_JUDUL_LENGTH}
              autoFocus
              aria-invalid={!!judulError}
            />
            {judulError && (
              <p className="text-xs text-destructive">{judulError}</p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="pengumuman-isi">Isi (markdown)</Label>
            <MarkdownEditor
              id="pengumuman-isi"
              value={isi}
              onChange={setIsi}
              disabled={isPending}
              rows={8}
            />
          </div>

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
              {isPending ? 'Mempublish…' : 'Publish'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
