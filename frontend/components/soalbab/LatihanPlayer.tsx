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
  ImageOff,
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
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Sparkles className="size-5 text-muted-foreground" />
            <CardTitle className="text-base">Latihan Bab</CardTitle>
          </div>
          <CardDescription>
            Latihan formative — kerjakan soal bebas, dapat feedback langsung tiap jawaban.
            Tidak ada batas waktu, tidak ada nilai persist. Re-attempt sebanyak yang lu mau.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            type="button"
            onClick={() => startMu.mutate()}
            disabled={disabled || startMu.isPending}
          >
            {startMu.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <PlayCircle className="size-4" />
            )}
            Mulai latihan
          </Button>
        </CardContent>
      </Card>
    );
  }

  // ---- Render: Loading items ----
  if (itemsQuery.isPending) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Memuat soal latihan…</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="h-32 animate-pulse rounded-md border bg-muted/40" />
        </CardContent>
      </Card>
    );
  }

  // ---- Render: Error ----
  if (itemsQuery.isError || !itemsQuery.data) {
    const msg =
      itemsQuery.error instanceof ApiError
        ? friendlyAttemptError(itemsQuery.error, 'items')
        : 'Gagal memuat soal latihan.';
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Gagal memuat latihan</CardTitle>
          <CardDescription>{msg}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => itemsQuery.refetch()}
            >
              <RotateCcw className="size-4" />
              Coba lagi
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => setHasilID(null)}
            >
              Reset
            </Button>
          </div>
        </CardContent>
      </Card>
    );
  }

  const att = itemsQuery.data.attempt;
  const answeredCount = att.items.filter((it) => !!it.jawaban_siswa).length;

  // ---- Render: Playing ----
  return (
    <>
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <CardTitle className="text-base">Latihan Bab — {att.total} soal</CardTitle>
              <CardDescription>
                Jawab semua soal lalu klik Selesai. Feedback muncul tiap kali pilih jawaban.
              </CardDescription>
            </div>
            <div className="text-sm text-muted-foreground">
              Terjawab: <strong className="text-foreground">{answeredCount}</strong> /{' '}
              {att.total}
            </div>
          </div>
        </CardHeader>
        <CardContent>
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
          <div className="mt-6 flex flex-wrap items-center justify-between gap-3 border-t pt-4">
            <p className="text-sm text-muted-foreground">
              {answeredCount === att.total
                ? 'Semua soal sudah dijawab. Lu bisa Selesai sekarang.'
                : `Masih ada ${att.total - answeredCount} soal yang belum dijawab.`}
            </p>
            <Button
              type="button"
              onClick={() => setConfirmFinish(true)}
              disabled={disabled || finishMu.isPending || finishing}
            >
              {finishMu.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
              Selesai
            </Button>
          </div>
        </CardContent>
      </Card>

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
    <li className="rounded-md border bg-card p-4">
      <div className="mb-3 flex items-start justify-between gap-2">
        <p className="text-sm font-medium uppercase tracking-wide text-muted-foreground">
          Soal {index + 1} <span className="font-normal text-foreground">— {item.poin} poin</span>
        </p>
        {feedback ? (
          <FeedbackBadge feedback={feedback} />
        ) : null}
      </div>

      <div className="space-y-2">
        {item.pertanyaan ? (
          <p className="whitespace-pre-wrap text-sm">{item.pertanyaan}</p>
        ) : (
          <p className="text-sm italic text-muted-foreground">(soal hanya gambar)</p>
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
                  'flex gap-3 rounded-md border p-3 transition-colors',
                  disabled ? 'cursor-not-allowed opacity-70' : 'cursor-pointer hover:bg-muted/40',
                  checked && feedback?.isBenar && 'border-emerald-300 bg-emerald-50',
                  wronglyChosen && 'border-rose-300 bg-rose-50',
                  !checked && isCorrectKey && 'border-emerald-300 bg-emerald-50/60',
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
    </li>
  );
}

function FeedbackBadge({ feedback }: { feedback: FeedbackState }) {
  if (feedback.isBenar) {
    return (
      <span className="inline-flex items-center gap-1 rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-xs text-emerald-700">
        <CheckCircle2 className="size-3.5" />
        Benar (+{feedback.poinDapat || 0})
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 rounded-full border border-rose-200 bg-rose-50 px-2 py-0.5 text-xs text-rose-700">
      <XCircle className="size-3.5" />
      Salah
      {feedback.jawabanBenar ? ` — jawaban: ${feedback.jawabanBenar.toUpperCase()}` : ''}
    </span>
  );
}

function SlotImage({ url, alt }: { url: string; alt: string }) {
  const [errored, setErrored] = React.useState(false);
  if (errored) {
    return (
      <div className="flex h-20 items-center justify-center gap-2 rounded-md border border-dashed text-xs text-muted-foreground">
        <ImageOff className="size-4" />
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

interface LatihanResultCardProps {
  summary: FinishResult;
  onRestart: () => void;
  disabled?: boolean;
  starting?: boolean;
}

function LatihanResultCard({ summary, onRestart, disabled, starting }: LatihanResultCardProps) {
  const persen = summary.total === 0 ? 0 : Math.round((summary.benar / summary.total) * 100);
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <CheckCircle2 className="size-5 text-emerald-600" />
          <CardTitle className="text-base">Latihan Selesai</CardTitle>
        </div>
        <CardDescription>
          Sip, latihan ini sudah selesai. Latihan formative — nilai tidak dipersist, tapi lu
          bisa pakai feedback ini buat refleksi.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="grid gap-3 sm:grid-cols-4">
          <SummaryCell label="Total soal" value={summary.total} />
          <SummaryCell label="Benar" value={summary.benar} accent="emerald" />
          <SummaryCell label="Salah" value={summary.salah} accent="rose" />
          <SummaryCell label="Skip" value={summary.skip} accent="muted" />
        </div>
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-md border bg-muted/30 p-3 text-sm">
          <span className="text-muted-foreground">Persentase benar:</span>
          <strong className="text-base">{persen}%</strong>
        </div>
        <div className="mt-4 flex flex-wrap gap-2">
          <Button type="button" onClick={onRestart} disabled={disabled || starting}>
            {starting ? <Loader2 className="size-4 animate-spin" /> : <PlayCircle className="size-4" />}
            Mulai latihan baru
          </Button>
        </div>
      </CardContent>
    </Card>
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
    default: 'border-border',
    emerald: 'border-emerald-200 bg-emerald-50 text-emerald-700',
    rose: 'border-rose-200 bg-rose-50 text-rose-700',
    muted: 'border-border bg-muted/30 text-muted-foreground',
  }[accent];
  return (
    <div className={cn('rounded-md border p-3 text-center', accentClass)}>
      <div className="text-xs uppercase tracking-wide opacity-80">{label}</div>
      <div className="mt-1 text-2xl font-bold">{value}</div>
    </div>
  );
}
