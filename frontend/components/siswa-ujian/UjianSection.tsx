'use client';

/**
 * UjianSection — orchestrator state machine untuk siswa ujian flow (Task 6.G.2).
 *
 * Mirror SoalBab UlanganSection pattern (5.G.2 commit `6c10d19`) tapi
 * untuk single-ujian page (vs cross-kelas catalog di 6.G.1).
 *
 * State:
 *   - 'lobby'    : metadata ujian + history attempt + start/resume CTA
 *   - 'playing'  : UjianPlayer dengan hasilID
 *   - 'result'   : summary card post-submit + tombol Lihat Pembahasan
 *   - 'review'   : UjianReview
 *
 * Transitions:
 *   lobby --start/resume--> playing
 *   playing --done--------> result
 *   playing --abort-------> lobby
 *   result --review-------> review
 *   result --back---------> lobby
 *   lobby --review--------> review (klik history attempt selesai)
 *   review --back---------> lobby
 *
 * Auto-resume: lobby query detect inflight attempt (status='berlangsung'),
 * tampil tombol "Lanjutkan" yang langsung set mode=playing dengan hasilID
 * tersebut. Server-side enforce single-attempt via partial-unique
 * (ujian_id, siswa_id) WHERE deleted_at IS NULL — start endpoint resume
 * existing kalau ketemu inflight.
 */

import * as React from 'react';
import Link from 'next/link';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  CalendarRange,
  CheckCircle2,
  Clock,
  Eye,
  Loader2,
  ListChecks,
  PlayCircle,
  Repeat,
  RotateCcw,
  ShieldCheck,
  Timer,
  Trophy,
  XCircle,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import { getUjian, type Ujian } from '@/lib/ujian-api';
import {
  friendlySiswaUjianError,
  listSiswaUjianHasil,
  startSiswaUjian,
  type SiswaUjianHasilListResult,
  type UjianHasilSummary,
  type UjianSubmitResult,
} from '@/lib/siswa-ujian-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { UjianPlayer } from '@/components/siswa-ujian/UjianPlayer';
import { UjianReview } from '@/components/siswa-ujian/UjianReview';
import {
  SiswaBadge,
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
  | { kind: 'result'; summary: UjianSubmitResult }
  | { kind: 'review'; hasilID: string };

export interface UjianSectionProps {
  ujianID: string;
  kelasID: string;
}

export function UjianSection({ ujianID, kelasID }: UjianSectionProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [mode, setMode] = React.useState<Mode>({ kind: 'lobby' });

  const ujianKey = React.useMemo(
    () => ['siswa', 'ujian', 'detail', ujianID] as const,
    [ujianID],
  );
  const hasilKey = React.useMemo(
    () => ['siswa', 'ujian', 'hasil', kelasID] as const,
    [kelasID],
  );

  const ujianQuery = useQuery({
    queryKey: ujianKey,
    queryFn: () => getUjian(ujianID),
    staleTime: 30_000,
    retry: (count, err) => {
      if (err instanceof ApiError) {
        if (err.status === 403 || err.status === 404) return false;
      }
      return count < 2;
    },
    enabled: mode.kind === 'lobby' || mode.kind === 'result',
  });

  const hasilQuery = useQuery({
    queryKey: hasilKey,
    queryFn: () => listSiswaUjianHasil(kelasID),
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
    mutationFn: () => startSiswaUjian(ujianID),
    onSuccess: ({ hasil }) => {
      setMode({ kind: 'playing', hasilID: hasil.hasil_id });
      queryClient.invalidateQueries({ queryKey: hasilKey });
    },
    onError: (err) => {
      const msg =
        err instanceof ApiError
          ? friendlySiswaUjianError(err, 'start')
          : 'Gagal memulai ujian.';
      toast({
        title: 'Gagal mulai',
        description: msg,
        variant: 'destructive',
      });
    },
  });

  // Helper: filter hasil aggregate ke ujian ini saja.
  const myItems = React.useMemo<UjianHasilSummary[]>(() => {
    const all = hasilQuery.data?.hasil.items ?? [];
    return all.filter((h) => h.ujian_id === ujianID);
  }, [hasilQuery.data, ujianID]);

  const inflight = React.useMemo(
    () => myItems.find((h) => h.status === 'berlangsung') ?? null,
    [myItems],
  );

  // ---- playing ----
  if (mode.kind === 'playing') {
    return (
      <div className="space-y-3">
        <SiswaButton asChild tone="ghost" size="sm" className="-ml-2">
          <Link href="/siswa/ujian">
            <ArrowLeft className="size-4" />
            Daftar ujian
          </Link>
        </SiswaButton>
        <UjianPlayer
          hasilID={mode.hasilID}
          onDone={(summary) => {
            setMode({ kind: 'result', summary });
            queryClient.invalidateQueries({ queryKey: hasilKey });
          }}
          onAbort={() => {
            setMode({ kind: 'lobby' });
            queryClient.invalidateQueries({ queryKey: hasilKey });
          }}
        />
      </div>
    );
  }

  // ---- review ----
  if (mode.kind === 'review') {
    return (
      <div className="space-y-3">
        <SiswaButton
          tone="ghost"
          size="sm"
          className="-ml-2"
          onClick={() => setMode({ kind: 'lobby' })}
        >
          <ArrowLeft className="size-4" />
          Kembali ke lobi
        </SiswaButton>
        <UjianReview
          hasilID={mode.hasilID}
          onBack={() => setMode({ kind: 'lobby' })}
        />
      </div>
    );
  }

  // ---- result (post-submit summary) ----
  if (mode.kind === 'result') {
    const ujian = ujianQuery.data?.ujian;
    return (
      <ResultPanel
        summary={mode.summary}
        ujian={ujian}
        onReview={() => {
          setMode({ kind: 'review', hasilID: mode.summary.hasil_id });
        }}
        onBackLobby={() => setMode({ kind: 'lobby' })}
      />
    );
  }

  // ---- lobby ----
  if (ujianQuery.isPending || hasilQuery.isPending) {
    return (
      <div className="space-y-3">
        <div className="h-7 w-32 animate-pulse rounded bg-siswa-text/10" />
        <div className="h-48 animate-pulse rounded-siswa siswa-border bg-siswa-surface/60" />
      </div>
    );
  }

  if (ujianQuery.isError) {
    const apiErr =
      ujianQuery.error instanceof ApiError ? ujianQuery.error : null;
    const msg = apiErr
      ? friendlySiswaUjianError(apiErr, 'list')
      : 'Gagal memuat ujian.';
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
              onClick={() => ujianQuery.refetch()}
            >
              <RotateCcw className="size-4" />
              Coba lagi
            </SiswaButton>
            <SiswaButton asChild type="button" size="sm" tone="ghost">
              <Link href="/siswa/ujian">
                <ArrowLeft className="size-4" />
                Daftar ujian
              </Link>
            </SiswaButton>
          </div>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  const ujian = ujianQuery.data!.ujian;

  return (
    <LobbyPanel
      ujian={ujian}
      myItems={myItems}
      inflight={inflight}
      onStart={() => startMu.mutate()}
      starting={startMu.isPending}
      startError={
        startMu.error instanceof ApiError ? startMu.error : null
      }
      onReview={(hasilID) => setMode({ kind: 'review', hasilID })}
      onResume={(hasilID) => setMode({ kind: 'playing', hasilID })}
    />
  );
}

// ---------- Lobby panel ----------

interface LobbyPanelProps {
  ujian: Ujian;
  myItems: UjianHasilSummary[];
  inflight: UjianHasilSummary | null;
  onStart: () => void;
  starting: boolean;
  startError: ApiError | null;
  onReview: (hasilID: string) => void;
  onResume: (hasilID: string) => void;
}

function LobbyPanel({
  ujian,
  myItems,
  inflight,
  onStart,
  starting,
  startError,
  onReview,
  onResume,
}: LobbyPanelProps) {
  const [now, setNow] = React.useState(() => Date.now());
  React.useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);

  const window_ = computeWindow(now, ujian);
  const jumlahSoal = jumlahSoalEstimasi(ujian);

  const attemptCount = myItems.filter((h) => h.status === 'selesai').length;
  const usedAttemptCount = myItems.filter(
    (h) => h.status !== 'dibatalkan',
  ).length;
  const cancelledCount = myItems.filter(
    (h) => h.status === 'dibatalkan',
  ).length;
  const attemptLimit = ujian.batas_attempt ?? 1;
  const remainingAttempts = ujian.attempt_unlimited
    ? null
    : Math.max(0, attemptLimit - usedAttemptCount);
  const attemptsExhausted = remainingAttempts === 0;
  const bestNilai = bestNilaiOf(myItems);

  return (
    <div className="space-y-4">
      <SiswaButton asChild tone="ghost" size="sm" className="-ml-2">
        <Link href="/siswa/ujian">
          <ArrowLeft className="size-4" />
          Daftar ujian
        </Link>
      </SiswaButton>

      <SiswaCard tone="ulangan" shadow="md">
        <SiswaCardHeader>
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div className="min-w-0 space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <SiswaCardTitle>{ujian.judul}</SiswaCardTitle>
                <WindowBadge state={window_.state} />
              </div>
              <SiswaCardDescription>
                {ujian.deskripsi || 'Ulangan harian — dinilai. Pastikan koneksi stabil sebelum mulai.'}
              </SiswaCardDescription>
            </div>
          </div>
        </SiswaCardHeader>
        <SiswaCardBody className="space-y-4">
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
            <InfoTile
              icon={Timer}
              label="Durasi"
              value={`${ujian.durasi_menit} menit`}
            />
            <InfoTile
              icon={ListChecks}
              label="Jumlah soal"
              value={jumlahSoal != null ? `${jumlahSoal} soal` : '—'}
            />
            <InfoTile
              icon={Repeat}
              label="Kesempatan"
              value={
                ujian.attempt_unlimited
                  ? 'Tidak terbatas'
                  : `${remainingAttempts} tersisa dari ${attemptLimit}${cancelledCount > 0 ? ` · ${cancelledCount} batal` : ''}`
              }
            />
            <InfoTile
              icon={Trophy}
              label="Nilai terbaik"
              value={bestNilai != null ? formatNilai(bestNilai) : '—'}
              accent={bestNilai != null ? 'emerald' : 'default'}
            />
          </div>

          <div className="rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface p-3">
            <div className="flex items-start gap-2 text-xs text-siswa-text-muted">
              <CalendarRange className="mt-0.5 size-3.5 shrink-0" />
              <div className="space-y-1">
                <div className="font-semibold text-siswa-text">
                  {formatRangeJakarta(ujian)}
                </div>
                <CountdownLine info={window_} now={now} />
              </div>
            </div>
          </div>

          <ReviewPolicyNote ujian={ujian} />

          {startError ? (
            <div className="flex items-start gap-2 rounded-siswa border-2 border-siswa-danger bg-rose-50 p-3 text-sm font-semibold">
              <AlertCircle className="mt-0.5 size-4 shrink-0" strokeWidth={2.5} />
              <span>{friendlySiswaUjianError(startError, 'start')}</span>
            </div>
          ) : null}

          <div className="flex flex-wrap gap-2">
            {inflight ? (
              <SiswaButton
                type="button"
                tone="primary"
                onClick={() => onResume(inflight.hasil_id)}
                disabled={starting}
              >
                <PlayCircle className="size-4" strokeWidth={2.5} />
                Lanjutkan ujian
              </SiswaButton>
            ) : (
              <PrimaryStartButton
                window={window_}
                attemptCount={attemptCount}
                attemptsExhausted={attemptsExhausted}
                onStart={onStart}
                starting={starting}
                now={now}
              />
            )}
          </div>
        </SiswaCardBody>
      </SiswaCard>

      <SiswaCard tone="surface" shadow="sm">
        <SiswaCardHeader>
          <SiswaCardTitle>Riwayat attempt</SiswaCardTitle>
          <SiswaCardDescription>
            {myItems.length === 0
              ? 'Belum ada attempt untuk ujian ini.'
              : `Total ${attemptCount} attempt selesai${cancelledCount > 0 ? `, ${cancelledCount} dibatalkan guru` : ''}.`}
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          {myItems.length === 0 ? (
            <p className="text-sm text-siswa-text-muted">
              Begitu lu mulai ujian, attempt akan tampil di sini.
            </p>
          ) : (
            <ul className="space-y-2">
              {myItems
                .slice()
                .sort((a, b) => b.mulai_at.localeCompare(a.mulai_at))
                .map((h) => (
                  <HistoryRow
                    key={h.hasil_id}
                    hasil={h}
                    ujian={ujian}
                    now={now}
                    onReview={() => onReview(h.hasil_id)}
                    onResume={() => onResume(h.hasil_id)}
                  />
                ))}
            </ul>
          )}
        </SiswaCardBody>
      </SiswaCard>
    </div>
  );
}

// ---------- Result panel ----------

interface ResultPanelProps {
  summary: UjianSubmitResult;
  ujian?: Ujian;
  onReview: () => void;
  onBackLobby: () => void;
}

function ResultPanel({
  summary,
  ujian,
  onReview,
  onBackLobby,
}: ResultPanelProps) {
  const total = summary.jawaban_total;
  const benar = summary.jawaban_benar_count;
  const persen = total === 0 ? 0 : Math.round((benar / total) * 100);
  const reviewable = canReviewResult(summary, ujian);
  const reviewMsg = reviewable ? null : reviewLockedMsg(summary, ujian);
  return (
    <div className="space-y-4">
      <SiswaCard tone="ulangan" shadow="md">
        <SiswaCardHeader>
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div>
              <SiswaCardTitle>
                <CheckCircle2 className="mr-1 inline size-5 text-emerald-600" strokeWidth={2.5} />
                Ujian selesai
              </SiswaCardTitle>
              <SiswaCardDescription>
                {summary.already_submitted
                  ? 'Attempt ini sudah disubmit sebelumnya — menampilkan rekap nilai.'
                  : 'Nilai sudah keluar. Lu bisa lihat pembahasan kalau guru aktivasi.'}
              </SiswaCardDescription>
            </div>
          </div>
        </SiswaCardHeader>
        <SiswaCardBody className="space-y-3">
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
              value={Number(summary.nilai_total).toFixed(0)}
              accent="primary"
            />
          </div>
          <div className="flex items-center justify-between rounded-siswa border-2 border-siswa-border-soft bg-siswa-cream/40 p-3 text-sm">
            <span className="text-siswa-text-muted">Persentase benar:</span>
            <strong className="siswa-display text-base">{persen}%</strong>
          </div>
          {reviewMsg ? (
            <p className="text-xs text-siswa-text-muted">{reviewMsg}</p>
          ) : null}
          <div className="flex flex-wrap gap-2">
            {reviewable ? (
              <SiswaButton tone="primary" onClick={onReview}>
                <Eye className="size-4" strokeWidth={2.5} />
                Lihat pembahasan
              </SiswaButton>
            ) : null}
            <SiswaButton tone="surface" onClick={onBackLobby}>
              <ArrowLeft className="size-4" />
              Kembali ke lobi
            </SiswaButton>
          </div>
        </SiswaCardBody>
      </SiswaCard>
    </div>
  );
}

// ---------- Sub-components ----------

interface InfoTileProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
  accent?: 'default' | 'emerald';
}

function InfoTile({ icon: Icon, label, value, accent = 'default' }: InfoTileProps) {
  return (
    <div
      className={cn(
        'rounded-siswa border-2 border-siswa-border bg-siswa-surface p-3',
        accent === 'emerald' && 'bg-siswa-green/40',
      )}
    >
      <div className="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-siswa-text-muted">
        <Icon className="size-3.5" />
        {label}
      </div>
      <div className="siswa-display mt-1 text-sm font-bold">{value}</div>
    </div>
  );
}

type WindowState = 'mendatang' | 'aktif' | 'berakhir' | 'tanpa-window';

function computeWindow(now: number, ujian: Ujian) {
  const startMs = ujian.waktu_mulai ? new Date(ujian.waktu_mulai).getTime() : null;
  const endMs = ujian.waktu_selesai ? new Date(ujian.waktu_selesai).getTime() : null;
  let state: WindowState;
  let target: number | undefined;
  if (startMs && now < startMs) {
    state = 'mendatang';
    target = startMs;
  } else if (endMs && now > endMs) {
    state = 'berakhir';
    target = endMs;
  } else if (startMs || endMs) {
    state = 'aktif';
    target = endMs ?? undefined;
  } else {
    state = 'tanpa-window';
  }
  return { state, target };
}

function WindowBadge({ state }: { state: WindowState }) {
  const map: Record<
    WindowState,
    {
      label: string;
      tone: React.ComponentProps<typeof SiswaBadge>['tone'];
      icon: React.ComponentType<{ className?: string }>;
    }
  > = {
    mendatang: { label: 'Mendatang', tone: 'warning', icon: Clock },
    aktif: { label: 'Aktif', tone: 'success', icon: PlayCircle },
    berakhir: { label: 'Berakhir', tone: 'neutral', icon: XCircle },
    'tanpa-window': { label: 'Tersedia', tone: 'blue', icon: PlayCircle },
  };
  const cfg = map[state];
  const Icon = cfg.icon;
  return (
    <SiswaBadge tone={cfg.tone}>
      <Icon className="size-3" />
      {cfg.label}
    </SiswaBadge>
  );
}

function CountdownLine({
  info,
  now,
}: {
  info: { state: WindowState; target?: number };
  now: number;
}) {
  if (!info.target) {
    if (info.state === 'tanpa-window') {
      return <div>Tidak ada batas waktu — mulai kapan saja.</div>;
    }
    return null;
  }
  const delta = Math.max(0, info.target - now);
  if (info.state === 'mendatang') {
    return <div>Dimulai dalam {formatCountdown(delta)}.</div>;
  }
  if (info.state === 'aktif') {
    return <div>Window ditutup dalam {formatCountdown(delta)}.</div>;
  }
  return <div>Sudah berakhir.</div>;
}

function ReviewPolicyNote({ ujian }: { ujian: Ujian }) {
  if (!ujian.izinkan_review_setelah_submit) {
    return (
      <p className="rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface/70 p-2 text-xs text-siswa-text-muted">
        Guru tidak mengaktifkan pembahasan untuk ujian ini.
      </p>
    );
  }
  if (ujian.waktu_buka_review) {
    const t = new Date(ujian.waktu_buka_review).toLocaleString('id-ID', {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: 'Asia/Jakarta',
    });
    return (
      <p className="rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface/70 p-2 text-xs text-siswa-text-muted">
        Pembahasan dibuka mulai <strong className="text-siswa-text">{t}</strong>.
      </p>
    );
  }
  return (
    <p className="rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface/70 p-2 text-xs text-siswa-text-muted">
      Pembahasan tersedia langsung setelah lu submit attempt.
    </p>
  );
}

interface PrimaryStartButtonProps {
  window: { state: WindowState; target?: number };
  attemptCount: number;
  attemptsExhausted: boolean;
  onStart: () => void;
  starting: boolean;
  now: number;
}

function PrimaryStartButton({
  window,
  attemptCount,
  attemptsExhausted,
  onStart,
  starting,
  now,
}: PrimaryStartButtonProps) {
  if (window.state === 'mendatang' && window.target) {
    return (
      <SiswaButton tone="surface" disabled>
        <Clock className="size-4" />
        Mulai dalam {formatCountdown(Math.max(0, window.target - now))}
      </SiswaButton>
    );
  }
  if (window.state === 'berakhir') {
    return (
      <SiswaButton tone="surface" disabled>
        <XCircle className="size-4" />
        Window berakhir
      </SiswaButton>
    );
  }
  if (attemptsExhausted) {
    return (
      <SiswaButton tone="surface" disabled>
        <XCircle className="size-4" />
        Kesempatan habis
      </SiswaButton>
    );
  }
  return (
    <SiswaButton tone="primary" onClick={onStart} disabled={starting}>
      {starting ? (
        <Loader2 className="size-4 animate-spin" />
      ) : (
        <PlayCircle className="size-4" strokeWidth={2.5} />
      )}
      {attemptCount > 0 ? 'Mulai ujian baru' : 'Mulai ujian'}
      <ArrowRight className="size-4" strokeWidth={2.5} />
    </SiswaButton>
  );
}

interface HistoryRowProps {
  hasil: UjianHasilSummary;
  ujian: Ujian;
  now: number;
  onReview: () => void;
  onResume: () => void;
}

function HistoryRow({ hasil, ujian, now, onReview, onResume }: HistoryRowProps) {
  const mulaiAt = hasil.mulai_at ? new Date(hasil.mulai_at) : null;
  const reviewable = canReviewHasil(hasil, ujian, now);
  const reviewLockReason = reviewLockMessage(hasil, ujian, now);

  return (
    <li className="flex flex-wrap items-start justify-between gap-3 rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface p-3">
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="siswa-display text-sm font-bold">
            Attempt #{hasil.attempt_no}
          </span>
          <StatusBadge status={hasil.status} />
        </div>
        <div className="text-xs text-siswa-text-muted">
          {mulaiAt
            ? `Mulai ${mulaiAt.toLocaleString('id-ID', {
                dateStyle: 'medium',
                timeStyle: 'short',
                timeZone: 'Asia/Jakarta',
              })}`
            : '—'}
          {hasil.status === 'selesai' &&
          hasil.jawaban_total != null &&
          hasil.jawaban_benar_count != null
            ? ` · ${hasil.jawaban_benar_count}/${hasil.jawaban_total} benar`
            : ''}
          {hasil.status === 'selesai' && hasil.nilai_total != null
            ? ` · nilai ${formatNilai(hasil.nilai_total)}`
            : ''}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {hasil.status === 'berlangsung' ? (
          <SiswaButton type="button" tone="primary" size="sm" onClick={onResume}>
            <PlayCircle className="size-3.5" strokeWidth={2.5} />
            Lanjutkan
          </SiswaButton>
        ) : null}
        {hasil.status === 'selesai' ? (
          <SiswaButton
            type="button"
            size="sm"
            tone="surface"
            onClick={onReview}
            disabled={!reviewable}
            title={reviewable ? 'Lihat pembahasan' : reviewLockReason ?? undefined}
          >
            <Eye className="size-3.5" strokeWidth={2.5} />
            Lihat pembahasan
          </SiswaButton>
        ) : null}
      </div>
      {!reviewable && reviewLockReason && hasil.status === 'selesai' ? (
        <p className="basis-full text-xs text-siswa-text-muted">
          {reviewLockReason}
        </p>
      ) : null}
    </li>
  );
}

function StatusBadge({ status }: { status: UjianHasilSummary['status'] }) {
  const map: Record<
    UjianHasilSummary['status'],
    { label: string; tone: React.ComponentProps<typeof SiswaBadge>['tone'] }
  > = {
    berlangsung: { label: 'Berlangsung', tone: 'warning' },
    selesai: { label: 'Selesai', tone: 'success' },
    dibatalkan: { label: 'Dibatalkan', tone: 'danger' },
  };
  const cfg = map[status];
  return <SiswaBadge tone={cfg.tone}>{cfg.label}</SiswaBadge>;
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

// ---------- Pure helpers ----------

function jumlahSoalEstimasi(ujian: Ujian): number | null {
  const cfg = ujian.source_config_json as
    | { mode?: string; soal_ids?: string[]; jumlah_soal?: number }
    | undefined;
  if (!cfg || !cfg.mode) return null;
  if (cfg.mode === 'manual' && Array.isArray(cfg.soal_ids)) {
    return cfg.soal_ids.length;
  }
  if (cfg.mode === 'random' && typeof cfg.jumlah_soal === 'number') {
    return cfg.jumlah_soal;
  }
  return null;
}

function formatRangeJakarta(ujian: Ujian): string {
  const fmt = (iso: string | null | undefined) => {
    if (!iso) return null;
    return new Date(iso).toLocaleString('id-ID', {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: 'Asia/Jakarta',
    });
  };
  const a = fmt(ujian.waktu_mulai);
  const b = fmt(ujian.waktu_selesai);
  if (a && b) return `${a} – ${b}`;
  if (a) return `Mulai ${a}`;
  if (b) return `Berakhir ${b}`;
  return 'Tidak ada batas waktu';
}

function formatCountdown(deltaMs: number): string {
  if (deltaMs <= 0) return '0 detik';
  const totalSec = Math.floor(deltaMs / 1000);
  const days = Math.floor(totalSec / 86400);
  const hours = Math.floor((totalSec % 86400) / 3600);
  const minutes = Math.floor((totalSec % 3600) / 60);
  const seconds = totalSec % 60;
  if (days > 0) return `${days}h ${hours}j ${minutes}m`;
  if (hours > 0) return `${hours}j ${minutes}m`;
  if (minutes > 0)
    return `${minutes}m ${seconds.toString().padStart(2, '0')}s`;
  return `${seconds}s`;
}

function formatNilai(n: number | null | undefined): string {
  if (n == null) return '—';
  return Number.isInteger(n) ? String(n) : n.toFixed(2);
}

function bestNilaiOf(items: UjianHasilSummary[]): number | null {
  let best: number | null = null;
  for (const h of items) {
    if (h.status !== 'selesai' || h.nilai_total == null) continue;
    if (best == null || h.nilai_total > best) best = h.nilai_total;
  }
  return best;
}

function canReviewHasil(
  hasil: UjianHasilSummary,
  ujian: Ujian,
  now: number,
): boolean {
  if (hasil.status !== 'selesai') return false;
  if (!ujian.izinkan_review_setelah_submit) return false;
  if (ujian.waktu_buka_review) {
    return new Date(ujian.waktu_buka_review).getTime() <= now;
  }
  return true;
}

function reviewLockMessage(
  hasil: UjianHasilSummary,
  ujian: Ujian,
  now: number,
): string | null {
  if (hasil.status !== 'selesai') return null;
  if (!ujian.izinkan_review_setelah_submit) {
    return 'Guru tidak mengaktifkan pembahasan untuk ujian ini.';
  }
  if (ujian.waktu_buka_review) {
    const t = new Date(ujian.waktu_buka_review);
    if (t.getTime() > now) {
      return `Pembahasan dibuka ${t.toLocaleString('id-ID', {
        dateStyle: 'medium',
        timeStyle: 'short',
        timeZone: 'Asia/Jakarta',
      })}.`;
    }
  }
  return null;
}

function canReviewResult(summary: UjianSubmitResult, ujian?: Ujian): boolean {
  if (!summary.izinkan_review) return false;
  if (summary.dapat_review_at) {
    return new Date(summary.dapat_review_at).getTime() <= Date.now();
  }
  if (ujian?.waktu_buka_review) {
    return new Date(ujian.waktu_buka_review).getTime() <= Date.now();
  }
  return true;
}

function reviewLockedMsg(
  summary: UjianSubmitResult,
  ujian?: Ujian,
): string | null {
  if (!summary.izinkan_review) {
    return 'Guru tidak mengaktifkan pembahasan untuk ujian ini.';
  }
  const t = summary.dapat_review_at ?? ujian?.waktu_buka_review;
  if (t && new Date(t).getTime() > Date.now()) {
    return `Pembahasan dibuka ${new Date(t).toLocaleString('id-ID', {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: 'Asia/Jakarta',
    })}.`;
  }
  return null;
}

void ShieldCheck;
