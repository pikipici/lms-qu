'use client';

/**
 * SoalBabEditDialog — create + edit dialog untuk soal bab (Task 5.F.1).
 *
 * Mode 'create' → POST /bab/:id/soal, mode 'edit' → PATCH /soal-bab/:id.
 * Form: pertanyaan + 5 opsi (a..e) + jawaban radio + poin + mode +
 * 6 image slots (pertanyaan + a..e). Image upload bersifat post-create
 * untuk mode 'create' — slot baru bisa di-upload setelah soal tersimpan
 * (sederhanakan UX: simpan dulu lalu kelola gambar). Untuk mode 'edit'
 * image slot bisa langsung di-upload/clear.
 *
 * Optimistic concurrency #56: kirim version, 409 → invalidate + re-sync.
 * Image swap NOT bump version (locked #78 — applies to text edits saja).
 */

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Loader2, Trash2, Upload, X } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type SoalBab,
  type SoalImageSlot,
  type SoalJawaban,
  type SoalMode,
  createSoal,
  deleteSoalImage,
  friendlySoalError,
  getSoalImageURL,
  updateSoal,
  uploadSoalImage,
} from '@/lib/soalbab-api';
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
import { cn } from '@/lib/utils';

const MAX_IMAGE_BYTES = 5 * 1024 * 1024;
const IMAGE_ACCEPT = 'image/jpeg,image/jpg,image/png,image/webp';

const MODE_OPTIONS: { value: SoalMode; label: string; hint: string }[] = [
  { value: 'keduanya', label: 'Keduanya', hint: 'Muncul di latihan & ulangan' },
  { value: 'latihan', label: 'Latihan', hint: 'Hanya formative practice' },
  { value: 'ulangan', label: 'Ulangan', hint: 'Hanya graded ulangan' },
];

const SLOTS: { slot: SoalImageSlot; label: string; descKey: 'pertanyaan' | 'opsi' }[] = [
  { slot: 'pertanyaan', label: 'Gambar Pertanyaan', descKey: 'pertanyaan' },
  { slot: 'a', label: 'Gambar A', descKey: 'opsi' },
  { slot: 'b', label: 'Gambar B', descKey: 'opsi' },
  { slot: 'c', label: 'Gambar C', descKey: 'opsi' },
  { slot: 'd', label: 'Gambar D', descKey: 'opsi' },
  { slot: 'e', label: 'Gambar E', descKey: 'opsi' },
];

function slotKey(soal: SoalBab, slot: SoalImageSlot): string | undefined {
  switch (slot) {
    case 'pertanyaan':
      return soal.pertanyaan_object_key;
    case 'a':
      return soal.opsi_a_object_key;
    case 'b':
      return soal.opsi_b_object_key;
    case 'c':
      return soal.opsi_c_object_key;
    case 'd':
      return soal.opsi_d_object_key;
    case 'e':
      return soal.opsi_e_object_key;
  }
}

interface FormState {
  pertanyaan: string;
  opsi_a: string;
  opsi_b: string;
  opsi_c: string;
  opsi_d: string;
  opsi_e: string;
  jawaban: SoalJawaban;
  poin: number;
  mode: SoalMode;
}

function initialFromSoal(s?: SoalBab | null): FormState {
  return {
    pertanyaan: s?.pertanyaan ?? '',
    opsi_a: s?.opsi_a ?? '',
    opsi_b: s?.opsi_b ?? '',
    opsi_c: s?.opsi_c ?? '',
    opsi_d: s?.opsi_d ?? '',
    opsi_e: s?.opsi_e ?? '',
    jawaban: s?.jawaban ?? 'a',
    poin: s?.poin ?? 1,
    mode: s?.mode ?? 'keduanya',
  };
}

export interface SoalBabEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  babID: string;
  /** undefined = create mode, present = edit mode */
  soal?: SoalBab | null;
  invalidateKeys: readonly (readonly unknown[])[];
}

export function SoalBabEditDialog({
  open,
  onOpenChange,
  babID,
  soal,
  invalidateKeys,
}: SoalBabEditDialogProps) {
  const isEdit = !!soal;
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const [form, setForm] = React.useState<FormState>(() => initialFromSoal(soal));
  const [errors, setErrors] = React.useState<Partial<Record<keyof FormState, string>>>({});

  // Re-sync setiap dialog di-open atau soal berubah (post-409 refetch).
  React.useEffect(() => {
    if (open) {
      setForm(initialFromSoal(soal));
      setErrors({});
    }
  }, [open, soal]);

  function validate(): boolean {
    const e: Partial<Record<keyof FormState, string>> = {};
    if (!form.pertanyaan.trim() && !soal?.pertanyaan_object_key) {
      e.pertanyaan = 'Pertanyaan wajib diisi (teks atau gambar).';
    }
    // Jawaban harus point ke opsi yang punya text atau image.
    const targetText = form[`opsi_${form.jawaban}` as keyof FormState] as string;
    const targetImg = soal ? slotKey(soal, form.jawaban) : undefined;
    if (!String(targetText).trim() && !targetImg) {
      e.jawaban = `Opsi ${form.jawaban.toUpperCase()} kosong — isi teks atau gambar dulu.`;
    }
    if (form.poin < 1 || form.poin > 100) {
      e.poin = 'Poin antara 1 sampai 100.';
    }
    setErrors(e);
    return Object.keys(e).length === 0;
  }

  const mutation = useMutation({
    mutationFn: () => {
      if (!validate()) throw new Error('validation');
      if (isEdit && soal) {
        return updateSoal(soal.id, {
          version: soal.version,
          pertanyaan: form.pertanyaan,
          opsi_a: form.opsi_a,
          opsi_b: form.opsi_b,
          opsi_c: form.opsi_c,
          opsi_d: form.opsi_d,
          opsi_e: form.opsi_e,
          jawaban: form.jawaban,
          poin: form.poin,
          mode: form.mode,
        });
      }
      return createSoal(babID, {
        pertanyaan: form.pertanyaan,
        opsi_a: form.opsi_a,
        opsi_b: form.opsi_b,
        opsi_c: form.opsi_c,
        opsi_d: form.opsi_d,
        opsi_e: form.opsi_e,
        jawaban: form.jawaban,
        poin: form.poin,
        mode: form.mode,
      });
    },
    onSuccess: () => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: isEdit ? 'Soal diperbarui' : 'Soal dibuat',
        description: isEdit
          ? 'Versi soal naik.'
          : 'Soal sudah masuk bank — bisa kelola gambar setelah ini.',
      });
      onOpenChange(false);
    },
    onError: (err) => {
      if (err instanceof Error && err.message === 'validation') return;
      const apiErr = err instanceof ApiError ? err : null;
      if (apiErr?.code === 'version_conflict') {
        for (const key of invalidateKeys) {
          queryClient.invalidateQueries({ queryKey: key });
        }
      }
      const message = apiErr
        ? friendlySoalError(apiErr, isEdit ? 'update' : 'create')
        : 'Gagal menyimpan soal.';
      toast({
        title: apiErr?.code === 'version_conflict'
          ? 'Soal sudah berubah'
          : isEdit
            ? 'Gagal menyimpan soal'
            : 'Gagal membuat soal',
        description: apiErr?.requestId
          ? `${message} (req: ${apiErr.requestId})`
          : message,
        variant: 'destructive',
      });
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEdit ? 'Edit soal' : 'Soal baru'}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? 'Edit konten soal + jawaban + mode. Kelola gambar pertanyaan/opsi di bagian Gambar.'
              : 'Tulis pertanyaan + 5 opsi + tandai jawaban benar. Setelah klik Buat soal, buka lagi soalnya untuk upload gambar.'}
          </DialogDescription>
        </DialogHeader>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
          className="space-y-5"
        >
          {/* Pertanyaan */}
          <div className="space-y-1.5">
            <Label htmlFor="pertanyaan">Pertanyaan</Label>
            <textarea
              id="pertanyaan"
              className="flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              placeholder="Tulis pertanyaan…"
              value={form.pertanyaan}
              onChange={(e) => setForm((f) => ({ ...f, pertanyaan: e.target.value }))}
              disabled={mutation.isPending}
            />
            {errors.pertanyaan && (
              <p className="text-xs text-destructive">{errors.pertanyaan}</p>
            )}
          </div>

          {/* Opsi a..e + jawaban radio */}
          <div className="space-y-2">
            <Label>Opsi & jawaban</Label>
            {(['a', 'b', 'c', 'd', 'e'] as const).map((letter) => {
              const fieldKey = `opsi_${letter}` as keyof FormState;
              const value = form[fieldKey] as string;
              const isAnswer = form.jawaban === letter;
              return (
                <div key={letter} className="flex items-start gap-2">
                  <label className="mt-2 flex items-center gap-1.5 text-sm font-medium">
                    <input
                      type="radio"
                      name="jawaban"
                      checked={isAnswer}
                      onChange={() => setForm((f) => ({ ...f, jawaban: letter }))}
                      disabled={mutation.isPending}
                      className="size-4"
                    />
                    <span className="w-5 text-center uppercase">{letter}</span>
                  </label>
                  <Input
                    value={value}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, [fieldKey]: e.target.value }))
                    }
                    placeholder={`Opsi ${letter.toUpperCase()}`}
                    disabled={mutation.isPending}
                    className={cn(isAnswer && 'border-primary')}
                  />
                </div>
              );
            })}
            {errors.jawaban && (
              <p className="text-xs text-destructive">{errors.jawaban}</p>
            )}
          </div>

          {/* Poin + mode */}
          <div className="grid grid-cols-1 sm:grid-cols-[8rem_1fr] gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="poin">Poin</Label>
              <Input
                id="poin"
                type="number"
                min={1}
                max={100}
                value={form.poin}
                onChange={(e) =>
                  setForm((f) => ({ ...f, poin: Number(e.target.value) || 0 }))
                }
                disabled={mutation.isPending}
              />
              {errors.poin && (
                <p className="text-xs text-destructive">{errors.poin}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="mode">Mode</Label>
              <select
                id="mode"
                className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                value={form.mode}
                onChange={(e) =>
                  setForm((f) => ({ ...f, mode: e.target.value as SoalMode }))
                }
                disabled={mutation.isPending}
              >
                {MODE_OPTIONS.map((m) => (
                  <option key={m.value} value={m.value}>
                    {m.label} — {m.hint}
                  </option>
                ))}
              </select>
            </div>
          </div>

          {/* Image slots — hanya tampil di edit mode */}
          {!isEdit && (
            <div className="rounded-md border border-dashed bg-muted/30 p-3 text-xs text-muted-foreground">
              Mau pakai gambar di pertanyaan atau opsi? Buat soalnya dulu, lalu buka Edit untuk upload gambar per slot.
            </div>
          )}
          {isEdit && soal && (
            <div className="space-y-2 rounded-lg border bg-muted/20 p-3">
              <Label>Gambar pertanyaan & opsi (opsional)</Label>
              <p className="text-xs text-muted-foreground">
                Upload gambar untuk pertanyaan atau opsi A-E. Maksimal 5 MB per slot, format JPG/PNG/WebP. Gambar otomatis di-resize ke 1920px.
              </p>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                {SLOTS.map((s) => (
                  <ImageSlotCard
                    key={s.slot}
                    soal={soal}
                    slot={s.slot}
                    label={s.label}
                    invalidateKeys={invalidateKeys}
                    disabled={mutation.isPending}
                  />
                ))}
              </div>
            </div>
          )}

          <DialogFooter className="gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
              disabled={mutation.isPending}
            >
              Tutup
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending && <Loader2 className="size-4 animate-spin" />}
              {isEdit ? 'Simpan perubahan' : 'Buat soal'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// ---------- Image slot subcomponent ----------

function ImageSlotCard({
  soal,
  slot,
  label,
  invalidateKeys,
  disabled,
}: {
  soal: SoalBab;
  slot: SoalImageSlot;
  label: string;
  invalidateKeys: readonly (readonly unknown[])[];
  disabled?: boolean;
}) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const fileRef = React.useRef<HTMLInputElement>(null);
  const [previewURL, setPreviewURL] = React.useState<string | null>(null);
  const [previewLoading, setPreviewLoading] = React.useState(false);
  const currentKey = slotKey(soal, slot);

  // Fetch presigned URL on mount jika ada key.
  React.useEffect(() => {
    let cancelled = false;
    if (!currentKey) {
      setPreviewURL(null);
      return;
    }
    setPreviewLoading(true);
    getSoalImageURL(soal.id, slot)
      .then((res) => {
        if (!cancelled) setPreviewURL(res.url);
      })
      .catch(() => {
        if (!cancelled) setPreviewURL(null);
      })
      .finally(() => {
        if (!cancelled) setPreviewLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [currentKey, slot, soal.id]);

  const uploadMutation = useMutation({
    mutationFn: async (file: File) => {
      if (file.size > MAX_IMAGE_BYTES) {
        throw new Error('image_too_large_client');
      }
      return uploadSoalImage({ id: soal.id, slot, file });
    },
    onSuccess: () => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({ title: `${label} berhasil di-upload` });
    },
    onError: (err) => {
      if (err instanceof Error && err.message === 'image_too_large_client') {
        toast({
          title: 'Gambar terlalu besar',
          description: 'Maksimal 5 MB.',
          variant: 'destructive',
        });
        return;
      }
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlySoalError(apiErr, 'upload-image')
        : 'Gagal mengunggah gambar.';
      toast({
        title: `Gagal upload ${label}`,
        description: apiErr?.requestId
          ? `${message} (req: ${apiErr.requestId})`
          : message,
        variant: 'destructive',
      });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteSoalImage(soal.id, slot),
    onSuccess: () => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({ title: `${label} dihapus` });
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlySoalError(apiErr, 'delete-image')
        : 'Gagal menghapus gambar.';
      toast({
        title: `Gagal hapus ${label}`,
        description: apiErr?.requestId
          ? `${message} (req: ${apiErr.requestId})`
          : message,
        variant: 'destructive',
      });
    },
  });

  const busy = uploadMutation.isPending || deleteMutation.isPending;

  return (
    <div className="rounded-md border p-2 space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium">{label}</span>
        {currentKey && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => deleteMutation.mutate()}
            disabled={disabled || busy}
            className="h-6 px-2 text-destructive hover:text-destructive"
          >
            <Trash2 className="size-3.5" />
          </Button>
        )}
      </div>
      <div className="relative h-28 w-full overflow-hidden rounded-md border-2 border-dashed bg-muted/40">
        {previewLoading ? (
          <div className="absolute inset-0 flex items-center justify-center">
            <Loader2 className="size-4 animate-spin text-muted-foreground" />
          </div>
        ) : previewURL ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={previewURL}
            alt={label}
            className="h-full w-full object-contain"
          />
        ) : (
          <div className="absolute inset-0 flex items-center justify-center text-xs text-muted-foreground">
            Tidak ada gambar
          </div>
        )}
        {busy && (
          <div className="absolute inset-0 flex items-center justify-center bg-background/80">
            <Loader2 className="size-4 animate-spin" />
          </div>
        )}
      </div>
      <input
        ref={fileRef}
        type="file"
        accept={IMAGE_ACCEPT}
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) uploadMutation.mutate(f);
          // reset input so same file can be re-selected
          if (fileRef.current) fileRef.current.value = '';
        }}
        disabled={disabled || busy}
      />
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="w-full"
        onClick={() => fileRef.current?.click()}
        disabled={disabled || busy}
      >
        <Upload className="size-3.5" />
        {currentKey ? 'Ganti' : 'Upload'}
      </Button>
    </div>
  );
}
