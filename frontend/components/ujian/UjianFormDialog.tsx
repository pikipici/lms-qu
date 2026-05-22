'use client';

/**
 * UjianFormDialog — create + edit Ujian Harian (Task 6.F.2).
 *
 * Mode 'create' → POST /kelas/:id/ujian, mode 'edit' → PATCH /ujian/:id.
 *
 * Fields:
 *   - judul + deskripsi
 *   - durasi_menit (1-360)
 *   - waktu_mulai / waktu_selesai (datetime-local optional, locked TZ user)
 *   - izinkan_review_setelah_submit toggle + waktu_buka_review optional
 *   - status: draft | published (archived dimanaged via tombol terpisah)
 *   - source: discriminated manual/random via UjianSourceConfigPanel
 *
 * Optimistic concurrency #56: PATCH bawa version + 409 invalidate +
 * re-sync via key change.
 *
 * Locked #85: source preview (POST /ujian/:id/source/preview) hanya
 * tersedia setelah ujian disimpan (preview butuh ujian id untuk auth
 * scoping). Pada create mode, panel preview disabled w/ hint.
 */

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Loader2 } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type CreateUjianInput,
  type Ujian,
  type UjianSourceConfig,
  type UjianStatus,
  type UpdateUjianInput,
  createUjian,
  friendlyUjianError,
  updateUjian,
} from '@/lib/ujian-api';
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

import { UjianSourceConfigPanel } from './UjianSourceConfigPanel';

interface FormState {
  judul: string;
  deskripsi: string;
  durasi_menit: number;
  waktu_mulai: string; // datetime-local string OR ''
  waktu_selesai: string;
  izinkan_review_setelah_submit: boolean;
  waktu_buka_review: string;
  status: UjianStatus;
  source: UjianSourceConfig | null;
}

function toLocalInputValue(rfc?: string | null): string {
  if (!rfc) return '';
  // datetime-local expects YYYY-MM-DDTHH:MM in local TZ
  const d = new Date(rfc);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function localInputToRFC3339(local: string): string | null {
  if (!local.trim()) return null;
  const d = new Date(local);
  if (Number.isNaN(d.getTime())) return null;
  return d.toISOString();
}

function initialFromUjian(u?: Ujian | null): FormState {
  const sc = u?.source_config_json && typeof u.source_config_json === 'object'
    ? (u.source_config_json as UjianSourceConfig)
    : null;
  return {
    judul: u?.judul ?? '',
    deskripsi: u?.deskripsi ?? '',
    durasi_menit: u?.durasi_menit ?? 60,
    waktu_mulai: toLocalInputValue(u?.waktu_mulai ?? null),
    waktu_selesai: toLocalInputValue(u?.waktu_selesai ?? null),
    izinkan_review_setelah_submit:
      u?.izinkan_review_setelah_submit ?? true,
    waktu_buka_review: toLocalInputValue(u?.waktu_buka_review ?? null),
    status: u?.status === 'published' ? 'published' : 'draft',
    source: sc && sc.mode ? sc : null,
  };
}

export interface UjianFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kelasID: string;
  /** undefined/null → create mode, present → edit */
  ujian?: Ujian | null;
  invalidateKeys: readonly (readonly unknown[])[];
}

export function UjianFormDialog({
  open,
  onOpenChange,
  kelasID,
  ujian,
  invalidateKeys,
}: UjianFormDialogProps) {
  const isEdit = !!ujian;
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const [form, setForm] = React.useState<FormState>(() =>
    initialFromUjian(ujian),
  );
  const [errors, setErrors] = React.useState<Record<string, string>>({});

  React.useEffect(() => {
    if (open) {
      setForm(initialFromUjian(ujian));
      setErrors({});
    }
  }, [open, ujian]);

  function validate(): boolean {
    const e: Record<string, string> = {};
    if (!form.judul.trim()) {
      e.judul = 'Judul wajib diisi.';
    } else if (form.judul.length > 200) {
      e.judul = 'Maksimal 200 karakter.';
    }
    if (form.deskripsi.length > 2000) {
      e.deskripsi = 'Maksimal 2000 karakter.';
    }
    if (form.durasi_menit < 1 || form.durasi_menit > 360) {
      e.durasi_menit = 'Durasi 1-360 menit.';
    }
    if (form.waktu_mulai && form.waktu_selesai) {
      if (
        new Date(form.waktu_mulai).getTime() >=
        new Date(form.waktu_selesai).getTime()
      ) {
        e.waktu_selesai = 'Waktu selesai harus setelah waktu mulai.';
      }
    }
    if (
      !form.izinkan_review_setelah_submit &&
      form.waktu_buka_review.trim()
    ) {
      e.waktu_buka_review =
        'Aktifkan toggle review dulu kalau mau set waktu buka.';
    }
    // Source validation: random butuh jumlah_soal > 0; manual butuh
    // setidaknya 1 soal (hindari ujian kosong).
    if (form.source) {
      if (form.source.mode === 'manual') {
        if (form.source.soal_ids.length === 0) {
          e.source = 'Pilih minimal 1 soal untuk mode manual.';
        }
      } else if (form.source.mode === 'random') {
        if (
          !form.source.jumlah_soal ||
          form.source.jumlah_soal < 1 ||
          form.source.jumlah_soal > 200
        ) {
          e.source = 'Jumlah soal random harus 1-200.';
        }
      }
    }
    setErrors(e);
    return Object.keys(e).length === 0;
  }

  const mutation = useMutation({
    mutationFn: () => {
      if (!validate()) throw new Error('validation');
      const common = {
        judul: form.judul,
        deskripsi: form.deskripsi,
        durasi_menit: form.durasi_menit,
        waktu_mulai: localInputToRFC3339(form.waktu_mulai),
        waktu_selesai: localInputToRFC3339(form.waktu_selesai),
        izinkan_review_setelah_submit:
          form.izinkan_review_setelah_submit,
        waktu_buka_review: form.izinkan_review_setelah_submit
          ? localInputToRFC3339(form.waktu_buka_review)
          : null,
        status: form.status,
      };
      if (isEdit && ujian) {
        const payload: UpdateUjianInput = {
          version: ujian.version,
          ...common,
        };
        if (form.source) payload.source = form.source;
        return updateUjian(ujian.id, payload);
      }
      const payload: CreateUjianInput = { ...common };
      if (form.source) payload.source = form.source;
      return createUjian(kelasID, payload);
    },
    onSuccess: () => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: isEdit ? 'Ujian diperbarui' : 'Ujian dibuat',
        description: isEdit
          ? 'Versi ujian naik.'
          : 'Ujian masuk daftar — set sumber soal lalu publish.',
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
        ? friendlyUjianError(apiErr, isEdit ? 'update' : 'create')
        : 'Gagal menyimpan ujian.';
      toast({
        title:
          apiErr?.code === 'version_conflict'
            ? 'Ujian sudah berubah'
            : isEdit
              ? 'Gagal menyimpan ujian'
              : 'Gagal membuat ujian',
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
          <DialogTitle>
            {isEdit ? 'Edit Ulangan Harian' : 'Ulangan Harian baru'}
          </DialogTitle>
          <DialogDescription>
            {isEdit
              ? 'Ubah judul/deskripsi/timing/sumber soal. Versi ujian akan naik.'
              : 'Buat ulangan baru untuk kelas ini. Mulai dari draft, lalu set sumber soal lalu publish ke siswa.'}
          </DialogDescription>
        </DialogHeader>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
          className="space-y-5"
        >
          {/* Judul + deskripsi */}
          <div className="space-y-1.5">
            <Label htmlFor="judul">Judul</Label>
            <Input
              id="judul"
              value={form.judul}
              onChange={(e) =>
                setForm((f) => ({ ...f, judul: e.target.value }))
              }
              disabled={mutation.isPending}
              placeholder="Ulangan Harian #1 — Aljabar"
            />
            {errors.judul && (
              <p className="text-xs text-destructive">{errors.judul}</p>
            )}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="deskripsi">Deskripsi</Label>
            <textarea
              id="deskripsi"
              className="flex min-h-[60px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              placeholder="Catatan singkat untuk siswa (opsional)"
              value={form.deskripsi}
              onChange={(e) =>
                setForm((f) => ({ ...f, deskripsi: e.target.value }))
              }
              disabled={mutation.isPending}
            />
            {errors.deskripsi && (
              <p className="text-xs text-destructive">{errors.deskripsi}</p>
            )}
          </div>

          {/* Durasi + status */}
          <div className="grid grid-cols-1 sm:grid-cols-[10rem_1fr] gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="durasi">Durasi (menit)</Label>
              <Input
                id="durasi"
                type="number"
                min={1}
                max={360}
                value={form.durasi_menit}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    durasi_menit: Number(e.target.value) || 0,
                  }))
                }
                disabled={mutation.isPending}
              />
              {errors.durasi_menit && (
                <p className="text-xs text-destructive">
                  {errors.durasi_menit}
                </p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="status">Status</Label>
              <select
                id="status"
                className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                value={form.status}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    status: e.target.value as UjianStatus,
                  }))
                }
                disabled={mutation.isPending}
              >
                <option value="draft">Draft (siswa tidak melihat)</option>
                <option value="published">
                  Published (siswa enrolled bisa start)
                </option>
              </select>
              <p className="text-xs text-muted-foreground">
                Untuk archive, pakai tombol terpisah di list.
              </p>
            </div>
          </div>

          {/* Timing */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="waktuMulai">Waktu mulai</Label>
              <Input
                id="waktuMulai"
                type="datetime-local"
                value={form.waktu_mulai}
                onChange={(e) =>
                  setForm((f) => ({ ...f, waktu_mulai: e.target.value }))
                }
                disabled={mutation.isPending}
              />
              <p className="text-xs text-muted-foreground">
                Opsional. Kosongkan supaya tersedia kapanpun.
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="waktuSelesai">Waktu selesai</Label>
              <Input
                id="waktuSelesai"
                type="datetime-local"
                value={form.waktu_selesai}
                onChange={(e) =>
                  setForm((f) => ({ ...f, waktu_selesai: e.target.value }))
                }
                disabled={mutation.isPending}
              />
              {errors.waktu_selesai && (
                <p className="text-xs text-destructive">
                  {errors.waktu_selesai}
                </p>
              )}
            </div>
          </div>

          {/* Review gating */}
          <div className="space-y-2 rounded-md border bg-muted/20 p-3">
            <div className="flex items-center gap-2">
              <input
                id="izinkanReview"
                type="checkbox"
                checked={form.izinkan_review_setelah_submit}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    izinkan_review_setelah_submit: e.target.checked,
                  }))
                }
                disabled={mutation.isPending}
                className="size-4"
              />
              <Label htmlFor="izinkanReview" className="cursor-pointer">
                Izinkan siswa review jawaban setelah submit
              </Label>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="waktuBukaReview" className="text-xs">
                Waktu buka review (opsional)
              </Label>
              <Input
                id="waktuBukaReview"
                type="datetime-local"
                value={form.waktu_buka_review}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    waktu_buka_review: e.target.value,
                  }))
                }
                disabled={
                  mutation.isPending || !form.izinkan_review_setelah_submit
                }
              />
              <p className="text-xs text-muted-foreground">
                Kalau diisi, review baru bisa dibuka siswa setelah waktu
                ini. Kosongkan untuk langsung tersedia post-submit.
              </p>
              {errors.waktu_buka_review && (
                <p className="text-xs text-destructive">
                  {errors.waktu_buka_review}
                </p>
              )}
            </div>
          </div>

          {/* Source config */}
          <div className="space-y-2 rounded-md border p-3">
            <UjianSourceConfigPanel
              ujianID={ujian?.id ?? null}
              value={form.source}
              onChange={(next) =>
                setForm((f) => ({ ...f, source: next }))
              }
              disabled={mutation.isPending}
            />
            {errors.source && (
              <p className="text-xs text-destructive">{errors.source}</p>
            )}
            {!isEdit && (
              <p className="rounded-md bg-amber-50 p-2 text-xs text-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
                Tip: simpan dulu sebagai draft, baru tombol Preview di
                panel sumber soal aktif (preview butuh ujian id untuk
                auth scoping).
              </p>
            )}
          </div>

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
              {mutation.isPending && (
                <Loader2 className="size-4 animate-spin" />
              )}
              {isEdit ? 'Simpan perubahan' : 'Buat ujian'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
