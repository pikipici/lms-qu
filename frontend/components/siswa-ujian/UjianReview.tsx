'use client';

/**
 * UjianReview — siswa pembahasan post-submit (Task 6.G.2).
 *
 * Visual: neo-brutalism + pastel pop. Header card ulangan tone, soal cards
 * surface dengan emerald/rose highlight per jawaban.
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
  friendlySiswaUjianError,
  getSiswaUjianReview,
  type UjianReviewItem,
  type UjianReviewResult,
  type UjianSoalJawaban,
} from '@/lib/siswa-ujian-api';
import { cn } from '@/lib/utils';
import {
  SiswaBadge,
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardDescription,
  SiswaCardHeader,
  SiswaCardTitle,
} from '@/components/siswa-ui';

const OPSI_LIST: { key: UjianSoalJawaban; label: string }[] = [
  { key: 'a', label: 'A' },
  { key: 'b', label: 'B' },
  { key: 'c', label: 'C' },
  { key: 'd', label: 'D' },
  { key: 'e', label: 'E' },
];

function opsiText(item: UjianReviewItem, k: UjianSoalJawaban): string {
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

export interface UjianReviewProps {
  hasilID: string;
  onBack: () => void;
}

export function UjianReview({ hasilID, onBack }: UjianReviewProps) {
  const reviewQuery = useQuery({
    queryKey: ['siswa', 'ujian', 'review', hasilID],
    queryFn: () => getSiswaUjianReview(hasilID),
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
      <SiswaCard tone="surface" shadow="sm">
        <SiswaCardHeader>
          <SiswaCardTitle>Memuat pembahasan…</SiswaCardTitle>
        </SiswaCardHeader>
        <SiswaCardBody>
          <div className="h-32 animate-pulse rounded-siswa siswa-border bg-siswa-surface/60" />
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  if (reviewQuery.isError) {
    const apiErr =
      reviewQuery.error instanceof ApiError ? reviewQuery.error : null;
    const msg = apiErr
      ? friendlySiswaUjianError(apiErr, 'review')
      : 'Gagal memuat pembahasan.';
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>Gagal memuat pembahasan</SiswaCardTitle>
          <SiswaCardDescription>{msg}</SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          <div className="flex flex-wrap gap-2">
            <SiswaButton
              type="button"
              size="sm"
              tone="surface"
              onClick={() => reviewQuery.refetch()}
            >
              <RotateCcw className="size-4" />
              Coba lagi
            </SiswaButton>
            <SiswaButton type="button" size="sm" tone="ghost" onClick={onBack}>
              <ArrowLeft className="size-4" />
              Kembali
            </SiswaButton>
          </div>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  const review = reviewQuery.data!.review;
  return (
    <ReviewBody
      review={review}
      onBack={onBack}
      onRefresh={() => reviewQuery.refetch()}
      refreshing={reviewQuery.isFetching}
    />
  );
}

function ReviewBody({
  review,
  onBack,
  onRefresh,
  refreshing,
}: {
  review: UjianReviewResult;
  onBack: () => void;
  onRefresh: () => void;
  refreshing: boolean;
}) {
  const total = review.jawaban_total ?? review.items.length;
  const benar = review.jawaban_benar_count ?? 0;
  const persen = total === 0 ? 0 : Math.round((benar / total) * 100);
  return (
    <div className="space-y-4">
      <SiswaCard tone="ulangan" shadow="md">
        <SiswaCardHeader>
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div>
              <SiswaCardTitle>Pembahasan Ujian</SiswaCardTitle>
              <SiswaCardDescription>
                Kesempatan #{review.attempt_no} ·{' '}
                {review.selesai_at
                  ? new Date(review.selesai_at).toLocaleString('id-ID', {
                      dateStyle: 'medium',
                      timeStyle: 'short',
                      timeZone: 'Asia/Jakarta',
                    })
                  : '—'}
              </SiswaCardDescription>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <SiswaButton
                type="button"
                size="sm"
                tone="surface"
                onClick={onRefresh}
                disabled={refreshing}
              >
                {refreshing ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <RotateCcw className="size-4" />
                )}
                Refresh
              </SiswaButton>
              <SiswaButton type="button" size="sm" tone="ghost" onClick={onBack}>
                <ArrowLeft className="size-4" />
                Kembali
              </SiswaButton>
            </div>
          </div>
        </SiswaCardHeader>
        <SiswaCardBody>
          <div className="grid gap-3 sm:grid-cols-4">
            <SummaryCell label="Total soal" value={total} />
            <SummaryCell label="Benar" value={benar} accent="emerald" />
            <SummaryCell
              label="Salah"
              value={Math.max(0, total - benar)}
              accent="rose"
            />
            <SummaryCell
              label="Nilai"
              value={
                review.nilai_total != null
                  ? Number(review.nilai_total).toFixed(0)
                  : '—'
              }
              accent="primary"
            />
          </div>
          <div className="mt-3 flex items-center justify-between rounded-siswa border-2 border-siswa-border-soft bg-siswa-cream/40 p-3 text-sm">
            <span className="text-siswa-text-muted">Persentase benar:</span>
            <strong className="siswa-display text-base">{persen}%</strong>
          </div>
        </SiswaCardBody>
      </SiswaCard>

      <ol className="space-y-4">
        {review.items.map((item, idx) => (
          <ReviewSoalCard
            key={`${item.soal_id}-${idx}`}
            item={item}
            index={idx}
          />
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
    default: 'border-siswa-border bg-siswa-surface',
    emerald: 'border-siswa-border bg-siswa-green/40 text-emerald-700',
    rose: 'border-siswa-border bg-rose-100 text-rose-700',
    primary: 'border-siswa-border bg-siswa-yellow text-siswa-text',
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

function ReviewSoalCard({
  item,
  index,
}: {
  item: UjianReviewItem;
  index: number;
}) {
  const tidakDijawab = !item.jawaban_siswa;
  const benar = item.is_benar === true;
  const salah = item.is_benar === false;
  const isDeleted = item.pertanyaan === '(soal sudah dihapus guru)';

  return (
    <li className="rounded-siswa siswa-border bg-siswa-surface p-4 siswa-shadow-sm">
      <div className="mb-3 flex items-start justify-between gap-2">
        <p className="siswa-display text-sm font-bold uppercase tracking-wide text-siswa-text-muted">
          Soal {index + 1}{' '}
          <span className="font-semibold normal-case tracking-normal text-siswa-text">
            — {item.poin_maksimal} poin
          </span>
        </p>
        {isDeleted ? null : tidakDijawab ? (
          <SiswaBadge tone="warning">
            <AlertCircle className="size-3" strokeWidth={2.5} />
            Tidak dijawab
          </SiswaBadge>
        ) : benar ? (
          <SiswaBadge tone="success">
            <CheckCircle2 className="size-3" strokeWidth={2.5} />
            Benar (+{item.poin_dapat})
          </SiswaBadge>
        ) : salah ? (
          <SiswaBadge tone="danger">
            <XCircle className="size-3" strokeWidth={2.5} />
            Salah
          </SiswaBadge>
        ) : null}
      </div>

      {item.pertanyaan ? (
        <p
          className={cn(
            'whitespace-pre-wrap text-sm',
            isDeleted && 'italic text-siswa-text-muted',
          )}
        >
          {item.pertanyaan}
        </p>
      ) : (
        <p className="text-sm italic text-siswa-text-muted">(tanpa teks)</p>
      )}

      {!isDeleted ? (
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
                  'flex gap-3 rounded-siswa border-2 border-siswa-border-soft p-3 transition-colors',
                  isCorrect && 'border-siswa-border bg-siswa-green/40',
                  wronglyPicked && 'border-siswa-border bg-rose-100',
                )}
              >
                <span
                  className={cn(
                    'mt-0.5 inline-flex size-5 items-center justify-center rounded-full border-2 text-xs font-bold uppercase',
                    isCorrect &&
                      'border-siswa-border bg-siswa-green text-emerald-800',
                    wronglyPicked &&
                      'border-siswa-border bg-rose-200 text-rose-800',
                    !isCorrect && !wronglyPicked && 'border-siswa-border-soft',
                  )}
                >
                  {label}
                </span>
                <div className="min-w-0 flex-1 space-y-1">
                  {text ? (
                    <span className="whitespace-pre-wrap text-sm">{text}</span>
                  ) : (
                    <span className="text-xs italic text-siswa-text-muted">
                      (tanpa teks)
                    </span>
                  )}
                  <div className="flex flex-wrap gap-2 text-xs font-semibold">
                    {isCorrect ? (
                      <span className="text-emerald-700">Jawaban benar</span>
                    ) : null}
                    {isPicked ? (
                      <span
                        className={cn(
                          wronglyPicked ? 'text-rose-700' : 'text-emerald-700',
                        )}
                      >
                        Pilihan kamu
                      </span>
                    ) : null}
                  </div>
                </div>
              </li>
            );
          })}
        </ul>
      ) : null}
    </li>
  );
}
