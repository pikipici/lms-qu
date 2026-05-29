'use client';

/**
 * UjianLobbyCard — siswa lobby card per Ujian (Task 6.G.1).
 *
 * Visual: neo-brutalism + pastel pop (siswa-only). Tone "ulangan" (yellow)
 * untuk header strip, surface body. CTA pakai SiswaButton.
 *
 * Conditions:
 *   - inflight attempt (status='berlangsung')        → CTA "Lanjutkan"
 *   - now < waktu_mulai                              → CTA disabled "Mulai dalam"
 *   - now > waktu_selesai                            → CTA disabled "Berakhir"
 *   - default + attempt selesai                      → CTA "Mulai ujian baru"
 *   - default                                        → CTA "Mulai ujian"
 */

import * as React from 'react';
import Link from 'next/link';
import {
  AlertCircle,
  ArrowRight,
  CalendarRange,
  Clock,
  Eye,
  ListChecks,
  PlayCircle,
  Repeat,
  ShieldCheck,
  Timer,
  XCircle,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

import {
  type Ujian,
  type SiswaUjianHasilListResult,
  type UjianHasilSummary,
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

// ---------- Public props ----------

export interface UjianLobbyCardProps {
  ujian: Ujian;
  /** Nama kelas — dipakai di header context (cross-kelas list). */
  kelasName?: string;
  /** Aggregate hasil siswa di kelas — UjianLobbyCard filter per ujian_id. */
  hasilAggregate?: SiswaUjianHasilListResult;
}

// ---------- Internal helpers ----------

type WindowState = 'mendatang' | 'aktif' | 'berakhir' | 'tanpa-window';

function computeWindowState(
  now: number,
  ujian: Ujian,
): { state: WindowState; targetMillis?: number } {
  const startMs = ujian.waktu_mulai
    ? new Date(ujian.waktu_mulai).getTime()
    : null;
  const endMs = ujian.waktu_selesai
    ? new Date(ujian.waktu_selesai).getTime()
    : null;
  if (startMs && now < startMs) {
    return { state: 'mendatang', targetMillis: startMs };
  }
  if (endMs && now > endMs) {
    return { state: 'berakhir', targetMillis: endMs };
  }
  if (startMs || endMs) {
    return { state: 'aktif', targetMillis: endMs ?? undefined };
  }
  return { state: 'tanpa-window' };
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
  if (days > 0) {
    return `${days}h ${hours}j ${minutes}m`;
  }
  if (hours > 0) {
    return `${hours}j ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds.toString().padStart(2, '0')}s`;
  }
  return `${seconds}s`;
}

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

function sourceModeLabel(ujian: Ujian): string {
  const cfg = ujian.source_config_json as { mode?: string } | undefined;
  if (cfg?.mode === 'random') return 'Random';
  if (cfg?.mode === 'manual') return 'Manual';
  return 'Belum diatur';
}

function pickInflight(items: UjianHasilSummary[]): UjianHasilSummary | null {
  return items.find((h) => h.status === 'berlangsung') ?? null;
}

function pickLatestSelesai(
  items: UjianHasilSummary[],
): UjianHasilSummary | null {
  let best: UjianHasilSummary | null = null;
  for (const h of items) {
    if (h.status !== 'selesai') continue;
    if (!best || h.mulai_at.localeCompare(best.mulai_at) > 0) {
      best = h;
    }
  }
  return best;
}

function pickBestNilai(items: UjianHasilSummary[]): number | null {
  let best: number | null = null;
  for (const h of items) {
    if (h.status !== 'selesai' || h.nilai_total == null) continue;
    if (best == null || h.nilai_total > best) {
      best = h.nilai_total;
    }
  }
  return best;
}

// ---------- Component ----------

export function UjianLobbyCard({
  ujian,
  kelasName,
  hasilAggregate,
}: UjianLobbyCardProps) {
  const [now, setNow] = React.useState<number>(() => Date.now());

  React.useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);

  const windowInfo = React.useMemo(
    () => computeWindowState(now, ujian),
    [now, ujian],
  );

  const myItems = React.useMemo(() => {
    if (!hasilAggregate) return [] as UjianHasilSummary[];
    return hasilAggregate.items.filter((h) => h.ujian_id === ujian.id);
  }, [hasilAggregate, ujian.id]);

  const inflight = React.useMemo(() => pickInflight(myItems), [myItems]);
  const latest = React.useMemo(() => pickLatestSelesai(myItems), [myItems]);
  const bestNilai = React.useMemo(() => pickBestNilai(myItems), [myItems]);

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

  const jumlahSoal = jumlahSoalEstimasi(ujian);
  const sourceLabel = sourceModeLabel(ujian);
  const range = formatRangeJakarta(ujian);

  const ctaHref = `/siswa/kelas/detail/ujian?id=${ujian.kelas_id}&uid=${ujian.id}`;

  return (
    <SiswaCard tone="ulangan" shadow="md" className="overflow-hidden">
      <SiswaCardHeader className="border-b-2 border-siswa-border bg-siswa-surface/70 pb-5">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0 space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <SiswaCardTitle className="truncate text-base">
                {ujian.judul}
              </SiswaCardTitle>
              <WindowBadge state={windowInfo.state} />
              <SiswaBadge tone="blue">{sourceLabel}</SiswaBadge>
            </div>
            <SiswaCardDescription className="space-y-1.5">
              {kelasName ? (
                <div className="font-semibold text-siswa-text">
                  {kelasName}
                </div>
              ) : null}
              {ujian.deskripsi ? (
                <div className="line-clamp-2 text-sm">{ujian.deskripsi}</div>
              ) : null}
            </SiswaCardDescription>
          </div>
        </div>
      </SiswaCardHeader>
      <SiswaCardBody className="space-y-4 pt-5">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
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
            icon={ShieldCheck}
            label="Nilai terbaik"
            value={bestNilai != null ? formatNilai(bestNilai) : '—'}
            accent={bestNilai != null ? 'emerald' : 'default'}
          />
        </div>

        <div className="rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface p-3">
          <div className="flex items-start gap-2 text-xs text-siswa-text-muted">
            <CalendarRange className="mt-0.5 size-3.5 shrink-0" />
            <div className="space-y-1">
              <div className="font-semibold text-siswa-text">{range}</div>
              <CountdownLine windowInfo={windowInfo} now={now} />
            </div>
          </div>
        </div>

        <ReviewPolicyNote ujian={ujian} />

        {latest ? (
          <LastAttemptLine latest={latest} ujian={ujian} now={now} />
        ) : null}

        <div className="flex flex-wrap items-center gap-2">
          <PrimaryCTA
            inflight={inflight}
            ctaHref={ctaHref}
            windowState={windowInfo.state}
            now={now}
            targetMillis={windowInfo.targetMillis}
            attemptCount={attemptCount}
            attemptsExhausted={attemptsExhausted}
          />
          {latest &&
          latest.status === 'selesai' &&
          canViewReview(ujian, latest, now) ? (
            <SiswaButton asChild tone="surface" size="sm">
              <Link href={ctaHref}>
                <Eye className="size-3.5" />
                Lihat pembahasan terakhir
              </Link>
            </SiswaButton>
          ) : null}
        </div>
      </SiswaCardBody>
    </SiswaCard>
  );
}

// ---------- Sub-components ----------

interface InfoTileProps {
  icon: LucideIcon;
  label: string;
  value: string;
  accent?: 'default' | 'emerald' | 'rose';
}

function InfoTile({
  icon: Icon,
  label,
  value,
  accent = 'default',
}: InfoTileProps) {
  return (
    <div
      className={cn(
        'rounded-siswa border-2 border-siswa-border bg-siswa-surface p-3',
        accent === 'emerald' && 'bg-siswa-green/40',
        accent === 'rose' && 'bg-rose-100',
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

function WindowBadge({ state }: { state: WindowState }) {
  const map: Record<
    WindowState,
    {
      label: string;
      tone: React.ComponentProps<typeof SiswaBadge>['tone'];
      icon: LucideIcon;
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
      <Icon className="size-3" strokeWidth={2.5} />
      {cfg.label}
    </SiswaBadge>
  );
}

function CountdownLine({
  windowInfo,
  now,
}: {
  windowInfo: { state: WindowState; targetMillis?: number };
  now: number;
}) {
  if (!windowInfo.targetMillis) {
    if (windowInfo.state === 'tanpa-window') {
      return <div>Tidak ada batas waktu — mulai kapan saja.</div>;
    }
    return null;
  }
  const delta = Math.max(0, windowInfo.targetMillis - now);
  if (windowInfo.state === 'mendatang') {
    return <div>Dimulai dalam {formatCountdown(delta)}.</div>;
  }
  if (windowInfo.state === 'aktif') {
    if (delta <= 0) return <div>Window ditutup.</div>;
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
        Pembahasan dibuka mulai{' '}
        <strong className="text-siswa-text">{t}</strong>.
      </p>
    );
  }
  return (
    <p className="rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface/70 p-2 text-xs text-siswa-text-muted">
      Pembahasan tersedia langsung setelah kamu submit jawaban.
    </p>
  );
}

function LastAttemptLine({
  latest,
  ujian,
  now,
}: {
  latest: UjianHasilSummary;
  ujian: Ujian;
  now: number;
}) {
  const reviewable = canViewReview(ujian, latest, now);
  const reviewMsg = reviewable ? null : reviewLockMessage(ujian, latest, now);
  return (
    <div className="rounded-siswa border-2 border-siswa-border bg-siswa-surface p-3 text-xs siswa-shadow-sm">
      <div className="flex flex-wrap items-center gap-2">
        <span className="siswa-display font-bold">
          Kesempatan terakhir #{latest.attempt_no}
        </span>
        <StatusBadge status={latest.status} />
        {latest.nilai_total != null ? (
          <span className="text-siswa-text-muted">
            Nilai{' '}
            <strong className="text-siswa-text">
              {formatNilai(latest.nilai_total)}
            </strong>
          </span>
        ) : null}
        {latest.jawaban_benar_count != null && latest.jawaban_total != null ? (
          <span className="text-siswa-text-muted">
            ({latest.jawaban_benar_count}/{latest.jawaban_total} benar)
          </span>
        ) : null}
      </div>
      {reviewMsg ? (
        <p className="mt-1 text-siswa-text-muted">{reviewMsg}</p>
      ) : null}
    </div>
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

interface PrimaryCTAProps {
  inflight: UjianHasilSummary | null;
  ctaHref: string;
  windowState: WindowState;
  now: number;
  targetMillis?: number;
  attemptCount: number;
  attemptsExhausted: boolean;
}

function PrimaryCTA({
  inflight,
  ctaHref,
  windowState,
  now,
  targetMillis,
  attemptCount,
  attemptsExhausted,
}: PrimaryCTAProps) {
  if (inflight) {
    return (
      <SiswaButton asChild tone="primary">
        <Link href={ctaHref}>
          <PlayCircle className="size-4" strokeWidth={2.5} />
          Lanjutkan ujian
          <ArrowRight className="size-4" strokeWidth={2.5} />
        </Link>
      </SiswaButton>
    );
  }
  if (windowState === 'mendatang' && targetMillis) {
    return (
      <SiswaButton tone="surface" disabled>
        <Clock className="size-4" />
        Mulai dalam {formatCountdown(Math.max(0, targetMillis - now))}
      </SiswaButton>
    );
  }
  if (windowState === 'berakhir') {
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
    <SiswaButton asChild tone="primary">
      <Link href={ctaHref}>
        <PlayCircle className="size-4" strokeWidth={2.5} />
        {attemptCount > 0 ? 'Mulai ujian baru' : 'Mulai ujian'}
        <ArrowRight className="size-4" strokeWidth={2.5} />
      </Link>
    </SiswaButton>
  );
}

// ---------- Pure helpers ----------

function formatNilai(n: number | null | undefined): string {
  if (n == null) return '—';
  return Number.isInteger(n) ? String(n) : n.toFixed(2);
}

function canViewReview(
  ujian: Ujian,
  hasil: UjianHasilSummary,
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
  ujian: Ujian,
  hasil: UjianHasilSummary,
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

void AlertCircle;
