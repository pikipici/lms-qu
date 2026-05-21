'use client';

/**
 * Dialog duplikat untuk POST /bab/:id/duplicate. Default suffix " (Salinan)"
 * di backend (Task 3.A.4 commit fcbf532); guru bisa override `judul`.
 *
 * Hasil: bab baru status='draft', urutan=max+1, version=1, deskripsi
 * disalin. Materi/pengumuman child belum di-copy (di-defer ke Task 3.C.1 +
 * 3.F.1 setelah tabel-tabelnya ada).
 */

import * as React from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useMutation, useQueryClient } from '@tanstack/react-query';

import { ApiError } from '@/lib/api';
import { type Bab, duplicateBab, friendlyBabError } from '@/lib/bab-api';
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
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';

const duplicateSchema = z.object({
  judul: z
    .string()
    .trim()
    .max(200, { message: 'Maksimal 200 karakter.' })
    .default(''),
});

type DuplicateForm = z.infer<typeof duplicateSchema>;

interface DuplicateBabDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bab: Bab | null;
  invalidateKeys: readonly (readonly unknown[])[];
}

export function DuplicateBabDialog({
  open,
  onOpenChange,
  bab,
  invalidateKeys,
}: DuplicateBabDialogProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const form = useForm<DuplicateForm>({
    resolver: zodResolver(duplicateSchema),
    defaultValues: { judul: '' },
  });

  React.useEffect(() => {
    if (!open) form.reset({ judul: '' });
  }, [open, form]);

  const mutation = useMutation({
    mutationFn: (input: DuplicateForm) => {
      if (!bab) throw new Error('bab is required');
      return duplicateBab(bab.id, {
        judul: input.judul.trim() || undefined,
      });
    },
    onSuccess: ({ bab: dup }) => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: 'Bab berhasil diduplikasi',
        description: `${dup.judul} (Bab ${dup.nomor}) — status draft, urutan ${dup.urutan}.`,
      });
      onOpenChange(false);
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyBabError(apiErr, 'duplicate')
        : 'Gagal menduplikasi bab.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal menduplikasi bab',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const onSubmit = form.handleSubmit((values) => mutation.mutate(values));

  return (
    <Dialog open={open && !!bab} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Duplikasi bab</DialogTitle>
          <DialogDescription>
            Buat bab baru dari{' '}
            <span className="font-medium">
              {bab ? `${bab.judul} (Bab ${bab.nomor})` : ''}
            </span>
            . Status default draft, urutan ditaruh paling akhir. Materi dan
            tugas tidak ikut tersalin (Fase 3 lanjut).
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={onSubmit} className="space-y-4">
            <FormField
              control={form.control}
              name="judul"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Judul baru (opsional)</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={bab ? `${bab.judul} (Salinan)` : ''}
                      autoFocus
                      disabled={mutation.isPending}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription className="text-xs">
                    Kosongkan untuk pakai default &quot;Judul (Salinan)&quot;.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
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
                type="submit"
                disabled={mutation.isPending || !bab}
              >
                {mutation.isPending ? 'Menduplikasi…' : 'Duplikat'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
