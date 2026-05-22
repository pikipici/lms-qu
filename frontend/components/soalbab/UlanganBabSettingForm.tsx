/**
 * UlanganBabSettingForm — guru setting editor untuk ulangan bab.
 *
 * Backend endpoints (Task 5.C.1):
 *   GET /bab/:id/ulangan-setting
 *   PUT /bab/:id/ulangan-setting
 *
 * Bounds (locked #74):
 *   jumlah_soal:   1-200, harus ≤ pool_size
 *   durasi_menit:  1-360
 *   batas_attempt: 1-10
 *   waktu_buka_review: optional RFC3339
 *
 * Optimistic concurrency:
 *   First insert → version=0, subsequent → server-known version
 *   409 → toast minta refresh + auto invalidate query.
 */

'use client';

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Loader2, RotateCcw, Save, Settings2 } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  friendlySettingError,
  getUlanganSetting,
  upsertUlanganSetting,
  type SettingView,
  type UpsertSettingInput,
} from '@/lib/soalbab-setting-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';

const JUMLAH_MIN = 1;
const JUMLAH_MAX = 200;
const DURASI_MIN = 1;
const DURASI_MAX = 360;
const ATTEMPT_MIN = 1;
const ATTEMPT_MAX = 10;

export interface UlanganBabSettingFormProps {
  babID: string;
  disabled?: boolean;
}

interface FormState {
  jumlah_soal: string;
  durasi_menit: string;
  batas_attempt: string;
  izinkan_review_setelah_submit: boolean;
  /** datetime-local format (YYYY-MM-DDTHH:mm) atau '' */
  waktu_buka_review_local: string;
}

function viewToForm(v: SettingView): FormState {
  return {
    jumlah_soal: String(v.jumlah_soal),
    durasi_menit: String(v.durasi_menit),
    batas_attempt: String(v.batas_attempt),
    izinkan_review_setelah_submit: v.izinkan_review_setelah_submit,
    waktu_buka_review_local: v.waktu_buka_review
      ? rfc3339ToLocalInput(v.waktu_buka_review)
      : '',
  };
}

/** Convert ISO/RFC3339 → datetime-local input value (in user's local TZ). */
function rfc3339ToLocalInput(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}`
  );
}

/** Convert datetime-local input value (local TZ) → RFC3339. */
function localInputToRFC3339(local: string): string | null {
  if (!local) return null;
  const d = new Date(local);
  if (Number.isNaN(d.getTime())) return null;
  return d.toISOString();
}

export function UlanganBabSettingForm({ babID, disabled }: UlanganBabSettingFormProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const settingQuery = useQuery({
    queryKey: ['guru', 'bab', 'ulangan-setting', babID],
    queryFn: () => getUlanganSetting(babID),
    staleTime: 15_000,
  });

  const view = settingQuery.data?.setting;
  const [form, setForm] = React.useState<FormState | null>(null);
  const [errors, setErrors] = React.useState<Partial<Record<keyof FormState, string>>>({});

  // Sync form state when query data changes (initial load + post-save).
  React.useEffect(() => {
    if (view) {
      setForm(viewToForm(view));
      setErrors({});
    }
  }, [view]);

  const upsertMu = useMutation({
    mutationFn: (input: UpsertSettingInput) => upsertUlanganSetting(babID, input),
    onSuccess: ({ setting }) => {
      queryClient.setQueryData(['guru', 'bab', 'ulangan-setting', babID], { setting });
      toast({
        title: 'Setting tersimpan',
        description: setting.configured
          ? `Versi naik ke ${setting.version}.`
          : 'Setting ulangan diaktifkan.',
      });
    },
    onError: (err) => {
      const msg = err instanceof ApiError ? friendlySettingError(err) : 'Gagal menyimpan setting.';
      toast({ title: 'Gagal menyimpan', description: msg, variant: 'destructive' });
      // Version conflict → re-fetch supaya FE pegang data terbaru.
      if (err instanceof ApiError && err.code === 'version_conflict') {
        queryClient.invalidateQueries({
          queryKey: ['guru', 'bab', 'ulangan-setting', babID],
        });
      }
    },
  });

  if (settingQuery.isPending) {
    return (
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Settings2 className="size-5 text-muted-foreground" />
            <CardTitle className="text-base">Setting Ulangan</CardTitle>
          </div>
          <CardDescription>Memuat konfigurasi ulangan bab…</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="h-32 animate-pulse rounded-md border bg-muted/40" />
        </CardContent>
      </Card>
    );
  }

  if (settingQuery.isError || !view || !form) {
    const msg =
      settingQuery.error instanceof ApiError
        ? friendlySettingError(settingQuery.error)
        : 'Gagal memuat setting.';
    return (
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Settings2 className="size-5 text-muted-foreground" />
            <CardTitle className="text-base">Setting Ulangan</CardTitle>
          </div>
          <CardDescription>Tidak bisa menampilkan setting saat ini.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="rounded-md border border-destructive/50 bg-destructive/5 p-3 text-sm text-destructive">
            {msg}
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => settingQuery.refetch()}
          >
            <RotateCcw className="size-4" />
            Coba lagi
          </Button>
        </CardContent>
      </Card>
    );
  }

  const poolSize = view.pool_size;
  const jumlah = Number(form.jumlah_soal);
  const durasi = Number(form.durasi_menit);
  const attempt = Number(form.batas_attempt);

  // Bind to local const so nested function bodies retain narrowed types
  // (TS control-flow narrowing on state values doesn't propagate to
  // nested closures even when the guard ensures non-null).
  const formNonNull = form;
  const viewNonNull = view;

  const isDirty =
    formNonNull.jumlah_soal !== String(viewNonNull.jumlah_soal) ||
    formNonNull.durasi_menit !== String(viewNonNull.durasi_menit) ||
    formNonNull.batas_attempt !== String(viewNonNull.batas_attempt) ||
    formNonNull.izinkan_review_setelah_submit !== viewNonNull.izinkan_review_setelah_submit ||
    formNonNull.waktu_buka_review_local !==
      (viewNonNull.waktu_buka_review ? rfc3339ToLocalInput(viewNonNull.waktu_buka_review) : '');

  function validate(): boolean {
    const errs: Partial<Record<keyof FormState, string>> = {};
    if (!Number.isInteger(jumlah) || jumlah < JUMLAH_MIN || jumlah > JUMLAH_MAX) {
      errs.jumlah_soal = `Harus angka bulat ${JUMLAH_MIN}–${JUMLAH_MAX}.`;
    } else if (poolSize > 0 && jumlah > poolSize) {
      errs.jumlah_soal = `Maksimal ${poolSize} (jumlah soal mode ulangan tersedia).`;
    } else if (poolSize === 0) {
      errs.jumlah_soal = 'Belum ada soal mode ulangan di bab ini.';
    }
    if (!Number.isInteger(durasi) || durasi < DURASI_MIN || durasi > DURASI_MAX) {
      errs.durasi_menit = `Harus angka bulat ${DURASI_MIN}–${DURASI_MAX} menit.`;
    }
    if (!Number.isInteger(attempt) || attempt < ATTEMPT_MIN || attempt > ATTEMPT_MAX) {
      errs.batas_attempt = `Harus angka bulat ${ATTEMPT_MIN}–${ATTEMPT_MAX}.`;
    }
    if (formNonNull.waktu_buka_review_local) {
      const iso = localInputToRFC3339(formNonNull.waktu_buka_review_local);
      if (!iso) {
        errs.waktu_buka_review_local = 'Format tanggal tidak valid.';
      }
    }
    setErrors(errs);
    return Object.keys(errs).length === 0;
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (disabled || upsertMu.isPending) return;
    if (!validate()) return;

    upsertMu.mutate({
      jumlah_soal: jumlah,
      durasi_menit: durasi,
      batas_attempt: attempt,
      izinkan_review_setelah_submit: formNonNull.izinkan_review_setelah_submit,
      waktu_buka_review: formNonNull.waktu_buka_review_local
        ? localInputToRFC3339(formNonNull.waktu_buka_review_local)
        : null,
      version: viewNonNull.version,
    });
  }

  function handleReset() {
    setForm(viewToForm(viewNonNull));
    setErrors({});
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Settings2 className="size-5 text-muted-foreground" />
          <CardTitle className="text-base">Setting Ulangan</CardTitle>
        </div>
        <CardDescription>
          Konfigurasi ulangan bab: jumlah soal yang ditarik dari pool, durasi, batas attempt,
          dan opsi review setelah submit.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="rounded-md border bg-muted/30 p-3 text-sm">
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
              <span className="font-medium">Pool ulangan:</span>
              <span>
                {poolSize} soal mode ulangan/keduanya
                {poolSize === 0 ? ' (belum ada)' : ''}
              </span>
              <span className="font-medium">Status:</span>
              <span>
                {view.configured ? (
                  <span className="text-emerald-600">Aktif (versi {view.version})</span>
                ) : (
                  <span className="text-muted-foreground">Belum disetel</span>
                )}
              </span>
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-1.5">
              <Label htmlFor="jumlah_soal">
                Jumlah soal <span className="text-destructive">*</span>
              </Label>
              <Input
                id="jumlah_soal"
                type="number"
                inputMode="numeric"
                min={JUMLAH_MIN}
                max={JUMLAH_MAX}
                value={form.jumlah_soal}
                onChange={(e) => setForm({ ...form, jumlah_soal: e.target.value })}
                disabled={disabled}
                aria-invalid={Boolean(errors.jumlah_soal)}
              />
              <p className={cn('text-xs', errors.jumlah_soal ? 'text-destructive' : 'text-muted-foreground')}>
                {errors.jumlah_soal ?? `${JUMLAH_MIN}–${JUMLAH_MAX}, maks ${poolSize || '0'} (pool).`}
              </p>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="durasi_menit">
                Durasi (menit) <span className="text-destructive">*</span>
              </Label>
              <Input
                id="durasi_menit"
                type="number"
                inputMode="numeric"
                min={DURASI_MIN}
                max={DURASI_MAX}
                value={form.durasi_menit}
                onChange={(e) => setForm({ ...form, durasi_menit: e.target.value })}
                disabled={disabled}
                aria-invalid={Boolean(errors.durasi_menit)}
              />
              <p className={cn('text-xs', errors.durasi_menit ? 'text-destructive' : 'text-muted-foreground')}>
                {errors.durasi_menit ?? `${DURASI_MIN}–${DURASI_MAX} menit.`}
              </p>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="batas_attempt">
                Batas attempt <span className="text-destructive">*</span>
              </Label>
              <Input
                id="batas_attempt"
                type="number"
                inputMode="numeric"
                min={ATTEMPT_MIN}
                max={ATTEMPT_MAX}
                value={form.batas_attempt}
                onChange={(e) => setForm({ ...form, batas_attempt: e.target.value })}
                disabled={disabled}
                aria-invalid={Boolean(errors.batas_attempt)}
              />
              <p className={cn('text-xs', errors.batas_attempt ? 'text-destructive' : 'text-muted-foreground')}>
                {errors.batas_attempt ?? `${ATTEMPT_MIN}–${ATTEMPT_MAX}.`}
              </p>
            </div>
          </div>

          <div className="space-y-3">
            <label className="flex items-start gap-2 text-sm">
              <input
                type="checkbox"
                className="mt-0.5 size-4 rounded border-input"
                checked={form.izinkan_review_setelah_submit}
                onChange={(e) =>
                  setForm({ ...form, izinkan_review_setelah_submit: e.target.checked })
                }
                disabled={disabled}
              />
              <span>
                <span className="font-medium">Izinkan review setelah submit</span>
                <span className="ml-1 text-muted-foreground">
                  — siswa bisa melihat soal + jawaban benar dari attempt selesai.
                </span>
              </span>
            </label>

            <div className="space-y-1.5">
              <Label htmlFor="waktu_buka_review">Waktu buka review (opsional)</Label>
              <Input
                id="waktu_buka_review"
                type="datetime-local"
                value={form.waktu_buka_review_local}
                onChange={(e) =>
                  setForm({ ...form, waktu_buka_review_local: e.target.value })
                }
                disabled={disabled || !form.izinkan_review_setelah_submit}
                aria-invalid={Boolean(errors.waktu_buka_review_local)}
              />
              <p
                className={cn(
                  'text-xs',
                  errors.waktu_buka_review_local ? 'text-destructive' : 'text-muted-foreground',
                )}
              >
                {errors.waktu_buka_review_local ??
                  'Kosong = review langsung tersedia begitu izinkan_review_setelah_submit aktif.'}
              </p>
            </div>
          </div>

          <div className="flex flex-wrap gap-2 pt-2">
            <Button
              type="submit"
              size="sm"
              disabled={disabled || upsertMu.isPending || !isDirty}
            >
              {upsertMu.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Save className="size-4" />
              )}
              Simpan setting
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={handleReset}
              disabled={disabled || upsertMu.isPending || !isDirty}
            >
              Batal
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
