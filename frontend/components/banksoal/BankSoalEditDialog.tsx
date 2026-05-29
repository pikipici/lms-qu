'use client';

/**
 * BankSoalEditDialog — create + edit dialog untuk Bank Soal pribadi guru
 * (Task 6.F.1).
 *
 * Mode 'create' → POST /bank-soal, mode 'edit' → PATCH /bank-soal/:id.
 * Form: mapel + tingkat + topik (tag) + pertanyaan + 5 opsi (a..e) +
 * jawaban radio + poin + 6 image slots (pertanyaan + a..e). Image upload
 * tersedia post-create — slot baru bisa di-upload setelah soal tersimpan.
 *
 * Optimistic concurrency #56: kirim version, 409 → invalidate + re-sync.
 * Image swap NOT bump version (locked #78 — applies to text edits saja).
 *
 * Beda dari SoalBabEditDialog: drop `mode` dropdown, tambah tag fields
 * (mapel/tingkat/topik). BankSoal cross-bab tanpa `mode` (locked #84).
 */

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Loader2, Trash2, Upload } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type BankSoal,
  type BankSoalImageSlot,
  type BankSoalJawaban,
  createBankSoal,
  deleteBankSoalImage,
  friendlyBankSoalError,
  getBankSoalImageURL,
  updateBankSoal,
  uploadBankSoalImage,
} from '@/lib/banksoal-api';
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

const SLOTS: {
  slot: BankSoalImageSlot;
  label: string;
}[] = [
  { slot: 'pertanyaan', label: 'Gambar Pertanyaan' },
  { slot: 'a', label: 'Gambar A' },
  { slot: 'b', label: 'Gambar B' },
  { slot: 'c', label: 'Gambar C' },
  { slot: 'd', label: 'Gambar D' },
  { slot: 'e', label: 'Gambar E' },
];

function slotKey(soal: BankSoal, slot: BankSoalImageSlot): string | undefined {
  switch (slot) {
    case 'pertanyaan':
      return soal.pertanyaan_object_key ?? undefined;
    case 'a':
      return soal.opsi_a_object_key ?? undefined;
    case 'b':
      return soal.opsi_b_object_key ?? undefined;
    case 'c':
      return soal.opsi_c_object_key ?? undefined;
    case 'd':
      return soal.opsi_d_object_key ?? undefined;
    case 'e':
      return soal.opsi_e_object_key ?? undefined;
  }
}

interface FormState {
  mapel: string;
  tingkat: string;
  topik: string;
  pertanyaan: string;
  opsi_a: string;
  opsi_b: string;
  opsi_c: string;
  opsi_d: string;
  opsi_e: string;
  jawaban: BankSoalJawaban;
  poin: number;
}

function initialFromSoal(s?: BankSoal | null): FormState {
  return {
    mapel: s?.mapel ?? '',
    tingkat: s?.tingkat ?? '',
    topik: s?.topik ?? '',
    pertanyaan: s?.pertanyaan ?? '',
    opsi_a: s?.opsi_a ?? '',
    opsi_b: s?.opsi_b ?? '',
    opsi_c: s?.opsi_c ?? '',
    opsi_d: s?.opsi_d ?? '',
    opsi_e: s?.opsi_e ?? '',
    jawaban: s?.jawaban ?? 'a',
    poin: s?.poin ?? 1,
  };
}

export interface BankSoalEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** undefined = create mode, present = edit mode */
  soal?: BankSoal | null;
  /** Default tag values untuk create mode (e.g. dari filter aktif). */
  defaultMapel?: string;
  defaultTingkat?: string;
  defaultTopik?: string;
  invalidateKeys: readonly (readonly unknown[])[];
}

export function BankSoalEditDialog({
  open,
  onOpenChange,
  soal,
  defaultMapel,
  defaultTingkat,
  defaultTopik,
  invalidateKeys,
}: BankSoalEditDialogProps) {
  const isEdit = !!soal;
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const [form, setForm] = React.useState<FormState>(() => {
    const base = initialFromSoal(soal);
    if (!soal) {
      return {
        ...base,
        mapel: defaultMapel ?? base.mapel,
        tingkat: defaultTingkat ?? base.tingkat,
        topik: defaultTopik ?? base.topik,
      };
    }
    return base;
  });
  const [errors, setErrors] = React.useState<
    Partial<Record<keyof FormState, string>>
  >({});

  // Re-sync setiap dialog di-open atau soal berubah (post-409 refetch).
  React.useEffect(() => {
    if (open) {
      if (soal) {
        setForm(initialFromSoal(soal));
      } else {
        const base = initialFromSoal(null);
        setForm({
          ...base,
          mapel: defaultMapel ?? base.mapel,
          tingkat: defaultTingkat ?? base.tingkat,
          topik: defaultTopik ?? base.topik,
        });
      }
      setErrors({});
    }
  }, [open, soal, defaultMapel, defaultTingkat, defaultTopik]);

  function validate(): boolean {
    const e: Partial<Record<keyof FormState, string>> = {};
    if (!form.pertanyaan.trim() && !soal?.pertanyaan_object_key) {
      e.pertanyaan = 'Pertanyaan wajib diisi (teks atau gambar).';
    }
    // Jawaban harus point ke opsi yang punya text atau image.
    const targetKey = `opsi_${form.jawaban}` as
      | 'opsi_a'
      | 'opsi_b'
      | 'opsi_c'
      | 'opsi_d'
      | 'opsi_e';
    const targetText = form[targetKey];
    const targetImg = soal ? slotKey(soal, form.jawaban) : undefined;
    if (!String(targetText).trim() && !targetImg) {
      e.jawaban = `Opsi ${form.jawaban.toUpperCase()} kosong — isi teks atau gambar dulu.`;
    }
    if (form.poin < 1 || form.poin > 100) {
      e.poin = 'Poin antara 1 sampai 100.';
    }
    if (form.mapel.length > 256) e.mapel = 'Mapel maksimal 256 karakter.';
    if (form.tingkat.length > 256) e.tingkat = 'Tingkat maksimal 256 karakter.';
    if (form.topik.length > 256) e.topik = 'Topik maksimal 256 karakter.';
    setErrors(e);
    return Object.keys(e).length === 0;
  }

  const mutation = useMutation({
    mutationFn: () => {
      if (!validate()) throw new Error('validation');
      if (isEdit && soal) {
        return updateBankSoal(soal.id, {
          version: soal.version,
          mapel: form.mapel,
          tingkat: form.tingkat,
          topik: form.topik,
          pertanyaan: form.pertanyaan,
          opsi_a: form.opsi_a,
          opsi_b: form.opsi_b,
          opsi_c: form.opsi_c,
          opsi_d: form.opsi_d,
          opsi_e: form.opsi_e,
          jawaban: form.jawaban,
          poin: form.poin,
        });
      }
      return createBankSoal({
        mapel: form.mapel,
        tingkat: form.tingkat,
        topik: form.topik,
        pertanyaan: form.pertanyaan,
        opsi_a: form.opsi_a,
        opsi_b: form.opsi_b,
        opsi_c: form.opsi_c,
        opsi_d: form.opsi_d,
        opsi_e: form.opsi_e,
        jawaban: form.jawaban,
        poin: form.poin,
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
          : 'Soal masuk Bank Soal pribadi — bisa kelola gambar setelah ini.',
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
        ? friendlyBankSoalError(apiErr, isEdit ? 'update' : 'create')
        : 'Gagal menyimpan soal.';
      toast({
        title:
          apiErr?.code === 'version_conflict'
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
      <DialogContent className="flex max-h-[92svh] w-[calc(100vw-1rem)] flex-col overflow-hidden p-0 sm:max-w-3xl">
        <DialogHeader className="border-b px-4 py-4 sm:px-6">
          <DialogTitle>{isEdit ? 'Edit soal bank' : 'Soal baru'}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? 'Edit konten + tag (mapel/tingkat/topik). Kelola gambar pertanyaan/opsi di bagian Gambar.'
              : 'Bank Soal pribadi untuk Ulangan Harian. Setelah klik Buat soal, buka lagi soalnya untuk upload gambar jika perlu.'}
          </DialogDescription>
        </DialogHeader>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
          className="flex min-h-0 flex-1 flex-col"
        >
          <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-4 py-4 sm:px-6">
          {/* Tag: mapel / tingkat / topik */}
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="mapel">Mapel</Label>
              <Input
                id="mapel"
                placeholder="Matematika"
                value={form.mapel}
                onChange={(e) =>
                  setForm((f) => ({ ...f, mapel: e.target.value }))
                }
                disabled={mutation.isPending}
              />
              {errors.mapel && (
                <p className="text-xs text-destructive">{errors.mapel}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="tingkat">Tingkat</Label>
              <Input
                id="tingkat"
                placeholder="X / XI / XII"
                value={form.tingkat}
                onChange={(e) =>
                  setForm((f) => ({ ...f, tingkat: e.target.value }))
                }
                disabled={mutation.isPending}
              />
              {errors.tingkat && (
                <p className="text-xs text-destructive">{errors.tingkat}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="topik">Topik</Label>
              <Input
                id="topik"
                placeholder="Aljabar / Trigonometri"
                value={form.topik}
                onChange={(e) =>
                  setForm((f) => ({ ...f, topik: e.target.value }))
                }
                disabled={mutation.isPending}
              />
              {errors.topik && (
                <p className="text-xs text-destructive">{errors.topik}</p>
              )}
            </div>
          </div>

          {/* Pertanyaan */}
          <div className="space-y-1.5">
            <Label htmlFor="pertanyaan">Pertanyaan</Label>
            <textarea
              id="pertanyaan"
              className="flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              placeholder="Tulis pertanyaan…"
              value={form.pertanyaan}
              onChange={(e) =>
                setForm((f) => ({ ...f, pertanyaan: e.target.value }))
              }
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
                      onChange={() =>
                        setForm((f) => ({ ...f, jawaban: letter }))
                      }
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

          {/* Poin */}
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
                  setForm((f) => ({
                    ...f,
                    poin: Number(e.target.value) || 0,
                  }))
                }
                disabled={mutation.isPending}
              />
              {errors.poin && (
                <p className="text-xs text-destructive">{errors.poin}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label className="text-muted-foreground">Catatan</Label>
              <p className="rounded-md border border-dashed bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
                Bank Soal pribadi (per-guru). Soft-delete: aman dihapus
                bahkan setelah dipakai di Ulangan — hasil siswa tetap
                tersimpan.
              </p>
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

          </div>

          <DialogFooter className="gap-2 border-t px-4 py-4 sm:px-6">
            <Button
              type="button"
              variant="ghost"
              className="w-full sm:w-auto"
              onClick={() => onOpenChange(false)}
              disabled={mutation.isPending}
            >
              Tutup
            </Button>
            <Button type="submit" className="w-full sm:w-auto" disabled={mutation.isPending}>
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
  soal: BankSoal;
  slot: BankSoalImageSlot;
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
    getBankSoalImageURL(soal.id, slot)
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
      return uploadBankSoalImage({ id: soal.id, slot, file });
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
        ? friendlyBankSoalError(apiErr, 'upload-image')
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
    mutationFn: () => deleteBankSoalImage(soal.id, slot),
    onSuccess: () => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({ title: `${label} dihapus` });
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyBankSoalError(apiErr, 'delete-image')
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
          <div className="absolute inset-0 grid place-items-center text-xs text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
          </div>
        ) : previewURL ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={previewURL}
            alt={label}
            className="size-full object-contain"
          />
        ) : (
          <div className="absolute inset-0 grid place-items-center text-xs text-muted-foreground">
            (belum ada gambar)
          </div>
        )}
      </div>
      <div>
        <input
          ref={fileRef}
          type="file"
          accept={IMAGE_ACCEPT}
          className="hidden"
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file) uploadMutation.mutate(file);
            // Reset value supaya same-file re-upload tetap fire onChange.
            if (fileRef.current) fileRef.current.value = '';
          }}
          disabled={disabled || busy}
        />
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => fileRef.current?.click()}
          disabled={disabled || busy}
          className="w-full"
        >
          {busy ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            <Upload className="size-4" />
          )}
          {currentKey ? 'Ganti' : 'Upload'}
        </Button>
      </div>
    </div>
  );
}
