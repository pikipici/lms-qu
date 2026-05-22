'use client';

/**
 * UlanganReview — siswa review jawaban setelah submit (Task 5.G.2).
 *
 * Render review payload dari GET /siswa/hasil-soal-bab/:id/review.
 * Backend handle gating (#81): kalau gated, response 403 review_locked /
 * review_disabled — kita map ke friendly notice + tombol Refresh.
 *
 * Per soal:
 *   - Card dengan pertanyaan + opsi A-E.
 *   - Highlight:
 *     · jawaban_benar  → border emerald + bg emerald/40
 *     · jawaban_siswa salah → border rose + bg rose/40
 *     · jawaban_siswa benar → sama dengan jawaban_benar
 *   - Badge top-right: "Benar +N poin" / "Salah" / "Tidak dijawab".
 */

import * as React from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  AlertCircle,
  ArrowLeft,
  CheckCircle2,
  Loader2,
  RotateCcw,
  XCircle,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  friendlyUlanganError,
  getReview,
  type ReviewItem,
  type ReviewResult,
} from '@/lib/soalbab-ulangan-api';
import type { SoalJawaban } from '@/lib/soalbab-attempt-api';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { cn } from '@/lib/utils';

const OPSI_LIST: { key: SoalJawaban; label: string }[] = [
  { key: 'a', label: 'A' },
  { key: 'b', label: 'B' },
  { key: 'c', label: 'C' },
  { key: 'd', label: 'D' },
  { key: 'e', label: 'E' },
];

function opsiText(item: ReviewItem, k: SoalJawaban): string {
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

export interface UlanganReviewProps {
  hasilID: string;
  onBack: () => void;
}

export function UlanganReview({ hasilID, onBack }: UlanganReviewProps) {
  const reviewQuery = useQuery({
    queryKey: ['siswa', 'ulangan', 'review', hasilID],
    queryFn: () => getReview(hasilID),
    staleTime: 60_000,
    retry: (count, err) => {
      if (err instanceof ApiError) {
        if (err.status === 403 || err.status === 404) return false;
      }
      return count < 2;
    },
  });

  if (reviewQuery.isPending) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Memuat pembahasan…</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="h-32 animate-pulse rounded-md border bg-muted/40" />
        </CardContent>
      </Card>
    );
  }

  if (reviewQuery.isError) {
    const apiErr = reviewQuery.error instanceof ApiError ? reviewQuery.error : null;
    const msg = apiErr ? friendlyUlanganError(apiErr, 'review') : 'Gagal memuat pembahasan.';
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Gagal memuat pembahasan</CardTitle>
          <CardDescription>{msg}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-2">
            <Button type="button" size="sm" variant="outline" onClick={() => reviewQuery.refetch()}>
              <RotateCcw className="size-4" />
              Coba lagi
            </Button>
            <Button type="button" size="sm" variant="ghost" onClick={onBack}>
              <ArrowLeft className="size-4" />
              Kembali ke lobi
            </Button>
          </div>
        </CardContent>
      </Card>
    );
  }

  const review = reviewQuery.data!.review;
  return <ReviewBody review={review} onBack={onBack} onRefresh={() => reviewQuery.refetch()} refreshing={reviewQuery.isFetching} />;
}

function ReviewBody({
  review,
  onBack,
  onRefresh,
  refreshing,
}: {
  review: ReviewResult;
  onBack: () => void;
  onRefresh: () => void;
  refreshing: boolean;
}) {
  const total = review.jawaban_total ?? review.items.length;
  const benar = review.jawaban_benar_count ?? 0;
  const persen = total === 0 ? 0 : Math.round((benar / total) * 100);
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div>
              <CardTitle className="text-base">Pembahasan Ulangan</CardTitle>
              <CardDescription>
                Attempt #{review.attempt_no} ·{' '}
                {review.selesai_at
                  ? new Date(review.selesai_at).toLocaleString('id-ID', {
                      dateStyle: 'medium',
                      timeStyle: 'short',
                    })
                  : '—'}
              </CardDescription>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={onRefresh}
                disabled={refreshing}
              >
                {refreshing ? <Loader2 className="size-4 animate-spin" /> : <RotateCcw className="size-4" />}
                Refresh
              </Button>
              <Button type="button" size="sm" variant="ghost" onClick={onBack}>
                <ArrowLeft className="size-4" />
                Kembali
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid gap-3 sm:grid-cols-4">
            <SummaryCell label="Total soal" value={total} />
            <SummaryCell label="Benar" value={benar} accent="emerald" />
            <SummaryCell label="Salah" value={Math.max(0, total - benar)} accent="rose" />
            <SummaryCell
              label="Nilai"
              value={review.nilai_total != null ? Number(review.nilai_total).toFixed(0) : '—'}
              accent="primary"
            />
          </div>
          <div className="mt-3 flex items-center justify-between rounded-md border bg-muted/30 p-3 text-sm">
            <span className="text-muted-foreground">Persentase benar:</span>
            <strong className="text-base">{persen}%</strong>
          </div>
        </CardContent>
      </Card>

      <ol className="space-y-4">
        {review.items.map((item, idx) => (
          <ReviewSoalCard key={`${item.soal_id}-${idx}`} item={item} index={idx} />
        ))}
      </ol>
    </div>
  );
}

function SummaryCell({
  label,
  value,
  accent = 'default',
}: {
  label: string;
  value: number | string;
  accent?: 'default' | 'emerald' | 'rose' | 'primary';
}) {
  const accentClass = {
    default: 'border-border',
    emerald: 'border-emerald-200 bg-emerald-50 text-emerald-700',
    rose: 'border-rose-200 bg-rose-50 text-rose-700',
    primary: 'border-primary/30 bg-primary/5 text-foreground',
  }[accent];
  return (
    <div className={cn('rounded-md border p-3 text-center', accentClass)}>
      <div className="text-xs uppercase tracking-wide opacity-80">{label}</div>
      <div className="mt-1 text-2xl font-bold">{value}</div>
    </div>
  );
}

function ReviewSoalCard({ item, index }: { item: ReviewItem; index: number }) {
  const tidakDijawab = !item.jawaban_siswa;
  const benar = item.is_benar === true;
  const salah = item.is_benar === false;

  return (
    <li className="rounded-md border bg-card p-4">
      <div className="mb-3 flex items-start justify-between gap-2">
        <p className="text-sm font-medium uppercase tracking-wide text-muted-foreground">
          Soal {index + 1} <span className="font-normal text-foreground">— {item.poin_maksimal} poin</span>
        </p>
        {tidakDijawab ? (
          <span className="inline-flex items-center gap-1 rounded-full border border-amber-200 bg-amber-50 px-2 py-0.5 text-xs text-amber-700">
            <AlertCircle className="size-3.5" />
            Tidak dijawab
          </span>
        ) : benar ? (
          <span className="inline-flex items-center gap-1 rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-xs text-emerald-700">
            <CheckCircle2 className="size-3.5" />
            Benar (+{item.poin_dapat})
          </span>
        ) : salah ? (
          <span className="inline-flex items-center gap-1 rounded-full border border-rose-200 bg-rose-50 px-2 py-0.5 text-xs text-rose-700">
            <XCircle className="size-3.5" />
            Salah
          </span>
        ) : null}
      </div>

      {item.pertanyaan ? (
        <p className="whitespace-pre-wrap text-sm">{item.pertanyaan}</p>
      ) : (
        <p className="text-sm italic text-muted-foreground">(soal sudah dihapus guru)</p>
      )}

      <ul className="mt-3 space-y-2">
        {OPSI_LIST.map(({ key, label }) => {
          const text = opsiText(item, key);
          const isCorrect = item.jawaban_benar === key;
          const isPicked = item.jawaban_siswa === key;
          const wronglyPicked = isPicked && !isCorrect;

          return (
            <li
              key={key}
              className={cn(
                'flex gap-3 rounded-md border p-3 transition-colors',
                isCorrect && 'border-emerald-300 bg-emerald-50/70',
                wronglyPicked && 'border-rose-300 bg-rose-50/70',
              )}
            >
              <span
                className={cn(
                  'mt-0.5 inline-flex size-5 items-center justify-center rounded-full border text-xs font-semibold uppercase',
                  isCorrect && 'border-emerald-400 bg-emerald-100 text-emerald-800',
                  wronglyPicked && 'border-rose-400 bg-rose-100 text-rose-800',
                  !isCorrect && !wronglyPicked && 'border-muted-foreground/30',
                )}
              >
                {label}
              </span>
              <div className="min-w-0 flex-1 space-y-1">
                {text ? (
                  <span className="whitespace-pre-wrap text-sm">{text}</span>
                ) : (
                  <span className="text-xs italic text-muted-foreground">(tanpa teks)</span>
                )}
                <div className="flex flex-wrap gap-2 text-xs">
                  {isCorrect ? (
                    <span className="text-emerald-700">Jawaban benar</span>
                  ) : null}
                  {isPicked ? (
                    <span className={cn(wronglyPicked ? 'text-rose-700' : 'text-emerald-700')}>
                      Pilihan kamu
                    </span>
                  ) : null}
                </div>
              </div>
            </li>
          );
        })}
      </ul>
    </li>
  );
}
