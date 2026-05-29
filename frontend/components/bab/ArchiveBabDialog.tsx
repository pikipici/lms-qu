'use client';

/**
 * Confirm dialog destructive untuk POST /bab/:id/archive.
 *
 * Idempotent guard di backend: 409 already_archived → toast informative
 * (varian non-destructive) supaya guru tau state-nya sudah berubah.
 */

import * as React from 'react';
import { Archive } from 'lucide-react';
import { useMutation, useQueryClient } from '@tanstack/react-query';

import { ApiError } from '@/lib/api';
import { type Bab, archiveBab, friendlyBabError } from '@/lib/bab-api';
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

interface ArchiveBabDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bab: Bab | null;
  invalidateKeys: readonly (readonly unknown[])[];
}

export function ArchiveBabDialog({
  open,
  onOpenChange,
  bab,
  invalidateKeys,
}: ArchiveBabDialogProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const mutation = useMutation({
    mutationFn: () => {
      if (!bab) throw new Error('bab is required');
      return archiveBab(bab.id);
    },
    onSuccess: ({ bab: archived }) => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: 'Bab diarsipkan',
        description: `${archived.judul} sudah disembunyikan dari siswa.`,
      });
      onOpenChange(false);
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyBabError(apiErr, 'archive')
        : 'Gagal mengarsipkan bab.';
      const requestId = apiErr?.requestId;
      if (apiErr?.code === 'already_archived') {
        for (const key of invalidateKeys) {
          queryClient.invalidateQueries({ queryKey: key });
        }
        toast({
          title: 'Sudah diarsipkan',
          description: requestId ? `${message} (req: ${requestId})` : message,
        });
        onOpenChange(false);
        return;
      }
      toast({
        title: 'Gagal mengarsipkan bab',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  return (
    <Dialog open={open && !!bab} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Archive className="size-4 text-destructive" />
            Arsipkan bab
          </DialogTitle>
          <DialogDescription>
            Bab{' '}
            <span className="font-medium">
              {bab ? `Bab ${bab.nomor} – ${bab.judul}` : ''}
            </span>{' '}
            akan di-set ke status <span className="font-mono">archived</span>.
            Materi/tugas yang menempel tetap ada, tapi siswa tidak akan
            melihatnya lagi. Kamu masih bisa duplikat untuk pakai ulang isinya.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={mutation.isPending}
          >
            Batal
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending || !bab}
          >
            {mutation.isPending ? 'Mengarsipkan…' : 'Arsipkan bab'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
