'use client';

/**
 * UjianPlayer — siswa ulangan attempt aktif (Task 6.G.2).
 *
 * Mirror SoalBab UlanganPlayer pattern (5.G.2 commit `6c10d19`) — single
 * Card dengan timer countdown sticky di header, autosave debounced 600ms,
 * NO immediate feedback per locked #76.
 *
 * State machine:
 *   - Loading items: skeleton.
 *   - Active: render scrollable list semua soal nomored. Per soal:
 *     pertanyaan + image presigned (max-h-64 object-contain) + 5 radio
 *     A-E + autosave inline error notice.
 *   - Submit confirm: dialog → POST submit → onDone(summary) handoff.
 *
 * Race conditions handled:
 *   - Tab inactive >durasi → countdown skip frame; setiap interval re-
 *     compute dari Date.now(); server tetap punya truth (cron 30s).
 *     Saat tick mencapai 0, auto-submit once via guard ref.
 *   - Multiple submit clicks → mutationKey + isPending guard.
 *   - Network blip autosave → toast + auto-retry 2x exp backoff.
 *     Terminal error codes (400/403/404/409/410/422) NOT retried.
 *   - R2 presigned URL TTL 15m vs ujian durasi up to 360m → items query
 *     auto-refetch every 12 menit kalau ada item dengan images, refresh
 *     presigned URLs sebelum expire.
 *   - Auto-submit guard: kalau timer expired sambil submitMu masih
 *     pending atau itemsQuery masih loading, skip frame; re-evaluate
 *     next tick.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  Clock,
  ListChecks,
  Loader2,
  RotateCcw,
  Send,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  friendlySiswaUjianError,
  getSiswaUjianItems,
  postSiswaUjianAnswer,
  submitSiswaUjian,
  type UjianAttemptItem,
  type UjianAttemptItemsResult,
  type UjianSoalJawaban,
  type UjianSubmitResult,
} from '@/lib/siswa-ujian-api';
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
import { cn } from '@/lib/utils';
import {
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardDescription,
  SiswaCardHeader,
  SiswaCardTitle,
  ZoomableSoalImage,
} from '@/components/siswa-ui';

const OPSI_LIST: { key: UjianSoalJawaban; label: string }[] = [
  { key: 'a', label: 'A' },
  { key: 'b', label: 'B' },
  { key: 'c', label: 'C' },
  { key: 'd', label: 'D' },
  { key: 'e', label: 'E' },
];

function opsiText(item: UjianAttemptItem, k: UjianSoalJawaban): string {
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

function imageURLForSlot(item: UjianAttemptItem, slot: string): string | undefined {
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

export interface UjianPlayerProps {
  hasilID: string;
  onDone: (summary: UjianSubmitResult) => void;
  onAbort: () => void;
  disabled?: boolean;
}

export function UjianPlayer({
  hasilID,
  onDone,
  onAbort,
  disabled,
}: UjianPlayerProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [confirmSubmit, setConfirmSubmit] = React.useState(false);
  const [questionMapOpen, setQuestionMapOpen] = React.useState(false);
  const [activeIndex, setActiveIndex] = React.useState(0);
  const [autosaveErr, setAutosaveErr] = React.useState<Record<string, string>>(
    {},
  );
  const autoSubmittedRef = React.useRef(false);

  const itemsQueryKey = React.useMemo(
    () => ['siswa', 'ujian', 'items', hasilID] as const,
    [hasilID],
  );

  const itemsQuery = useQuery({
    queryKey: itemsQueryKey,
    queryFn: () => getSiswaUjianItems(hasilID),
    staleTime: 60_000,
    retry: (count, err) => {
      if (err instanceof ApiError) {
        if (
          err.code === 'hasil_not_active' ||
          err.status === 403 ||
          err.status === 404
        ) {
          return false;
        }
      }
      return count < 2;
    },
    // Refresh presigned image URLs every 12 menit untuk attempt aktif
    // dengan images. R2 presign TTL = 15 menit; ujian durasi up to 360m.
    refetchInterval: (query) => {
      const att = query.state.data as UjianAttemptItemsResult | undefined;
      if (!att) return false;
      if (att.status !== 'berlangsung') return false;
      const hasImages = att.items.some(
        (it) => it.images && it.images.length > 0,
      );
      if (!hasImages) return false;
      return 12 * 60 * 1000;
    },
    refetchIntervalInBackground: false,
  });

  const att = itemsQuery.data;
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
    mutationFn: ({
      soalID,
      jawaban,
    }: {
      soalID: string;
      jawaban: UjianSoalJawaban;
    }) => postSiswaUjianAnswer(hasilID, { soal_id: soalID, jawaban }),
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
          ? friendlySiswaUjianError(err, 'answer')
          : 'Gagal menyimpan jawaban.';
      setAutosaveErr((prev) => ({ ...prev, [soalID]: msg }));
      toast({
        title: 'Gagal menyimpan',
        description: msg,
        variant: 'destructive',
      });
    },
  });

  const submitMu = useMutation({
    mutationFn: () => submitSiswaUjian(hasilID),
    onSuccess: ({ summary }) => {
      setConfirmSubmit(false);
      onDone(summary);
    },
    onError: (err) => {
      const msg =
        err instanceof ApiError
          ? friendlySiswaUjianError(err, 'submit')
          : 'Gagal submit ujian.';
      toast({
        title: 'Gagal submit',
        description: msg,
        variant: 'destructive',
      });
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
      description: 'Sistem otomatis submit jawaban ini.',
    });
    submitMu.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [remaining, deadline]);

  // Debounce timer per soalID.
  const debounceRef = React.useRef<Record<string, number>>({});
  React.useEffect(() => {
    const timers = debounceRef.current;
    return () => {
      Object.values(timers).forEach((id) => window.clearTimeout(id));
    };
  }, []);

  function handleAnswer(soalID: string, jawaban: UjianSoalJawaban) {
    if (disabled || submitMu.isPending) return;
    if (deadline && Date.now() >= deadline) return;

    // Optimistic local cache update.
    queryClient.setQueryData(
      itemsQueryKey,
      (old: UjianAttemptItemsResult | undefined) => {
        if (!old) return old;
        return {
          ...old,
          items: old.items.map((it) =>
            it.soal_id === soalID ? { ...it, jawaban_siswa: jawaban } : it,
          ),
        };
      },
    );

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
    const apiErr =
      itemsQuery.error instanceof ApiError ? itemsQuery.error : null;
    const msg = apiErr
      ? friendlySiswaUjianError(apiErr, 'items')
      : 'Gagal memuat soal ujian.';
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>Gagal memuat ujian</SiswaCardTitle>
          <SiswaCardDescription>{msg}</SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          <div className="flex flex-wrap gap-2">
            <SiswaButton
              type="button"
              size="sm"
              tone="surface"
              onClick={() => itemsQuery.refetch()}
            >
              <RotateCcw className="size-4" />
              Coba lagi
            </SiswaButton>
            <SiswaButton type="button" size="sm" tone="ghost" onClick={onAbort}>
              Kembali ke daftar
            </SiswaButton>
          </div>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  // ---- Render: loading ----
  if (itemsQuery.isPending || !att) {
    return (
      <SiswaCard tone="surface" shadow="sm">
        <SiswaCardHeader>
          <SiswaCardTitle>Memuat soal ujian…</SiswaCardTitle>
        </SiswaCardHeader>
        <SiswaCardBody>
          <div className="h-32 animate-pulse rounded-siswa siswa-border bg-siswa-surface/60" />
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  const items = att.items;
  const totalQuestions = att.total;
  const answeredCount = items.filter((it) => !!it.jawaban_siswa).length;
  const firstUnansweredIndex = items.findIndex((it) => !it.jawaban_siswa);
  const allAnswered = items.length > 0 && firstUnansweredIndex === -1;
  const safeActiveIndex = Math.min(activeIndex, Math.max(items.length - 1, 0));
  const currentItem = items[safeActiveIndex];
  const isFirstQuestion = safeActiveIndex === 0;
  const timerWarn = deadline && remaining > 0 && remaining < 5 * 60 * 1000;
  const timerCritical = deadline && remaining > 0 && remaining < 60 * 1000;

  function openManualSubmitConfirm() {
    if (disabled || submitMu.isPending) return;
    if (!allAnswered) {
      const targetIndex = firstUnansweredIndex >= 0 ? firstUnansweredIndex : safeActiveIndex;
      setActiveIndex(targetIndex);
      toast({
        title: 'Masih ada soal kosong',
        description: `Lengkapi soal nomor ${targetIndex + 1} sebelum submit ujian.`,
        variant: 'destructive',
      });
      return;
    }
    setConfirmSubmit(true);
  }

  function handleManualSubmit() {
    if (!allAnswered) {
      openManualSubmitConfirm();
      return;
    }
    submitMu.mutate();
  }

  function handlePrimaryNavigation() {
    if (disabled) return;
    if (submitMu.isPending) return;
    if (allAnswered) {
      openManualSubmitConfirm();
      return;
    }
    const nextUnansweredIndex = items.findIndex(
      (it, idx) => idx > safeActiveIndex && !it.jawaban_siswa,
    );
    if (nextUnansweredIndex >= 0) {
      setActiveIndex(nextUnansweredIndex);
      return;
    }
    if (firstUnansweredIndex >= 0) {
      setActiveIndex(firstUnansweredIndex);
    }
  }

  return (
    <>
      <SiswaCard tone="ulangan" shadow="md">
        <SiswaCardHeader className="space-y-3">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div>
              <SiswaCardTitle>
                Ulangan Harian — {totalQuestions} soal
              </SiswaCardTitle>
              <SiswaCardDescription>
                Jawab semua soal lalu klik Submit. Tidak ada feedback per soal —
                nilai keluar setelah submit.
              </SiswaCardDescription>
            </div>
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <div
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-siswa border-2 border-siswa-border bg-siswa-surface px-3 py-1.5 font-mono text-sm font-bold tabular-nums siswa-shadow-sm',
                  timerCritical && 'bg-rose-200 text-rose-900',
                  timerWarn && !timerCritical && 'bg-siswa-yellow text-siswa-text',
                )}
                aria-live="polite"
              >
                <Clock className="size-4" strokeWidth={2.5} />
                {deadline ? formatRemaining(remaining) : '—'}
              </div>
              <span className="font-semibold text-siswa-text-muted">
                {answeredCount}/{totalQuestions} terjawab
              </span>
            </div>
          </div>
          {att.attempt_no ? (
            <div className="text-xs font-semibold text-siswa-text-muted">
              Kesempatan #{att.attempt_no}
            </div>
          ) : null}
        </SiswaCardHeader>
        <SiswaCardBody className="pt-2 sm:pt-3">
          {currentItem ? (
            <SoalCard
              key={currentItem.soal_id}
              item={currentItem}
              index={safeActiveIndex}
              autosaveErrMsg={autosaveErr[currentItem.soal_id]}
              onChoose={(j) => handleAnswer(currentItem.soal_id, j)}
              disabled={disabled || submitMu.isPending}
            />
          ) : null}

          <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-siswa border-2 border-siswa-border-soft bg-siswa-cream/30 p-3">
            <SiswaButton
              type="button"
              tone="ghost"
              size="sm"
              onClick={() => setActiveIndex((idx) => Math.max(idx - 1, 0))}
              disabled={isFirstQuestion || submitMu.isPending}
            >
              <ChevronLeft className="size-4" strokeWidth={2.5} />
              Kembali
            </SiswaButton>
            <SiswaButton
              type="button"
              tone="surface"
              size="sm"
              onClick={() => setQuestionMapOpen(true)}
            >
              <ListChecks className="size-4" strokeWidth={2.5} />
              Nomor soal
            </SiswaButton>
            <SiswaButton
              type="button"
              tone="primary"
              size="sm"
              onClick={handlePrimaryNavigation}
              disabled={disabled || submitMu.isPending}
            >
              {allAnswered ? (
                <>
                  {submitMu.isPending ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <Send className="size-4" strokeWidth={2.5} />
                  )}
                  Submit ujian
                </>
              ) : (
                <>
                  Lanjut
                  <ChevronRight className="size-4" strokeWidth={2.5} />
                </>
              )}
            </SiswaButton>
          </div>
          <div className="mt-6 flex flex-wrap items-center justify-between gap-3 border-t-2 border-siswa-border-soft pt-4">
            <p className="text-sm text-siswa-text-muted">
              {answeredCount === att.total
                ? 'Semua soal sudah dijawab. Submit kalau kamu yakin.'
                : `Masih ada ${totalQuestions - answeredCount} soal yang belum dijawab.`}
            </p>
            <SiswaButton
              type="button"
              tone="ghost"
              size="sm"
              onClick={onAbort}
              disabled={submitMu.isPending}
            >
              Keluar
            </SiswaButton>
          </div>
        </SiswaCardBody>
      </SiswaCard>

      <Dialog open={questionMapOpen} onOpenChange={setQuestionMapOpen}>
        <DialogContent className="siswa-theme max-w-lg">
          <DialogHeader>
            <DialogTitle>Pilih nomor soal</DialogTitle>
            <DialogDescription>
              Lompat ke soal tertentu. Warna kuning berarti sudah dijawab, putih berarti belum.
            </DialogDescription>
          </DialogHeader>
          <div className="grid grid-cols-5 gap-2 sm:grid-cols-8">
            {items.map((item, idx) => {
              const answered = !!item.jawaban_siswa;
              const active = idx === safeActiveIndex;
              return (
                <button
                  key={item.soal_id}
                  type="button"
                  onClick={() => {
                    setActiveIndex(idx);
                    setQuestionMapOpen(false);
                  }}
                  className={cn(
                    'rounded-siswa border-2 border-siswa-border px-3 py-2 text-sm font-black siswa-shadow-sm transition-transform hover:-translate-y-0.5',
                    answered ? 'bg-siswa-yellow' : 'bg-siswa-surface',
                    active && 'translate-x-0.5 translate-y-0.5 bg-siswa-blue shadow-none ring-2 ring-siswa-border',
                  )}
                  aria-label={`Buka soal ${idx + 1}${answered ? ', sudah dijawab' : ', belum dijawab'}`}
                >
                  {idx + 1}
                </button>
              );
            })}
          </div>
          <div className="flex flex-wrap gap-2 text-xs font-semibold text-siswa-text-muted">
            <span className="rounded-full border-2 border-siswa-border bg-siswa-yellow px-2 py-1">Sudah dijawab</span>
            <span className="rounded-full border-2 border-siswa-border bg-siswa-surface px-2 py-1">Belum dijawab</span>
            <span className="rounded-full border-2 border-siswa-border bg-siswa-blue px-2 py-1">Soal aktif</span>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog
        open={confirmSubmit}
        onOpenChange={(o) => !submitMu.isPending && setConfirmSubmit(o)}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Submit ujian?</DialogTitle>
            <DialogDescription>
              Kamu sudah menjawab <strong>{answeredCount}</strong> dari {totalQuestions}{' '}
              soal. Setelah submit, jawaban ini dinilai dan tidak bisa diubah
              lagi.
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
              onClick={handleManualSubmit}
              disabled={submitMu.isPending || !allAnswered}
            >
              {submitMu.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Send className="size-4" />
              )}
              Ya, submit
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

interface SoalCardProps {
  item: UjianAttemptItem;
  index: number;
  autosaveErrMsg?: string;
  onChoose: (jawaban: UjianSoalJawaban) => void;
  disabled?: boolean;
}

function SoalCard({
  item,
  index,
  autosaveErrMsg,
  onChoose,
  disabled,
}: SoalCardProps) {
  const pertanyaanImg = imageURLForSlot(item, 'pertanyaan');
  return (
    <article className="rounded-siswa siswa-border bg-siswa-surface p-4 siswa-shadow-sm">
      <div className="mb-3 flex items-start justify-between gap-2">
        <p className="siswa-display text-sm font-bold uppercase tracking-wide text-siswa-text-muted">
          Soal {index + 1}{' '}
          <span className="font-semibold normal-case tracking-normal text-siswa-text">
            — {item.poin} poin
          </span>
        </p>
      </div>

      <div className="space-y-2">
        {item.pertanyaan ? (
          <p className="whitespace-pre-wrap text-sm">{item.pertanyaan}</p>
        ) : (
          <p className="text-sm italic text-siswa-text-muted">
            (soal hanya gambar)
          </p>
        )}
        {pertanyaanImg ? (
          <SlotImage url={pertanyaanImg} alt="Gambar pertanyaan" />
        ) : null}
      </div>

      <ul className="mt-3 space-y-2">
        {OPSI_LIST.map(({ key, label }) => {
          const text = opsiText(item, key);
          const slotImg = imageURLForSlot(item, `opsi_${key}`);
          const checked = item.jawaban_siswa === key;
          return (
            <li key={key}>
              <label
                className={cn(
                  'flex gap-3 rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface p-3 transition-colors',
                  disabled
                    ? 'cursor-not-allowed opacity-70'
                    : 'cursor-pointer hover:bg-siswa-cream/40',
                  checked && 'border-siswa-border bg-siswa-yellow/40',
                )}
              >
                <input
                  type="radio"
                  name={`ujian-soal-${item.soal_id}`}
                  value={key}
                  checked={checked}
                  onChange={() => onChoose(key)}
                  disabled={disabled}
                  className="mt-1 size-4"
                />
                <div className="min-w-0 flex-1 space-y-1.5">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm font-bold uppercase">
                      {label}.
                    </span>
                    {text ? (
                      <span className="whitespace-pre-wrap text-sm">{text}</span>
                    ) : (
                      <span className="text-xs italic text-siswa-text-muted">
                        (tanpa teks)
                      </span>
                    )}
                  </div>
                  {slotImg ? (
                    <SlotImage url={slotImg} alt={`Gambar opsi ${label}`} />
                  ) : null}
                </div>
              </label>
            </li>
          );
        })}
      </ul>

      {autosaveErrMsg ? (
        <div className="mt-2 flex items-start gap-1.5 rounded-siswa border-2 border-siswa-danger bg-rose-50 p-2 text-xs font-semibold">
          <AlertCircle className="mt-0.5 size-3.5 shrink-0" strokeWidth={2.5} />
          <span>
            Gagal simpan: {autosaveErrMsg} Klik ulang jawaban untuk coba lagi.
          </span>
        </div>
      ) : null}
    </article>
  );
}

function SlotImage({ url, alt }: { url: string; alt: string }) {
  return <ZoomableSoalImage url={url} alt={alt} />;
}
