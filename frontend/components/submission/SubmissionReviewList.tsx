'use client';

/**
 * SubmissionReviewList — guru-side list submissions per tugas.
 *
 * Props:
 *   - tugasID: scope.
 *   - kelasID: untuk back-link kalau perlu.
 *   - tugas (optional): pass dari parent untuk avoid double-fetch + late penalty hint.
 *
 * UX:
 *   - Status filter tabs (all/submitted/graded) — list re-fetches per filter.
 *   - Card per row: badge status (submitted=biru, graded=ijo) + LATE badge merah
 *     kalau is_late + nilai (kalau graded) + tombol "Beri Nilai" (open dialog).
 *   - Sort: server returns submitted_at DESC.
 *
 * Note: Submission tidak prefetch siswa nama — defer to v0.10 (butuh
 * /admin/users batch lookup atau Preload Siswa di service). MVP tampil siswa_id.
 */

import * as React from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  CheckCircle2,
  Clock,
  Eye,
  Loader2,
  RotateCcw,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { cn } from '@/lib/utils';

import {
  type Submission,
  type SubmissionStatus,
  formatSubmissionTimestamp,
  friendlySubmissionError,
  listTugasSubmissions,
  statusLabel,
} from '@/lib/submission-api';

import { GradeSubmissionDialog } from './GradeSubmissionDialog';

const STATUS_FILTERS: Array<{ key: 'all' | SubmissionStatus; label: string }> = [
  { key: 'all', label: 'Semua' },
  { key: 'submitted', label: 'Belum dinilai' },
  { key: 'graded', label: 'Sudah dinilai' },
];

interface SubmissionReviewListProps {
  tugasID: string;
  /** Late penalty hint untuk dialog grading. Default 0. */
  penaltyPersen?: number;
  /** Tugas judul untuk dialog header. */
  tugasJudul?: string;
}

export function SubmissionReviewList({
  tugasID,
  penaltyPersen = 0,
  tugasJudul,
}: SubmissionReviewListProps) {
  const [filter, setFilter] = React.useState<'all' | SubmissionStatus>('all');
  const [gradeTarget, setGradeTarget] = React.useState<Submission | null>(null);

  const query = useQuery({
    queryKey: ['guru', 'tugas', tugasID, 'submissions', filter],
    queryFn: () =>
      listTugasSubmissions(tugasID, {
        status: filter === 'all' ? undefined : filter,
      }),
  });

  return (
    <>
      <Card>
        <CardHeader className="flex flex-col items-stretch justify-between gap-3 sm:flex-row sm:items-start">
          <div className="space-y-1">
            <CardTitle className="text-base">Submission siswa</CardTitle>
            <CardDescription>
              Klik &quot;Beri Nilai&quot; untuk grade. Resubmit dari siswa
              auto-update versi di list ini.
            </CardDescription>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => query.refetch()}
            disabled={query.isFetching}
          >
            <RotateCcw className="size-4" />
            Refresh
          </Button>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex max-w-full items-center gap-1 self-start overflow-x-auto rounded-md border bg-muted/40 p-0.5">
            {STATUS_FILTERS.map((f) => {
              const active = filter === f.key;
              return (
                <button
                  key={f.key}
                  type="button"
                  onClick={() => setFilter(f.key)}
                  className={cn(
                    'rounded px-2 py-1 text-xs transition-colors',
                    active
                      ? 'bg-background shadow-sm'
                      : 'text-muted-foreground hover:bg-background/60',
                  )}
                >
                  {f.label}
                </button>
              );
            })}
          </div>

          {query.isPending && (
            <div className="space-y-2">
              {[0, 1, 2].map((i) => (
                <div
                  key={i}
                  className="h-16 animate-pulse rounded-md border bg-muted/40"
                />
              ))}
            </div>
          )}

          {query.isError && (
            <ErrorBlock
              err={query.error}
              onRetry={() => query.refetch()}
            />
          )}

          {query.isSuccess && (query.data?.items?.length ?? 0) === 0 && (
            <p className="rounded-md border border-dashed py-6 text-center text-sm text-muted-foreground">
              {filter === 'all'
                ? 'Belum ada siswa yang submit.'
                : filter === 'submitted'
                  ? 'Tidak ada submission yang belum dinilai.'
                  : 'Belum ada submission yang sudah dinilai.'}
            </p>
          )}

          {query.isSuccess && (query.data?.items?.length ?? 0) > 0 && (
            <ul className="space-y-2">
              {query.data!.items.map((s) => (
                <SubmissionRow
                  key={s.id}
                  submission={s}
                  onGrade={() => setGradeTarget(s)}
                />
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      {gradeTarget && (
        <GradeSubmissionDialog
          open={!!gradeTarget}
          onOpenChange={(open) => {
            if (!open) setGradeTarget(null);
          }}
          submission={gradeTarget}
          tugasID={tugasID}
          tugasJudul={tugasJudul}
          tugasPenaltyPersen={penaltyPersen}
        />
      )}
    </>
  );
}

function SubmissionRow({
  submission,
  onGrade,
}: {
  submission: Submission;
  onGrade: () => void;
}) {
  const isGraded = submission.status === 'graded';
  return (
    <li className="rounded-md border bg-background">
      <div className="flex flex-col gap-3 px-3 py-2.5 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <code className="truncate text-xs">
              {submission.siswa_id.slice(0, 8)}…
            </code>
            <span
              className={cn(
                'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
                isGraded
                  ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
                  : 'bg-sky-100 text-sky-700 dark:bg-sky-900/40 dark:text-sky-300',
              )}
            >
              {isGraded ? (
                <CheckCircle2 className="size-3" />
              ) : (
                <Clock className="size-3" />
              )}
              {statusLabel(submission.status)}
            </span>
            {submission.is_late && (
              <span className="inline-flex items-center gap-1 rounded-full bg-rose-100 px-2 py-0.5 text-[10px] font-medium text-rose-700 dark:bg-rose-900/40 dark:text-rose-300">
                LATE
              </span>
            )}
          </div>
          <p className="text-xs text-muted-foreground">
            v{submission.version} · {formatSubmissionTimestamp(submission.submitted_at)}
            {isGraded && submission.nilai_setelah_penalty != null && (
              <>
                {' · '}Nilai{' '}
                <span className="font-semibold text-foreground">
                  {submission.nilai_setelah_penalty.toFixed(2)}
                </span>
                {submission.is_late &&
                  submission.penalty_persen_applied != null &&
                  submission.penalty_persen_applied > 0 && (
                    <>
                      {' '}
                      (asli {submission.nilai_asli?.toFixed(2)} -{' '}
                      {submission.penalty_persen_applied}%)
                    </>
                  )}
              </>
            )}
          </p>
          {submission.catatan && (
            <p className="line-clamp-2 text-xs text-muted-foreground">
              {submission.catatan}
            </p>
          )}
        </div>
        <Button
          type="button"
          size="sm"
          variant={isGraded ? 'outline' : 'default'}
          onClick={onGrade}
        >
          <Eye className="size-3.5" />
          {isGraded ? 'Lihat' : 'Beri Nilai'}
        </Button>
      </div>
    </li>
  );
}

function ErrorBlock({
  err,
  onRetry,
}: {
  err: unknown;
  onRetry: () => void;
}) {
  const apiErr = err instanceof ApiError ? err : null;
  const message = apiErr
    ? friendlySubmissionError(apiErr, 'list')
    : (err as Error)?.message ?? 'Gagal memuat data.';
  return (
    <div className="space-y-2 rounded-md border border-destructive/30 bg-destructive/5 p-3">
      <p className="text-sm text-destructive">{message}</p>
      <Button type="button" variant="outline" size="sm" onClick={onRetry}>
        <Loader2 className="size-3.5" />
        Coba lagi
      </Button>
    </div>
  );
}
