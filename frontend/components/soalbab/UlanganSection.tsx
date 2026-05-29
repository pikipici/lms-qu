'use client';

/**
 * UlanganSection — orchestrator state machine untuk ulangan flow (Task 5.G.2).
 *
 * State:
 *   - 'lobby'    : tampil setting + history
 *   - 'playing'  : tampil UlanganPlayer dengan hasilID
 *   - 'result'   : tampil rekap nilai post-submit + tombol Lihat Pembahasan
 *   - 'review'   : tampil UlanganReview
 *
 * Transitions:
 *   lobby --start/resume--> playing
 *   playing --done--------> result
 *   playing --abort-------> lobby
 *   result --review-------> review
 *   result --back---------> lobby
 *   lobby --review--------> review (klik history)
 *   review --back---------> lobby
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, CheckCircle2, Eye, Loader2, RotateCcw } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  friendlyUlanganError,
  getSiswaHasilList,
  getSiswaUlanganLobby,
  startUlangan,
  type UlanganSubmitResult,
} from '@/lib/soalbab-ulangan-api';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';
import { UlanganLobby } from '@/components/soalbab/UlanganLobby';
import { UlanganPlayer } from '@/components/soalbab/UlanganPlayer';
import { UlanganReview } from '@/components/soalbab/UlanganReview';
import {
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardDescription,
  SiswaCardHeader,
  SiswaCardTitle,
} from '@/components/siswa-ui';

type Mode =
  | { kind: 'lobby' }
  | { kind: 'playing'; hasilID: string }
  | { kind: 'result'; summary: UlanganSubmitResult }
  | { kind: 'review'; hasilID: string };

export interface UlanganSectionProps {
  babID: string;
  disabled?: boolean;
}

export function UlanganSection({ babID, disabled }: UlanganSectionProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [mode, setMode] = React.useState<Mode>({ kind: 'lobby' });

  const lobbyKey = React.useMemo(() => ['siswa', 'ulangan', 'lobby', babID] as const, [babID]);
  const historyKey = React.useMemo(() => ['siswa', 'bab', 'hasil', babID] as const, [babID]);

  const lobbyQuery = useQuery({
    queryKey: lobbyKey,
    queryFn: () => getSiswaUlanganLobby(babID),
    staleTime: 30_000,
    retry: (count, err) => {
      if (err instanceof ApiError) {
        if (err.status === 403 || err.status === 404) return false;
      }
      return count < 2;
    },
    enabled: mode.kind === 'lobby' || mode.kind === 'result',
  });

  const historyQuery = useQuery({
    queryKey: historyKey,
    queryFn: () => getSiswaHasilList(babID),
    staleTime: 15_000,
    retry: (count, err) => {
      if (err instanceof ApiError) {
        if (err.status === 403 || err.status === 404) return false;
      }
      return count < 2;
    },
    enabled: mode.kind === 'lobby' || mode.kind === 'result',
  });

  const startMu = useMutation({
    mutationFn: () => startUlangan(babID),
    onSuccess: ({ hasil }) => {
      setMode({ kind: 'playing', hasilID: hasil.hasil_id });
      // Invalidate history so attempt count refreshes when we come back.
      queryClient.invalidateQueries({ queryKey: historyKey });
    },
    onError: (err) => {
      const msg =
        err instanceof ApiError
          ? friendlyUlanganError(err, 'start')
          : 'Gagal memulai ulangan.';
      toast({ title: 'Gagal mulai', description: msg, variant: 'destructive' });
    },
  });

  // ---- playing ----
  if (mode.kind === 'playing') {
    return (
      <UlanganPlayer
        hasilID={mode.hasilID}
        disabled={disabled}
        onDone={(summary) => {
          setMode({ kind: 'result', summary });
          queryClient.invalidateQueries({ queryKey: lobbyKey });
          queryClient.invalidateQueries({ queryKey: historyKey });
        }}
        onAbort={() => {
          setMode({ kind: 'lobby' });
          queryClient.invalidateQueries({ queryKey: historyKey });
        }}
      />
    );
  }

  // ---- result ----
  if (mode.kind === 'result') {
    const setting = lobbyQuery.data?.setting;
    const summary = mode.summary;
    const reviewable =
      summary.izinkan_review &&
      (!summary.dapat_review_at || new Date(summary.dapat_review_at).getTime() <= Date.now());
    const reviewLockMsg = !summary.izinkan_review
      ? 'Guru tidak mengaktifkan review untuk ulangan ini.'
      : summary.dapat_review_at && new Date(summary.dapat_review_at).getTime() > Date.now()
        ? `Pembahasan dibuka ${new Date(summary.dapat_review_at).toLocaleString('id-ID', {
            dateStyle: 'medium',
            timeStyle: 'short',
          })}.`
        : null;

    const persen =
      summary.jawaban_total === 0
        ? 0
        : Math.round((summary.jawaban_benar_count / summary.jawaban_total) * 100);

    return (
      <div className="space-y-4">
        <SiswaCard tone="ulangan" shadow="md">
          <SiswaCardHeader>
            <div className="flex items-center gap-2">
              <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                <CheckCircle2 className="size-4 text-emerald-600" strokeWidth={2.5} />
              </span>
              <SiswaCardTitle>Ulangan Selesai</SiswaCardTitle>
            </div>
            <SiswaCardDescription>
              {summary.already_submitted
                ? 'Attempt ini sudah disubmit sebelumnya. Berikut nilai yang tersimpan.'
                : 'Sip, jawaban kamu sudah dinilai. Berikut rekapnya.'}
            </SiswaCardDescription>
          </SiswaCardHeader>
          <SiswaCardBody className="space-y-4">
            <div className="grid gap-3 sm:grid-cols-4">
              <SummaryCell label="Total soal" value={summary.jawaban_total} />
              <SummaryCell label="Benar" value={summary.jawaban_benar_count} accent="emerald" />
              <SummaryCell
                label="Salah"
                value={Math.max(0, summary.jawaban_total - summary.jawaban_benar_count)}
                accent="rose"
              />
              <SummaryCell label="Nilai" value={summary.nilai_total} accent="primary" />
            </div>
            <div className="flex items-center justify-between rounded-siswa border-2 border-siswa-border-soft bg-siswa-cream/40 p-3 text-sm">
              <span className="text-siswa-text-muted">Persentase benar:</span>
              <strong className="siswa-display text-base">{persen}%</strong>
            </div>
            <div className="flex flex-wrap gap-2">
              <SiswaButton
                type="button"
                tone="primary"
                onClick={() => setMode({ kind: 'review', hasilID: summary.hasil_id })}
                disabled={!reviewable}
                title={reviewable ? 'Lihat pembahasan jawaban' : reviewLockMsg ?? undefined}
              >
                <Eye className="size-4" strokeWidth={2.5} />
                Lihat pembahasan
              </SiswaButton>
              <SiswaButton type="button" tone="surface" onClick={() => setMode({ kind: 'lobby' })}>
                <ArrowLeft className="size-4" />
                Kembali ke lobi
              </SiswaButton>
            </div>
            {!reviewable && reviewLockMsg ? (
              <p className="text-xs text-siswa-text-muted">{reviewLockMsg}</p>
            ) : null}
            {setting && setting.batas_attempt > 0 && historyQuery.data ? (
              <p className="text-xs text-siswa-text-muted">
                Attempt terpakai:{' '}
                {historyQuery.data.hasil.attempt_count} / {setting.batas_attempt}
              </p>
            ) : null}
          </SiswaCardBody>
        </SiswaCard>
      </div>
    );
  }

  // ---- review ----
  if (mode.kind === 'review') {
    return (
      <UlanganReview
        hasilID={mode.hasilID}
        onBack={() => setMode({ kind: 'lobby' })}
      />
    );
  }

  // ---- lobby ----
  if (lobbyQuery.isPending || historyQuery.isPending) {
    return (
      <SiswaCard tone="surface" shadow="sm">
        <SiswaCardHeader>
          <SiswaCardTitle>Memuat info ulangan…</SiswaCardTitle>
        </SiswaCardHeader>
        <SiswaCardBody>
          <div className="h-32 animate-pulse rounded-siswa siswa-border bg-siswa-surface/60" />
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  if (lobbyQuery.isError || historyQuery.isError) {
    const apiErr =
      lobbyQuery.error instanceof ApiError
        ? lobbyQuery.error
        : historyQuery.error instanceof ApiError
          ? historyQuery.error
          : null;
    const msg = apiErr ? friendlyUlanganError(apiErr, 'lobby') : 'Gagal memuat info ulangan.';
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>Gagal memuat ulangan</SiswaCardTitle>
          <SiswaCardDescription>{msg}</SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          <SiswaButton
            type="button"
            size="sm"
            tone="surface"
            onClick={() => {
              lobbyQuery.refetch();
              historyQuery.refetch();
            }}
          >
            <RotateCcw className="size-4" />
            Coba lagi
          </SiswaButton>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  const setting = lobbyQuery.data!.setting;
  const hasilList = historyQuery.data!.hasil;
  const startError = startMu.error instanceof ApiError ? startMu.error : null;

  return (
    <UlanganLobby
      setting={setting}
      hasilList={hasilList}
      starting={startMu.isPending}
      startError={startError}
      disabled={disabled}
      refreshing={lobbyQuery.isFetching || historyQuery.isFetching}
      onRefresh={() => {
        lobbyQuery.refetch();
        historyQuery.refetch();
      }}
      onStart={() => startMu.mutate()}
      onResume={(hasilID) => setMode({ kind: 'playing', hasilID })}
      onReview={(hasilID) => setMode({ kind: 'review', hasilID })}
    />
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
