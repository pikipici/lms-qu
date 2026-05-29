'use client';

/**
 * Reusable Create + Edit dialog untuk bab. Mode 'create' bikin bab baru di
 * kelas; mode 'edit' kirim PATCH dengan version field (#56 optimistic
 * concurrency).
 *
 * Status field cuma muncul di mode 'edit' — saat create selalu default
 * 'draft' di backend. Pilihan status terbatas ke 'draft' / 'published'
 * (transition ke 'archived' lewat `ArchiveBabDialog` supaya user dapat
 * konfirmasi destructive).
 */

import * as React from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useMutation, useQueryClient } from '@tanstack/react-query';

import { ApiError } from '@/lib/api';
import {
  type Bab,
  type BabStatus,
  createBab,
  friendlyBabError,
  updateBab,
} from '@/lib/bab-api';
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

const babFormSchema = z.object({
  nomor: z.coerce
    .number({ invalid_type_error: 'Nomor harus angka.' })
    .int({ message: 'Nomor harus angka bulat.' })
    .min(1, { message: 'Minimal 1.' })
    .max(999, { message: 'Maksimal 999.' }),
  judul: z
    .string()
    .trim()
    .min(1, { message: 'Judul wajib diisi.' })
    .max(200, { message: 'Maksimal 200 karakter.' }),
  deskripsi: z
    .string()
    .trim()
    .max(2000, { message: 'Maksimal 2000 karakter.' }),
  status: z.enum(['draft', 'published']),
});

type BabForm = z.infer<typeof babFormSchema>;

type Mode = 'create' | 'edit';

interface BabFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: Mode;
  kelasID: string;
  /** Default nomor saran untuk mode create (`max(nomor)+1` dari list). */
  defaultNomor?: number;
  /** Required when mode='edit'. */
  bab?: Bab | null;
  /** Query keys to invalidate on success — bab list + detail. */
  invalidateKeys: readonly (readonly unknown[])[];
}

export function BabFormDialog({
  open,
  onOpenChange,
  mode,
  kelasID,
  defaultNomor,
  bab,
  invalidateKeys,
}: BabFormDialogProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const buildDefaults = React.useCallback((): BabForm => {
    if (mode === 'edit' && bab) {
      const status: BabStatus =
        bab.status === 'archived' ? 'draft' : bab.status;
      return {
        nomor: bab.nomor,
        judul: bab.judul,
        deskripsi: bab.deskripsi,
        status,
      };
    }
    return {
      nomor: defaultNomor ?? 1,
      judul: '',
      deskripsi: '',
      status: 'draft',
    };
  }, [mode, bab, defaultNomor]);

  const form = useForm<BabForm>({
    resolver: zodResolver(babFormSchema),
    defaultValues: buildDefaults(),
  });

  // Re-sync form when dialog opens or target bab changes.
  React.useEffect(() => {
    if (open) {
      form.reset(buildDefaults());
    }
  }, [open, buildDefaults, form]);

  const archived = mode === 'edit' && bab?.status === 'archived';

  const createMutation = useMutation({
    mutationFn: (input: BabForm) =>
      createBab(kelasID, {
        nomor: input.nomor,
        judul: input.judul,
        deskripsi: input.deskripsi || undefined,
      }),
    onSuccess: ({ bab: created }) => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: 'Bab dibuat',
        description: `${created.judul} (Bab ${created.nomor}) — status draft.`,
      });
      onOpenChange(false);
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyBabError(apiErr, 'create')
        : 'Gagal membuat bab.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal membuat bab',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const updateMutation = useMutation({
    mutationFn: (input: BabForm) => {
      if (!bab) throw new Error('bab is required for edit mode');
      return updateBab(bab.id, {
        version: bab.version,
        nomor: input.nomor,
        judul: input.judul,
        deskripsi: input.deskripsi,
        status: input.status,
      });
    },
    onSuccess: ({ bab: updated }) => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: 'Bab diperbarui',
        description: `${updated.judul} versi naik ke ${updated.version}.`,
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
        ? friendlyBabError(apiErr, 'update')
        : 'Gagal menyimpan perubahan bab.';
      const requestId = apiErr?.requestId;
      toast({
        title:
          apiErr?.code === 'version_conflict'
            ? 'Bab sudah berubah'
            : 'Gagal menyimpan bab',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const isPending =
    createMutation.isPending || updateMutation.isPending;

  const onSubmit = form.handleSubmit((values) => {
    if (mode === 'create') {
      createMutation.mutate(values);
    } else {
      updateMutation.mutate(values);
    }
  });

  const title = mode === 'create' ? 'Tambah bab baru' : 'Edit bab';
  const description =
    mode === 'create'
      ? 'Bab baru akan dibuat dengan status draft. Kamu bisa publikasikan setelah materi siap.'
      : `Edit detail bab. Versi saat ini: ${bab?.version ?? '-'}.`;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={onSubmit} className="space-y-4">
            <div className="grid grid-cols-[5rem_1fr] gap-3">
              <FormField
                control={form.control}
                name="nomor"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Nomor</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min={1}
                        max={999}
                        disabled={archived || isPending}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="judul"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Judul</FormLabel>
                    <FormControl>
                      <Input
                        autoFocus
                        placeholder="cth. Pengenalan Aljabar"
                        disabled={archived || isPending}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <FormField
              control={form.control}
              name="deskripsi"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Deskripsi</FormLabel>
                  <FormControl>
                    <textarea
                      className="flex min-h-[72px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                      placeholder="Catatan singkat tentang isi bab ini."
                      disabled={archived || isPending}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription className="text-xs">
                    Opsional. Maksimal 2000 karakter.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            {mode === 'edit' && (
              <FormField
                control={form.control}
                name="status"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Status</FormLabel>
                    <FormControl>
                      <select
                        className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                        disabled={archived || isPending}
                        value={field.value}
                        onChange={(e) =>
                          field.onChange(e.target.value as 'draft' | 'published')
                        }
                      >
                        <option value="draft">Draft (siswa tidak melihat)</option>
                        <option value="published">Published (siswa lihat)</option>
                      </select>
                    </FormControl>
                    <FormDescription className="text-xs">
                      Untuk mengarsipkan, pakai tombol khusus &quot;Arsipkan&quot;
                      di menu aksi.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            {archived && (
              <p className="text-xs text-muted-foreground">
                Bab ini sudah diarsipkan. Edit dinonaktifkan.
              </p>
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
              <Button
                type="submit"
                disabled={
                  isPending ||
                  archived ||
                  (mode === 'edit' && !form.formState.isDirty)
                }
              >
                {isPending
                  ? mode === 'create'
                    ? 'Membuat…'
                    : 'Menyimpan…'
                  : mode === 'create'
                    ? 'Buat bab'
                    : 'Simpan'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
