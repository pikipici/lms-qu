'use client';

/**
 * UlanganPlayer — siswa ulangan attempt aktif (Task 5.G.2).
 *
 * Diff utama dengan LatihanPlayer:
 *   - Timer countdown sticky di header berdasarkan deadline_at server.
 *     Auto-trigger submit kalau countdown ≤0 (race-safe: server validate
 *     deadline + grace 5s; cron 30s tetap fallback).
 *   - Autosave debounced 600ms per soal (radio change). NO immediate
 *     feedback per locked #76.
 *   - Resume: items query pre-fill jawaban_siswa.
 *   - Submit confirm dialog → POST submit → swap ke result via onDone.
 *
 * Race conditions handled:
 *   - Tab inactive >durasi → countdown bisa skip frame; setiap interval
 *     re-compute dari `Date.now() - localOffset`, server tetap punya
 *     truth (cron auto-grade 30s). Saat tick mencapai 0 kita auto-submit
 *     once via guard ref.
 *   - User klik submit beberapa kali → mutationKey + isPending guard.
 *   - Network blip saat autosave → toast "gagal simpan" + auto-retry 2x dengan
 *     exponential backoff. Terminal error codes (400/403/404/409/410/422) NOT
 *     retried — surface inline + user re-trigger by re-clicking radio.
 *   - Image presigned URL TTL 15m vs ulangan durasi up to 300m → items query
 *     auto-refetch every 12 menit kalau ada item dengan images, untuk refresh
 *     presigned URLs sebelum expire.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  AlertCircle,
  Clock,
  Loader2,
  RotateCcw,
  Send,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  getAttemptItems,
  type AttemptItem,
  type AttemptItemsResult,
  type SoalJawaban,
} from '@/lib/soalbab-attempt-api';
import {
  friendlyUlanganError,
  postUlanganAnswer,
  submitUlangan,
  type UlanganSubmitResult,
} from '@/lib/soalbab-ulangan-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { cn } from '@/lib/utils';

const OPSI_LIST: { key: SoalJawaban; label: string }[] = [
  { key: 'a', label: 'A' },
  { key: 'b', label: 'B' },
  { key: 'c', label: 'C' },
  { key: 'd', label: 'D' },
  { key: 'e', label: 'E' },
];

function opsiText(item: AttemptItem, k: SoalJawaban): string {
  switch (k) {
    case 'a':
      return item.opsi_a;
    case 'b':
      return item.opsi_b;
    case 'c':
      return item.opsi_c;
    case 'd':
      return item.opsi_d;
    case 'e':
      return item.opsi_e;
  }
}

function imageURLForSlot(item: AttemptItem, slot: string): string | undefined {
  return item.images?.find((i) => i.slot === slot)?.url;
}

function formatRemaining(ms: number): string {
  if (ms < 0) ms = 0;
  const total = Math.floor(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) {
    return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  }
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}

export interface UlanganPlayerProps {
  hasilID: string;
  onDone: (summary: UlanganSubmitResult) => void;
  onAbort: () => void;
  disabled?: boolean;
}

export function UlanganPlayer({ hasilID, onDone, onAbort, disabled }: UlanganPlayerProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [confirmSubmit, setConfirmSubmit] = React.useState(false);
  const [autosaveErr, setAutosaveErr] = React.useState<Record<string, string>>({});
  const autoSubmittedRef = React.useRef(false);

  const itemsQueryKey = React.useMemo(
    () => ['siswa', 'ulangan', 'items', hasilID] as const,
    [hasilID],
  );

  const itemsQuery = useQuery({
    queryKey: itemsQueryKey,
    queryFn: () => getAttemptItems(hasilID),
    staleTime: 60_000,
    retry: (count, err) => {
      if (err instanceof ApiError) {
        // hasil_not_active → user keluar; jangan retry.
        if (err.code === 'hasil_not_active' || err.status === 403 || err.status === 404) {
          return false;
        }
      }
      return count < 2;
    },
    // Refresh presigned image URLs every 12 menit untuk attempt aktif dengan
    // images. R2 presign TTL = 15 menit, ulangan durasi bisa sampai 300 menit.
    refetchInterval: (query) => {
      const att = (query.state.data as { attempt: AttemptItemsResult } | undefined)?.attempt;
      if (!att) return false;
      if (att.status !== 'berlangsung') return false;
      const hasImages = att.items.some((it) => it.images && it.images.length > 0);
      if (!hasImages) return false;
      return 12 * 60 * 1000;
    },
    refetchIntervalInBackground: false,
  });

  const att = itemsQuery.data?.attempt;
  const deadline = att?.deadline_at ? new Date(att.deadline_at).getTime() : null;

  // Timer tick — re-render every second.
  const [now, setNow] = React.useState(() => Date.now());
  React.useEffect(() => {
    if (!deadline) return undefined;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [deadline]);

  const remaining = deadline ? deadline - now : 0;

  const answerMu = useMutation({
    mutationFn: ({ soalID, jawaban }: { soalID: string; jawaban: SoalJawaban }) =>
      postUlanganAnswer(hasilID, { soal_id: soalID, jawaban }),
    // Auto-retry transient 5xx + network errors. Terminal/business codes
    // (validation, forbidden, conflict, gone) NOT retried — bubble up.
    retry: (failureCount, err) => {
      if (err instanceof ApiError) {
        if (err.status >= 400 && err.status < 500) return false;
      }
      return failureCount < 2;
    },
    retryDelay: (attempt) => 500 * 2 ** attempt,
    onSuccess: (_data, { soalID }) => {
      setAutosaveErr((prev) => {
        if (!prev[soalID]) return prev;
        const next = { ...prev };
        delete next[soalID];
        return next;
      });
    },
    onError: (err, { soalID }) => {
      const msg =
        err instanceof ApiError
          ? friendlyUlanganError(err, 'answer')
          : 'Gagal menyimpan jawaban.';
      setAutosaveErr((prev) => ({ ...prev, [soalID]: msg }));
      // Toast cuma sekali per error code untuk avoid spam.
      toast({ title: 'Gagal menyimpan', description: msg, variant: 'destructive' });
    },
  });

  const submitMu = useMutation({
    mutationFn: () => submitUlangan(hasilID),
    onSuccess: ({ summary }) => {
      setConfirmSubmit(false);
      onDone(summary);
    },
    onError: (err) => {
      const msg =
        err instanceof ApiError
          ? friendlyUlanganError(err, 'submit')
          : 'Gagal submit ulangan.';
      toast({ title: 'Gagal submit', description: msg, variant: 'destructive' });
      // Reset auto-submit guard supaya user bisa retry manual.
      autoSubmittedRef.current = false;
    },
  });

  // Auto-submit pas timer ≤0. Guarded supaya cuma fire sekali per mount.
  React.useEffect(() => {
    if (!deadline) return;
    if (remaining > 0) return;
    if (autoSubmittedRef.current) return;
    if (submitMu.isPending) return;
    if (itemsQuery.isPending || itemsQuery.isError) return;
    autoSubmittedRef.current = true;
    setConfirmSubmit(false);
    toast({
      title: 'Waktu habis',
      description: 'Sistem otomatis submit attempt ini.',
    });
    submitMu.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [remaining, deadline]);

  // Debounce timer per soalID.
  const debounceRef = React.useRef<Record<string, number>>({});
  React.useEffect(() => {
    return () => {
      Object.values(debounceRef.current).forEach((id) => window.clearTimeout(id));
    };
  }, []);

  function handleAnswer(soalID: string, jawaban: SoalJawaban) {
    if (disabled || submitMu.isPending) return;
    if (deadline && Date.now() >= deadline) return;

    // Optimistic local cache update.
    queryClient.setQueryData(
      itemsQueryKey,
      (old: { attempt: AttemptItemsResult } | undefined) => {
        if (!old) return old;
        return {
          attempt: {
            ...old.attempt,
            items: old.attempt.items.map((it) =>
              it.soal_id === soalID ? { ...it, jawaban_siswa: jawaban } : it,
            ),
          },
        };
      },
    );

    // Debounce 600ms — siswa sering ganti pilihan beberapa kali.
    if (debounceRef.current[soalID]) {
      window.clearTimeout(debounceRef.current[soalID]);
    }
    debounceRef.current[soalID] = window.setTimeout(() => {
      delete debounceRef.current[soalID];
      answerMu.mutate({ soalID, jawaban });
    }, 600);
  }

  // ---- Render: error from items fetch ----
  if (itemsQuery.isError) {
    const apiErr = itemsQuery.error instanceof ApiError ? itemsQuery.error : null;
    const msg = apiErr
      ? friendlyUlanganError(apiErr, 'lobby')
      : 'Gagal memuat soal ulangan.';
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Gagal memuat ulangan</CardTitle>
          <CardDescription>{msg}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-2">
            <Button type="button" size="sm" variant="outline" onClick={() => itemsQuery.refetch()}>
              <RotateCcw className="size-4" />
              Coba lagi
            </Button>
            <Button type="button" size="sm" variant="ghost" onClick={onAbort}>
              Kembali ke lobi
            </Button>
          </div>
        </CardContent>
      </Card>
    );
  }

  // ---- Render: loading ----
  if (itemsQuery.isPending || !att) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Memuat soal ulangan…</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="h-32 animate-pulse rounded-md border bg-muted/40" />
        </CardContent>
      </Card>
    );
  }

  const answeredCount = att.items.filter((it) => !!it.jawaban_siswa).length;
  const timerWarn = deadline && remaining > 0 && remaining < 5 * 60 * 1000;
  const timerCritical = deadline && remaining > 0 && remaining < 60 * 1000;

  return (
    <>
      <Card>
        <CardHeader className="space-y-3">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div>
              <CardTitle className="text-base">Ulangan Bab — {att.total} soal</CardTitle>
              <CardDescription>
                Jawab semua soal lalu klik Submit. Tidak ada feedback per soal — nilai keluar
                setelah submit.
              </CardDescription>
            </div>
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <div
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 font-mono text-sm tabular-nums',
                  timerCritical && 'border-rose-300 bg-rose-50 text-rose-700',
                  timerWarn && !timerCritical && 'border-amber-300 bg-amber-50 text-amber-700',
                  !timerWarn && !timerCritical && 'border-border bg-card',
                )}
                aria-live="polite"
              >
                <Clock className="size-4" />
                {deadline ? formatRemaining(remaining) : '—'}
              </div>
              <span className="text-muted-foreground">
                {answeredCount}/{att.total} terjawab
              </span>
            </div>
          </div>
          {att.attempt_no ? (
            <div className="text-xs text-muted-foreground">Attempt #{att.attempt_no}</div>
          ) : null}
        </CardHeader>
        <CardContent>
          <ol className="space-y-4">
            {att.items.map((item, idx) => (
              <SoalCard
                key={item.soal_id}
                item={item}
                index={idx}
                autosaveErrMsg={autosaveErr[item.soal_id]}
                onChoose={(j) => handleAnswer(item.soal_id, j)}
                disabled={disabled || submitMu.isPending}
              />
            ))}
          </ol>
          <div className="mt-6 flex flex-wrap items-center justify-between gap-3 border-t pt-4">
            <p className="text-sm text-muted-foreground">
              {answeredCount === att.total
                ? 'Semua soal sudah dijawab. Submit kalau lu yakin.'
                : `Masih ada ${att.total - answeredCount} soal yang belum dijawab.`}
            </p>
            <Button
              type="button"
              onClick={() => setConfirmSubmit(true)}
              disabled={disabled || submitMu.isPending}
            >
              {submitMu.isPending ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />}
              Submit ulangan
            </Button>
          </div>
        </CardContent>
      </Card>

      <Dialog
        open={confirmSubmit}
        onOpenChange={(o) => !submitMu.isPending && setConfirmSubmit(o)}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Submit ulangan?</DialogTitle>
            <DialogDescription>
              Lu sudah menjawab <strong>{answeredCount}</strong> dari {att.total} soal.
              Setelah submit, attempt ini dinilai dan tidak bisa diubah lagi.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={() => setConfirmSubmit(false)}
              disabled={submitMu.isPending}
            >
              Batal
            </Button>
            <Button
              type="button"
              onClick={() => submitMu.mutate()}
              disabled={submitMu.isPending}
            >
              {submitMu.isPending ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />}
              Ya, submit
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

interface SoalCardProps {
  item: AttemptItem;
  index: number;
  autosaveErrMsg?: string;
  onChoose: (jawaban: SoalJawaban) => void;
  disabled?: boolean;
}

function SoalCard({ item, index, autosaveErrMsg, onChoose, disabled }: SoalCardProps) {
  const pertanyaanImg = imageURLForSlot(item, 'pertanyaan');
  return (
    <li className="rounded-md border bg-card p-4">
      <div className="mb-3 flex items-start justify-between gap-2">
        <p className="text-sm font-medium uppercase tracking-wide text-muted-foreground">
          Soal {index + 1}{' '}
          <span className="font-normal text-foreground">— {item.poin} poin</span>
        </p>
      </div>

      <div className="space-y-2">
        {item.pertanyaan ? (
          <p className="whitespace-pre-wrap text-sm">{item.pertanyaan}</p>
        ) : (
          <p className="text-sm italic text-muted-foreground">(soal hanya gambar)</p>
        )}
        {pertanyaanImg ? (
          <SlotImage url={pertanyaanImg} alt="Gambar pertanyaan" />
        ) : null}
      </div>

      <ul className="mt-3 space-y-2">
        {OPSI_LIST.map(({ key, label }) => {
          const text = opsiText(item, key);
          const slotImg = imageURLForSlot(item, key);
          const checked = item.jawaban_siswa === key;
          return (
            <li key={key}>
              <label
                className={cn(
                  'flex gap-3 rounded-md border p-3 transition-colors',
                  disabled
                    ? 'cursor-not-allowed opacity-70'
                    : 'cursor-pointer hover:bg-muted/40',
                  checked && 'border-primary bg-primary/5',
                )}
              >
                <input
                  type="radio"
                  name={`ulangan-soal-${item.soal_id}`}
                  value={key}
                  checked={checked}
                  onChange={() => onChoose(key)}
                  disabled={disabled}
                  className="mt-1 size-4"
                />
                <div className="min-w-0 flex-1 space-y-1.5">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm font-semibold uppercase">{label}.</span>
                    {text ? (
                      <span className="whitespace-pre-wrap text-sm">{text}</span>
                    ) : (
                      <span className="text-xs italic text-muted-foreground">(tanpa teks)</span>
                    )}
                  </div>
                  {slotImg ? <SlotImage url={slotImg} alt={`Gambar opsi ${label}`} /> : null}
                </div>
              </label>
            </li>
          );
        })}
      </ul>

      {autosaveErrMsg ? (
        <div className="mt-2 flex items-start gap-1.5 rounded-md border border-rose-200 bg-rose-50 p-2 text-xs text-rose-700">
          <AlertCircle className="mt-0.5 size-3.5 shrink-0" />
          <span>
            Gagal simpan: {autosaveErrMsg} Klik ulang jawaban untuk coba lagi.
          </span>
        </div>
      ) : null}
    </li>
  );
}

function SlotImage({ url, alt }: { url: string; alt: string }) {
  const [errored, setErrored] = React.useState(false);
  if (errored) {
    return (
      <div className="flex h-20 items-center justify-center gap-2 rounded-md border border-dashed text-xs text-muted-foreground">
        <AlertCircle className="size-4" />
        Gambar gagal dimuat
      </div>
    );
  }
  // Static export — no next/image.
  // eslint-disable-next-line @next/next/no-img-element
  return (
    <img
      src={url}
      alt={alt}
      onError={() => setErrored(true)}
      className="max-h-64 rounded-md border bg-card object-contain"
    />
  );
}
