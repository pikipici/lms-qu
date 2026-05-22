'use client';

/**
 * UjianLobbyCard — siswa lobby card per Ujian (Task 6.G.1).
 *
 * Mirror SoalBab UlanganLobby pre-flight pattern, tapi as a single
 * standalone card untuk dipakai di list page `/siswa/ujian` (cross-kelas).
 *
 * Tampilan:
 *   - Header: judul + status window badge (mendatang/aktif/lewat).
 *   - Tile grid: durasi, jumlah soal, source mode hint, attempt aggregate
 *     (count, nilai_terbaik, status_terakhir).
 *   - Window range Asia/Jakarta + countdown ke waktu_mulai/waktu_selesai
 *     (auto re-render setiap detik).
 *   - Review policy note (configured guru).
 *   - CTA: "Mulai" / "Lanjutkan" / "Lihat hasil" / "Mulai dalam ..."
 *     bergantung kondisi window + attempt state.
 *
 * Conditions:
 *   - inflight attempt (status='berlangsung')        → CTA "Lanjutkan"
 *   - now < waktu_mulai                              → CTA disabled "Mulai dalam"
 *   - now > waktu_selesai                            → CTA disabled "Berakhir"
 *   - default + attempt selesai                      → CTA "Mulai ujian baru"
 *   - default                                        → CTA "Mulai ujian"
 *
 * CTA navigates ke /siswa/kelas/detail/ujian?id=:kelasID&uid=:ujianID
 * (target 6.G.2 player+review). Sampai 6.G.2 deploy, route ini 404 —
 * deliberate two-stage rollout (lobby ship dulu).
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

import {
  type Ujian,
  type SiswaUjianHasilListResult,
  type UjianHasilSummary,
} from '@/lib/siswa-ujian-api';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { cn } from '@/lib/utils';

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
  const startMs = ujian.waktu_mulai ? new Date(ujian.waktu_mulai).getTime() : null;
  const endMs = ujian.waktu_selesai ? new Date(ujian.waktu_selesai).getTime() : null;
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

function pickLatestSelesai(items: UjianHasilSummary[]): UjianHasilSummary | null {
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

  const windowInfo = React.useMemo(() => computeWindowState(now, ujian), [now, ujian]);

  const myItems = React.useMemo(() => {
    if (!hasilAggregate) return [] as UjianHasilSummary[];
    return hasilAggregate.items.filter((h) => h.ujian_id === ujian.id);
  }, [hasilAggregate, ujian.id]);

  const inflight = React.useMemo(() => pickInflight(myItems), [myItems]);
  const latest = React.useMemo(() => pickLatestSelesai(myItems), [myItems]);
  const bestNilai = React.useMemo(() => pickBestNilai(myItems), [myItems]);

  const attemptCount = myItems.filter((h) => h.status === 'selesai').length;
  const cancelledCount = myItems.filter((h) => h.status === 'dibatalkan').length;

  const jumlahSoal = jumlahSoalEstimasi(ujian);
  const sourceLabel = sourceModeLabel(ujian);
  const range = formatRangeJakarta(ujian);

  const ctaHref = `/siswa/kelas/detail/ujian?id=${ujian.kelas_id}&uid=${ujian.id}`;

  return (
    <Card className="overflow-hidden">
      <CardHeader className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <CardTitle className="truncate text-base">{ujian.judul}</CardTitle>
            <WindowBadge state={windowInfo.state} />
            <SourceBadge label={sourceLabel} />
          </div>
          <CardDescription className="space-y-0.5">
            {kelasName ? <div className="text-foreground">{kelasName}</div> : null}
            {ujian.deskripsi ? (
              <div className="line-clamp-2 text-sm">{ujian.deskripsi}</div>
            ) : null}
          </CardDescription>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
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
            label="Attempt selesai"
            value={`${attemptCount}×${cancelledCount > 0 ? ` · ${cancelledCount} dibatalkan` : ''}`}
          />
          <InfoTile
            icon={ShieldCheck}
            label="Nilai terbaik"
            value={bestNilai != null ? formatNilai(bestNilai) : '—'}
            accent={bestNilai != null ? 'emerald' : 'default'}
          />
        </div>

        <div className="rounded-md border bg-muted/40 p-3">
          <div className="flex items-start gap-2 text-xs text-muted-foreground">
            <CalendarRange className="mt-0.5 size-3.5 shrink-0" />
            <div className="space-y-1">
              <div className="font-medium text-foreground">{range}</div>
              <CountdownLine windowInfo={windowInfo} now={now} />
            </div>
          </div>
        </div>

        <ReviewPolicyNote ujian={ujian} />

        {latest && (
          <LastAttemptLine latest={latest} ujian={ujian} now={now} />
        )}

        <div className="flex flex-wrap items-center gap-2">
          <PrimaryCTA
            inflight={inflight}
            ctaHref={ctaHref}
            windowState={windowInfo.state}
            now={now}
            targetMillis={windowInfo.targetMillis}
            attemptCount={attemptCount}
          />
          {latest && latest.status === 'selesai' && canViewReview(ujian, latest, now) ? (
            <Button asChild variant="outline" size="sm">
              <Link href={ctaHref}>
                <Eye className="size-3.5" />
                Lihat pembahasan terakhir
              </Link>
            </Button>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}

// ---------- Sub-components ----------

interface InfoTileProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
  accent?: 'default' | 'emerald' | 'rose';
}

function InfoTile({ icon: Icon, label, value, accent = 'default' }: InfoTileProps) {
  return (
    <div
      className={cn(
        'rounded-md border bg-card p-3',
        accent === 'emerald' &&
          'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-200',
        accent === 'rose' &&
          'border-rose-200 bg-rose-50 text-rose-800 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-200',
      )}
    >
      <div className="flex items-center gap-1.5 text-[11px] uppercase tracking-wide text-muted-foreground">
        <Icon className="size-3.5" />
        {label}
      </div>
      <div className="mt-1 text-sm font-medium">{value}</div>
    </div>
  );
}

function WindowBadge({ state }: { state: WindowState }) {
  const map: Record<
    WindowState,
    { label: string; cn: string; icon: React.ComponentType<{ className?: string }> }
  > = {
    mendatang: {
      label: 'Mendatang',
      cn: 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200',
      icon: Clock,
    },
    aktif: {
      label: 'Aktif',
      cn: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200',
      icon: PlayCircle,
    },
    berakhir: {
      label: 'Berakhir',
      cn: 'bg-zinc-200 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-300',
      icon: XCircle,
    },
    'tanpa-window': {
      label: 'Tersedia',
      cn: 'bg-sky-100 text-sky-800 dark:bg-sky-900 dark:text-sky-200',
      icon: PlayCircle,
    },
  };
  const cfg = map[state];
  const Icon = cfg.icon;
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium',
        cfg.cn,
      )}
    >
      <Icon className="size-3" />
      {cfg.label}
    </span>
  );
}

function SourceBadge({ label }: { label: string }) {
  return (
    <span className="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-[11px] font-medium text-blue-800 dark:bg-blue-900 dark:text-blue-200">
      {label}
    </span>
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
      <p className="rounded-md bg-muted/40 p-2 text-xs text-muted-foreground">
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
      <p className="rounded-md bg-muted/40 p-2 text-xs text-muted-foreground">
        Pembahasan dibuka mulai <strong>{t}</strong>.
      </p>
    );
  }
  return (
    <p className="rounded-md bg-muted/40 p-2 text-xs text-muted-foreground">
      Pembahasan tersedia langsung setelah lu submit attempt.
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
    <div className="rounded-md border bg-card p-3 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">Attempt terakhir #{latest.attempt_no}</span>
        <StatusBadge status={latest.status} />
        {latest.nilai_total != null ? (
          <span className="text-muted-foreground">
            Nilai <strong className="text-foreground">{formatNilai(latest.nilai_total)}</strong>
          </span>
        ) : null}
        {latest.jawaban_benar_count != null && latest.jawaban_total != null ? (
          <span className="text-muted-foreground">
            ({latest.jawaban_benar_count}/{latest.jawaban_total} benar)
          </span>
        ) : null}
      </div>
      {reviewMsg ? (
        <p className="mt-1 text-muted-foreground">{reviewMsg}</p>
      ) : null}
    </div>
  );
}

function StatusBadge({ status }: { status: UjianHasilSummary['status'] }) {
  const map: Record<UjianHasilSummary['status'], { label: string; cn: string }> = {
    berlangsung: {
      label: 'Berlangsung',
      cn: 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200',
    },
    selesai: {
      label: 'Selesai',
      cn: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200',
    },
    dibatalkan: {
      label: 'Dibatalkan',
      cn: 'bg-rose-100 text-rose-800 dark:bg-rose-900 dark:text-rose-200',
    },
  };
  const cfg = map[status];
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium',
        cfg.cn,
      )}
    >
      {cfg.label}
    </span>
  );
}

interface PrimaryCTAProps {
  inflight: UjianHasilSummary | null;
  ctaHref: string;
  windowState: WindowState;
  now: number;
  targetMillis?: number;
  attemptCount: number;
}

function PrimaryCTA({
  inflight,
  ctaHref,
  windowState,
  now,
  targetMillis,
  attemptCount,
}: PrimaryCTAProps) {
  // Inflight selalu prioritas — siswa wajib lanjut attempt sebelum mulai baru.
  if (inflight) {
    return (
      <Button asChild>
        <Link href={ctaHref}>
          <PlayCircle className="size-4" />
          Lanjutkan ujian
          <ArrowRight className="size-4" />
        </Link>
      </Button>
    );
  }
  if (windowState === 'mendatang' && targetMillis) {
    return (
      <Button disabled>
        <Clock className="size-4" />
        Mulai dalam {formatCountdown(Math.max(0, targetMillis - now))}
      </Button>
    );
  }
  if (windowState === 'berakhir') {
    return (
      <Button disabled variant="outline">
        <XCircle className="size-4" />
        Window berakhir
      </Button>
    );
  }
  // Aktif atau tanpa window.
  return (
    <Button asChild>
      <Link href={ctaHref}>
        <PlayCircle className="size-4" />
        {attemptCount > 0 ? 'Mulai ujian baru' : 'Mulai ujian'}
        <ArrowRight className="size-4" />
      </Link>
    </Button>
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

// AlertCircle imported for future use; suppress unused warnings.
void AlertCircle;
