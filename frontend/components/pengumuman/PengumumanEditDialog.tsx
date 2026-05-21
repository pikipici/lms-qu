'use client';

/**
 * PengumumanEditDialog — edit dialog untuk pengumuman yang sudah ada.
 *
 * Bisa update judul + isi + status (published ↔ archived). Kelas/bab scope
 * tidak bisa pindah (backend gak expose endpoint move; user delete + create
 * ulang kalau perlu pindah scope).
 *
 * Server pakai optimistic concurrency (#56) — kirim version dari snapshot.
 * 409 version_conflict → toast + invalidate untuk refetch + form re-sync.
 */

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Archive, Megaphone } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type Pengumuman,
  type PengumumanStatus,
  MAX_PENGUMUMAN_ISI_BYTES,
  MAX_PENGUMUMAN_JUDUL_LENGTH,
  friendlyPengumumanError,
  updatePengumuman,
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
import { cn } from '@/lib/utils';

export interface PengumumanEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pengumuman: Pengumuman;
  invalidateKeys: readonly (readonly unknown[])[];
}

export function PengumumanEditDialog({
  open,
  onOpenChange,
  pengumuman,
  invalidateKeys,
}: PengumumanEditDialogProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const [judul, setJudul] = React.useState(pengumuman.judul);
  const [isi, setIsi] = React.useState(pengumuman.isi);
  const [status, setStatus] = React.useState<PengumumanStatus>(pengumuman.status);
  const [judulError, setJudulError] = React.useState<string | null>(null);

  // Re-sync setiap dialog di-open atau pengumuman berubah (mis. abis 409 refetch).
  React.useEffect(() => {
    if (open) {
      setJudul(pengumuman.judul);
      setIsi(pengumuman.isi);
      setStatus(pengumuman.status);
      setJudulError(null);
    }
  }, [
    open,
    pengumuman.id,
    pengumuman.judul,
    pengumuman.isi,
    pengumuman.status,
    pengumuman.version,
  ]);

  const mutation = useMutation({
    mutationFn: () => {
      // Kirim hanya field yang berubah supaya backend audit bisa decide
      // pengumuman_archived vs pengumuman_updated.
      const trimmedJudul = judul.trim();
      return updatePengumuman(pengumuman.id, {
        version: pengumuman.version,
        judul: trimmedJudul !== pengumuman.judul ? trimmedJudul : undefined,
        isi: isi !== pengumuman.isi ? isi : undefined,
        status: status !== pengumuman.status ? status : undefined,
      });
    },
    onSuccess: ({ pengumuman: updated }) => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      const archivedNow =
        updated.status === 'archived' && pengumuman.status !== 'archived';
      toast({
        title: archivedNow ? 'Pengumuman diarsipkan' : 'Pengumuman diperbarui',
        description: archivedNow
          ? `"${updated.judul}" disembunyikan dari siswa. Lu masih bisa unarchive lewat tombol di bawah.`
          : `"${updated.judul}" tersimpan (versi ${updated.version}).`,
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
        ? friendlyPengumumanError(apiErr, 'update')
        : 'Gagal menyimpan perubahan.';
      const requestId = apiErr?.requestId;
      toast({
        title:
          apiErr?.code === 'version_conflict'
            ? 'Pengumuman sudah berubah'
            : 'Gagal menyimpan pengumuman',
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
      setJudulError(`Judul maksimal ${MAX_PENGUMUMAN_JUDUL_LENGTH} karakter.`);
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

    mutation.mutate();
  }

  const dirty =
    judul.trim() !== pengumuman.judul ||
    isi !== pengumuman.isi ||
    status !== pengumuman.status;

  const submitDisabled = isPending || !judul.trim() || !dirty;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Megaphone className="size-5 text-muted-foreground" aria-hidden />
            Edit pengumuman
          </DialogTitle>
          <DialogDescription>
            Versi saat ini: {pengumuman.version}. Status archive nyembunyiin
            pengumuman dari siswa, tapi masih lu lihat di list guru.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="pengumuman-edit-judul">Judul</Label>
            <Input
              id="pengumuman-edit-judul"
              value={judul}
              onChange={(e) => {
                setJudul(e.target.value);
                if (judulError) setJudulError(null);
              }}
              disabled={isPending}
              maxLength={MAX_PENGUMUMAN_JUDUL_LENGTH}
              autoFocus
              aria-invalid={!!judulError}
            />
            {judulError && (
              <p className="text-xs text-destructive">{judulError}</p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="pengumuman-edit-isi">Isi (markdown)</Label>
            <MarkdownEditor
              id="pengumuman-edit-isi"
              value={isi}
              onChange={setIsi}
              disabled={isPending}
              rows={8}
            />
          </div>

          <div className="space-y-1.5">
            <Label>Status</Label>
            <div className="grid gap-2 sm:grid-cols-2">
              <StatusOption
                active={status === 'published'}
                onClick={() => setStatus('published')}
                disabled={isPending}
                title="Published"
                desc="Tampil ke siswa enrolled."
                Icon={Megaphone}
              />
              <StatusOption
                active={status === 'archived'}
                onClick={() => setStatus('archived')}
                disabled={isPending}
                title="Archived"
                desc="Disembunyiin dari siswa."
                Icon={Archive}
              />
            </div>
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
              {isPending ? 'Menyimpan…' : 'Simpan'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

interface StatusOptionProps {
  active: boolean;
  onClick: () => void;
  disabled?: boolean;
  title: string;
  desc: string;
  Icon: React.ComponentType<{ className?: string }>;
}

function StatusOption({
  active,
  onClick,
  disabled,
  title,
  desc,
  Icon,
}: StatusOptionProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={cn(
        'flex items-start gap-2 rounded-md border p-3 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-60',
        active
          ? 'border-primary bg-primary/5 ring-1 ring-primary'
          : 'border-input bg-background hover:bg-muted/40',
      )}
    >
      <Icon
        className={cn(
          'mt-0.5 size-4 shrink-0',
          active ? 'text-primary' : 'text-muted-foreground',
        )}
      />
      <div className="min-w-0">
        <p className="text-sm font-medium">{title}</p>
        <p className="text-xs text-muted-foreground">{desc}</p>
      </div>
    </button>
  );
}
