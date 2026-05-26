'use client';

/**
 * LatihanPlayer — siswa latihan flow (Task 5.G.1).
 *
 * Backend pipeline:
 *   POST /siswa/bab/:id/latihan/start    → resume kalau ada attempt
 *                                          berlangsung, atau create baru.
 *   GET  /siswa/hasil-soal-bab/:id/items → fetch full content (pertanyaan +
 *                                          opsi + image presign + jawaban_siswa
 *                                          pre-fill + is_benar untuk latihan).
 *   POST /siswa/hasil-soal-bab/:id/answer → upsert jawaban. Latihan: server
 *                                          balikin is_benar + jawaban_benar
 *                                          + poin_dapat (locked #81).
 *   POST /siswa/hasil-soal-bab/:id/finish → status=selesai. Latihan no nilai
 *                                          persist. Returns summary
 *                                          {total, benar, salah, skip}.
 *
 * UX:
 *   1. Idle: tombol "Mulai latihan" + intro card.
 *   2. Playing: scroll list semua soal. Per radio change:
 *      - Optimistic update jawaban_siswa di local cache.
 *      - POST answer → balikin is_benar + jawaban_benar.
 *      - Banner inline hijau (benar) atau merah (salah) + label
 *        "Jawaban benar: <letter>" kalau salah.
 *   3. Submit: tombol "Selesai" sticky bottom. Confirm → finish → swap
 *      ke result screen.
 *   4. Result: card summary {total, benar, salah, skip} dengan tombol
 *      "Mulai lagi" → start ulang.
 *
 * Locked decisions:
 *   - #81: latihan re-attempt unlimited. is_benar surfaced langsung.
 *   - #62: image presign 15m. Gak refetch otomatis — kalau exp, FE re-fetch
 *     items via Refresh button (rare, 15m cukup untuk 1 latihan).
 *   - #76: anti-cheat — items tidak include jawaban_benar untuk attempt
 *     aktif; itu cuma muncul di response answer per soal.
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  AlertCircle,
  CheckCircle2,
  Loader2,
  PlayCircle,
  RotateCcw,
  Sparkles,
  XCircle,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  finishLatihan,
  friendlyAttemptError,
  getAttemptItems,
  postAnswer,
  startLatihan,
  type AttemptItem,
  type AttemptItemsResult,
  type AnswerResult,
  type FinishResult,
  type SoalJawaban,
} from '@/lib/soalbab-attempt-api';
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
import {
  SiswaBadge,
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardDescription,
  SiswaCardHeader,
  SiswaCardTitle,
  ZoomableSoalImage,
} from '@/components/siswa-ui';

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

export interface LatihanPlayerProps {
  babID: string;
  /** disabled saat bab archived; tombol start + answer di-block. */
  disabled?: boolean;
}

interface FeedbackState {
  isBenar: boolean;
  jawabanBenar: SoalJawaban | '';
  poinDapat: number;
}

type FeedbackMap = Record<string, FeedbackState | undefined>;

export function LatihanPlayer({ babID, disabled }: LatihanPlayerProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [hasilID, setHasilID] = React.useState<string | null>(null);
  const [feedback, setFeedback] = React.useState<FeedbackMap>({});
  const [finishing, setFinishing] = React.useState(false);
  const [confirmFinish, setConfirmFinish] = React.useState(false);
  const [summary, setSummary] = React.useState<FinishResult | null>(null);

  const itemsQueryKey = React.useMemo(
    () => ['siswa', 'latihan', 'items', hasilID] as const,
    [hasilID],
  );

  const itemsQuery = useQuery({
    queryKey: itemsQueryKey,
    queryFn: () => {
      if (!hasilID) throw new Error('no hasil');
      return getAttemptItems(hasilID);
    },
    enabled: !!hasilID && !summary,
    staleTime: 60_000,
  });

  // Pre-fill feedback dari is_benar + jawaban_siswa di items (resume case
  // — siswa pernah jawab di session lama, refresh, ga ada jawaban_benar
  // tapi kita masih bisa flag is_benar untuk visual cue. jawabanBenar
  // dibiarkan kosong; akan di-fill ulang kalau siswa jawab ulang).
  React.useEffect(() => {
    const att = itemsQuery.data?.attempt;
    if (!att) return;
    const next: FeedbackMap = {};
    for (const it of att.items) {
      if (it.jawaban_siswa && typeof it.is_benar === 'boolean') {
        next[it.soal_id] = {
          isBenar: it.is_benar,
          jawabanBenar: '',
          poinDapat: 0,
        };
      }
    }
    setFeedback((prev) => ({ ...next, ...prev }));
    // ↑ existing in-session feedback wins over server snapshot.
  }, [itemsQuery.data?.attempt]);

  const startMu = useMutation({
    mutationFn: () => startLatihan(babID),
    onSuccess: ({ hasil }) => {
      setHasilID(hasil.hasil_id);
      setSummary(null);
      // Jawaban map dari resume: pre-fill local cache di items query
      // setelah load. Feedback (is_benar) datang dari items query.
    },
    onError: (err) => {
      const msg =
        err instanceof ApiError
          ? friendlyAttemptError(err, 'start')
          : 'Gagal memulai latihan.';
      toast({ title: 'Gagal memulai latihan', description: msg, variant: 'destructive' });
    },
  });

  const answerMu = useMutation({
    mutationFn: ({ soalID, jawaban }: { soalID: string; jawaban: SoalJawaban }) => {
      if (!hasilID) throw new Error('no hasil');
      return postAnswer(hasilID, { soal_id: soalID, jawaban });
    },
    onSuccess: ({ answer }, { soalID }) => {
      setFeedback((prev) => ({
        ...prev,
        [soalID]: {
          isBenar: answer.is_benar,
          // server return '' kalau ulangan; latihan should always have letter.
          jawabanBenar: answer.jawaban_benar,
          poinDapat: answer.poin_dapat,
        },
      }));
    },
    onError: (err) => {
      const msg =
        err instanceof ApiError
          ? friendlyAttemptError(err, 'answer')
          : 'Gagal menyimpan jawaban.';
      toast({ title: 'Gagal menyimpan', description: msg, variant: 'destructive' });
    },
  });

  const finishMu = useMutation({
    mutationFn: () => {
      if (!hasilID) throw new Error('no hasil');
      return finishLatihan(hasilID);
    },
    onSuccess: ({ summary: s }) => {
      setSummary(s);
      setHasilID(null);
      setFeedback({});
      setConfirmFinish(false);
      // Trigger refresh of any rekap query nearby.
      queryClient.invalidateQueries({ queryKey: ['siswa', 'bab', 'hasil', babID] });
    },
    onError: (err) => {
      const msg =
        err instanceof ApiError
          ? friendlyAttemptError(err, 'finish')
          : 'Gagal menyelesaikan latihan.';
      toast({ title: 'Gagal selesai', description: msg, variant: 'destructive' });
      setFinishing(false);
    },
  });

  function handleAnswer(soalID: string, jawaban: SoalJawaban) {
    if (disabled || answerMu.isPending) return;
    // Optimistic UI: update local cache jawaban_siswa, clear stale feedback.
    queryClient.setQueryData(itemsQueryKey, (old: { attempt: AttemptItemsResult } | undefined) => {
      if (!old) return old;
      return {
        attempt: {
          ...old.attempt,
          items: old.attempt.items.map((it) =>
            it.soal_id === soalID ? { ...it, jawaban_siswa: jawaban } : it,
          ),
        },
      };
    });
    setFeedback((prev) => ({ ...prev, [soalID]: undefined }));
    answerMu.mutate({ soalID, jawaban });
  }

  // ---- Render: Result summary ----
  if (summary) {
    return <LatihanResultCard summary={summary} onRestart={() => startMu.mutate()} disabled={disabled} starting={startMu.isPending} />;
  }

  // ---- Render: Idle ----
  if (!hasilID) {
    return (
      <SiswaCard tone="latihan" shadow="md">
        <SiswaCardHeader>
          <div className="flex items-center gap-2">
            <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
              <Sparkles className="size-4" strokeWidth={2.5} />
            </span>
            <SiswaCardTitle>Latihan Bab</SiswaCardTitle>
          </div>
          <SiswaCardDescription>
            Latihan formative — kerjakan soal bebas, dapat feedback langsung tiap jawaban.
            Tidak ada batas waktu, tidak ada nilai persist. Re-attempt sebanyak yang lu mau.
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          <SiswaButton
            type="button"
            tone="primary"
            onClick={() => startMu.mutate()}
            disabled={disabled || startMu.isPending}
          >
            {startMu.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <PlayCircle className="size-4" strokeWidth={2.5} />
            )}
            Mulai latihan
          </SiswaButton>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  // ---- Render: Loading items ----
  if (itemsQuery.isPending) {
    return (
      <SiswaCard tone="surface" shadow="sm">
        <SiswaCardHeader>
          <SiswaCardTitle>Memuat soal latihan…</SiswaCardTitle>
        </SiswaCardHeader>
        <SiswaCardBody>
          <div className="h-32 animate-pulse rounded-siswa siswa-border bg-siswa-surface/60" />
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  // ---- Render: Error ----
  if (itemsQuery.isError || !itemsQuery.data) {
    const msg =
      itemsQuery.error instanceof ApiError
        ? friendlyAttemptError(itemsQuery.error, 'items')
        : 'Gagal memuat soal latihan.';
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>Gagal memuat latihan</SiswaCardTitle>
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
            <SiswaButton
              type="button"
              size="sm"
              tone="ghost"
              onClick={() => setHasilID(null)}
            >
              Reset
            </SiswaButton>
          </div>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  const att = itemsQuery.data.attempt;
  const answeredCount = att.items.filter((it) => !!it.jawaban_siswa).length;

  // ---- Render: Playing ----
  return (
    <>
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <SiswaCardTitle>
                Latihan Bab — {att.total} soal
              </SiswaCardTitle>
              <SiswaCardDescription>
                Jawab semua soal lalu klik Selesai. Feedback muncul tiap kali pilih jawaban.
              </SiswaCardDescription>
            </div>
            <div className="text-sm text-siswa-text-muted">
              Terjawab:{' '}
              <strong className="siswa-display text-siswa-text">{answeredCount}</strong> /{' '}
              {att.total}
            </div>
          </div>
        </SiswaCardHeader>
        <SiswaCardBody>
          <ol className="space-y-4">
            {att.items.map((item, idx) => (
              <SoalCard
                key={item.soal_id}
                item={item}
                index={idx}
                feedback={feedback[item.soal_id]}
                onChoose={(j) => handleAnswer(item.soal_id, j)}
                disabled={disabled || answerMu.isPending}
              />
            ))}
          </ol>
          <div className="mt-6 flex flex-wrap items-center justify-between gap-3 border-t-2 border-siswa-border-soft pt-4">
            <p className="text-sm text-siswa-text-muted">
              {answeredCount === att.total
                ? 'Semua soal sudah dijawab. Lu bisa Selesai sekarang.'
                : `Masih ada ${att.total - answeredCount} soal yang belum dijawab.`}
            </p>
            <SiswaButton
              type="button"
              tone="primary"
              onClick={() => setConfirmFinish(true)}
              disabled={disabled || finishMu.isPending || finishing}
            >
              {finishMu.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
              Selesai
            </SiswaButton>
          </div>
        </SiswaCardBody>
      </SiswaCard>

      <Dialog open={confirmFinish} onOpenChange={(o) => !finishMu.isPending && setConfirmFinish(o)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Selesai latihan?</DialogTitle>
            <DialogDescription>
              Lu sudah menjawab {answeredCount} dari {att.total} soal. Setelah selesai, attempt
              ini ditandai selesai dan lu bisa langsung mulai latihan baru kalau mau.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={() => setConfirmFinish(false)}
              disabled={finishMu.isPending}
            >
              Batal
            </Button>
            <Button
              type="button"
              onClick={() => {
                setFinishing(true);
                finishMu.mutate();
              }}
              disabled={finishMu.isPending}
            >
              {finishMu.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
              Ya, selesai
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
  feedback?: FeedbackState;
  onChoose: (jawaban: SoalJawaban) => void;
  disabled?: boolean;
}

function SoalCard({ item, index, feedback, onChoose, disabled }: SoalCardProps) {
  const pertanyaanImg = imageURLForSlot(item, 'pertanyaan');
  return (
    <li className="rounded-siswa siswa-border bg-siswa-surface p-4 siswa-shadow-sm">
      <div className="mb-3 flex items-start justify-between gap-2">
        <p className="siswa-display text-sm font-bold uppercase tracking-wide text-siswa-text-muted">
          Soal {index + 1}{' '}
          <span className="font-semibold normal-case tracking-normal text-siswa-text">
            — {item.poin} poin
          </span>
        </p>
        {feedback ? <FeedbackBadge feedback={feedback} /> : null}
      </div>

      <div className="space-y-2">
        {item.pertanyaan ? (
          <p className="whitespace-pre-wrap text-sm">{item.pertanyaan}</p>
        ) : (
          <p className="text-sm italic text-siswa-text-muted">(soal hanya gambar)</p>
        )}
        {pertanyaanImg ? <SlotImage url={pertanyaanImg} alt="Gambar pertanyaan" /> : null}
      </div>

      <ul className="mt-3 space-y-2">
        {OPSI_LIST.map(({ key, label }) => {
          const text = opsiText(item, key);
          const slotImg = imageURLForSlot(item, key);
          const checked = item.jawaban_siswa === key;
          const isCorrectKey = feedback?.jawabanBenar === key;
          const wronglyChosen = checked && feedback && !feedback.isBenar;
          return (
            <li key={key}>
              <label
                className={cn(
                  'flex gap-3 rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface p-3 transition-colors',
                  disabled
                    ? 'cursor-not-allowed opacity-70'
                    : 'cursor-pointer hover:bg-siswa-cream/40',
                  checked && feedback?.isBenar && 'border-siswa-border bg-siswa-green/40',
                  wronglyChosen && 'border-siswa-border bg-rose-100',
                  !checked && isCorrectKey && 'border-siswa-border bg-siswa-green/30',
                )}
              >
                <input
                  type="radio"
                  name={`soal-${item.soal_id}`}
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
    </li>
  );
}

function FeedbackBadge({ feedback }: { feedback: FeedbackState }) {
  if (feedback.isBenar) {
    return (
      <SiswaBadge tone="success">
        <CheckCircle2 className="size-3" strokeWidth={2.5} />
        Benar (+{feedback.poinDapat || 0})
      </SiswaBadge>
    );
  }
  return (
    <SiswaBadge tone="danger">
      <XCircle className="size-3" strokeWidth={2.5} />
      Salah
      {feedback.jawabanBenar
        ? ` — jawaban: ${feedback.jawabanBenar.toUpperCase()}`
        : ''}
    </SiswaBadge>
  );
}

function SlotImage({ url, alt }: { url: string; alt: string }) {
  return <ZoomableSoalImage url={url} alt={alt} />;
}

interface LatihanResultCardProps {
  summary: FinishResult;
  onRestart: () => void;
  disabled?: boolean;
  starting?: boolean;
}

function LatihanResultCard({
  summary,
  onRestart,
  disabled,
  starting,
}: LatihanResultCardProps) {
  const persen =
    summary.total === 0
      ? 0
      : Math.round((summary.benar / summary.total) * 100);
  return (
    <SiswaCard tone="latihan" shadow="md">
      <SiswaCardHeader>
        <div className="flex items-center gap-2">
          <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
            <CheckCircle2 className="size-4 text-emerald-600" strokeWidth={2.5} />
          </span>
          <SiswaCardTitle>Latihan Selesai</SiswaCardTitle>
        </div>
        <SiswaCardDescription>
          Sip, latihan ini sudah selesai. Latihan formative — nilai tidak
          dipersist, tapi lu bisa pakai feedback ini buat refleksi.
        </SiswaCardDescription>
      </SiswaCardHeader>
      <SiswaCardBody>
        <div className="grid gap-3 sm:grid-cols-4">
          <SummaryCell label="Total soal" value={summary.total} />
          <SummaryCell label="Benar" value={summary.benar} accent="emerald" />
          <SummaryCell label="Salah" value={summary.salah} accent="rose" />
          <SummaryCell label="Skip" value={summary.skip} accent="muted" />
        </div>
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-siswa border-2 border-siswa-border-soft bg-siswa-cream/40 p-3 text-sm">
          <span className="text-siswa-text-muted">Persentase benar:</span>
          <strong className="siswa-display text-base">{persen}%</strong>
        </div>
        <div className="mt-4 flex flex-wrap gap-2">
          <SiswaButton
            type="button"
            tone="primary"
            onClick={onRestart}
            disabled={disabled || starting}
          >
            {starting ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <PlayCircle className="size-4" strokeWidth={2.5} />
            )}
            Mulai latihan baru
          </SiswaButton>
        </div>
      </SiswaCardBody>
    </SiswaCard>
  );
}

function SummaryCell({
  label,
  value,
  accent = 'default',
}: {
  label: string;
  value: number;
  accent?: 'default' | 'emerald' | 'rose' | 'muted';
}) {
  const accentClass = {
    default: 'border-siswa-border bg-siswa-surface',
    emerald: 'border-siswa-border bg-siswa-green/40 text-emerald-700',
    rose: 'border-siswa-border bg-rose-100 text-rose-700',
    muted: 'border-siswa-border-soft bg-siswa-cream/30 text-siswa-text-muted',
  }[accent];
  return (
    <div className={cn('rounded-siswa border-2 p-3 text-center', accentClass)}>
      <div className="text-xs font-semibold uppercase tracking-wide opacity-80">
        {label}
      </div>
      <div className="siswa-display mt-1 text-2xl font-bold">{value}</div>
    </div>
  );
}
