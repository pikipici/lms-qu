'use client';

/**
 * UlanganLobby — siswa ulangan pre-flight (Task 5.G.2).
 *
 * Tampilan:
 *   - Setting card: durasi, batas attempt, sisa attempt, izinkan review +
 *     waktu_buka_review (kalau ada).
 *   - History attempt: list nilai per attempt + status badge + tombol
 *     "Lihat Pembahasan" gated #81 (kalau gated, tampil disabled +
 *     countdown ke waktu_buka_review).
 *   - CTA: "Mulai" / "Lanjutkan" / "Tidak bisa mulai" tergantung kondisi.
 *
 * Conditions:
 *   - !configured            → tampil notice "guru belum aktivasi"
 *   - berlangsung exists     → CTA "Lanjutkan"
 *   - attempt_count >= batas → CTA disabled "batas tercapai"
 *   - default                → CTA "Mulai ulangan"
 */

import * as React from 'react';
import {
  AlertCircle,
  Clock3,
  Eye,
  Loader2,
  PlayCircle,
  RefreshCcw,
  Repeat,
  ShieldCheck,
  Timer,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  friendlyUlanganError,
  type HasilSummary,
  type SiswaHasilListResult,
  type SiswaLobbyView,
} from '@/lib/soalbab-ulangan-api';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { cn } from '@/lib/utils';

export interface UlanganLobbyProps {
  setting: SiswaLobbyView;
  hasilList: SiswaHasilListResult;
  onStart: () => void;
  onResume: (hasilID: string) => void;
  onReview: (hasilID: string) => void;
  starting?: boolean;
  startError?: ApiError | null;
  /** Refresh callback dipanggil saat user klik tombol Refresh setting+history. */
  onRefresh?: () => void;
  refreshing?: boolean;
  disabled?: boolean;
}

export function UlanganLobby({
  setting,
  hasilList,
  onStart,
  onResume,
  onReview,
  starting,
  startError,
  onRefresh,
  refreshing,
  disabled,
}: UlanganLobbyProps) {
  const ulanganItems = React.useMemo(
    () => hasilList.items.filter((h) => h.mode === 'ulangan'),
    [hasilList.items],
  );

  const inflight = React.useMemo(
    () => ulanganItems.find((h) => h.status === 'berlangsung'),
    [ulanganItems],
  );

  // attempt_count dari backend = ulangan-only excluding dibatalkan.
  const sisaAttempt = Math.max(0, setting.batas_attempt - hasilList.attempt_count);
  const batasReached = !inflight && sisaAttempt <= 0;

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div>
              <div className="flex items-center gap-2">
                <ShieldCheck className="size-5 text-muted-foreground" />
                <CardTitle className="text-base">Ulangan Bab</CardTitle>
              </div>
              <CardDescription>
                Ulangan dinilai. Tidak ada feedback per soal — nilai keluar setelah submit.
                Pastikan koneksi stabil sebelum mulai.
              </CardDescription>
            </div>
            {onRefresh ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={onRefresh}
                disabled={refreshing}
              >
                {refreshing ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <RefreshCcw className="size-4" />
                )}
                Refresh
              </Button>
            ) : null}
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {!setting.configured ? (
            <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-200">
              <AlertCircle className="mt-0.5 size-4 shrink-0" />
              <span>
                Guru belum mengaktifkan ulangan untuk bab ini. Tombol mulai akan tersedia
                setelah setting dipublish.
              </span>
            </div>
          ) : null}

          <div className="grid gap-3 sm:grid-cols-3">
            <InfoTile icon={Timer} label="Durasi" value={`${setting.durasi_menit} menit`} />
            <InfoTile
              icon={Repeat}
              label="Batas attempt"
              value={`${setting.batas_attempt}×`}
            />
            <InfoTile
              icon={Clock3}
              label="Sisa attempt"
              value={inflight ? '— (masih ada attempt berjalan)' : `${sisaAttempt}×`}
              accent={!inflight && sisaAttempt === 0 ? 'rose' : 'default'}
            />
          </div>

          <ReviewPolicyNote setting={setting} />

          {startError ? (
            <div className="flex items-start gap-2 rounded-md border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-200">
              <AlertCircle className="mt-0.5 size-4 shrink-0" />
              <span>{friendlyUlanganError(startError, 'start')}</span>
            </div>
          ) : null}

          <div className="flex flex-wrap gap-2">
            {inflight ? (
              <Button
                type="button"
                onClick={() => onResume(inflight.hasil_id)}
                disabled={disabled || starting}
              >
                {starting ? <Loader2 className="size-4 animate-spin" /> : <PlayCircle className="size-4" />}
                Lanjutkan ulangan
              </Button>
            ) : (
              <Button
                type="button"
                onClick={onStart}
                disabled={
                  disabled || starting || !setting.configured || batasReached
                }
              >
                {starting ? <Loader2 className="size-4 animate-spin" /> : <PlayCircle className="size-4" />}
                {batasReached ? 'Batas attempt tercapai' : 'Mulai ulangan'}
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Riwayat attempt</CardTitle>
          <CardDescription>
            {ulanganItems.length === 0
              ? 'Belum ada attempt ulangan untuk bab ini.'
              : `Total ${hasilList.attempt_count} attempt selesai${
                  typeof hasilList.nilai_terbaik === 'number'
                    ? ` · nilai terbaik ${hasilList.nilai_terbaik}`
                    : ''
                }.`}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {ulanganItems.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Begitu lu mulai ulangan, attempt akan tampil di sini.
            </p>
          ) : (
            <ul className="space-y-2">
              {ulanganItems.map((h) => (
                <HistoryRow
                  key={h.hasil_id}
                  hasil={h}
                  setting={setting}
                  onReview={() => onReview(h.hasil_id)}
                />
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

interface InfoTileProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
  accent?: 'default' | 'rose';
}

function InfoTile({ icon: Icon, label, value, accent = 'default' }: InfoTileProps) {
  return (
    <div
      className={cn(
        'rounded-md border p-3',
        accent === 'rose' && 'border-rose-200 bg-rose-50 text-rose-800 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-200',
      )}
    >
      <div className="flex items-center gap-1.5 text-xs uppercase tracking-wide text-muted-foreground">
        <Icon className="size-3.5" />
        {label}
      </div>
      <div className="mt-1 text-sm font-medium">{value}</div>
    </div>
  );
}

function ReviewPolicyNote({ setting }: { setting: SiswaLobbyView }) {
  if (!setting.izinkan_review_setelah_submit) {
    return (
      <p className="rounded-md bg-muted/40 p-2 text-xs text-muted-foreground">
        Guru tidak mengaktifkan review setelah submit untuk ulangan ini.
      </p>
    );
  }
  if (setting.waktu_buka_review) {
    const t = new Date(setting.waktu_buka_review);
    return (
      <p className="rounded-md bg-muted/40 p-2 text-xs text-muted-foreground">
        Pembahasan jawaban dibuka mulai{' '}
        <strong>
          {t.toLocaleString('id-ID', {
            dateStyle: 'medium',
            timeStyle: 'short',
          })}
        </strong>
        .
      </p>
    );
  }
  return (
    <p className="rounded-md bg-muted/40 p-2 text-xs text-muted-foreground">
      Pembahasan jawaban tersedia langsung setelah lu submit attempt.
    </p>
  );
}

function HistoryRow({
  hasil,
  setting,
  onReview,
}: {
  hasil: HasilSummary;
  setting: SiswaLobbyView;
  onReview: () => void;
}) {
  const mulaiAt = hasil.mulai_at ? new Date(hasil.mulai_at) : null;
  const reviewable = canReview(hasil, setting);
  const reviewLockReason = reviewLockMessage(hasil, setting);

  return (
    <li className="flex flex-wrap items-start justify-between gap-3 rounded-md border bg-card p-3">
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-medium">Attempt #{hasil.attempt_no}</span>
          <StatusBadge status={hasil.status} />
        </div>
        <div className="text-xs text-muted-foreground">
          {mulaiAt
            ? `Mulai ${mulaiAt.toLocaleString('id-ID', {
                dateStyle: 'medium',
                timeStyle: 'short',
              })}`
            : '—'}
          {hasil.status === 'selesai' && hasil.jawaban_total != null && hasil.jawaban_benar_count != null
            ? ` · ${hasil.jawaban_benar_count}/${hasil.jawaban_total} benar`
            : ''}
          {hasil.status === 'selesai' && hasil.nilai_total != null
            ? ` · nilai ${hasil.nilai_total}`
            : ''}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {hasil.status === 'selesai' ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={onReview}
            disabled={!reviewable}
            title={reviewable ? 'Lihat pembahasan' : reviewLockReason ?? undefined}
          >
            <Eye className="size-3.5" />
            Lihat pembahasan
          </Button>
        ) : null}
      </div>
      {!reviewable && reviewLockReason && hasil.status === 'selesai' ? (
        <p className="basis-full text-xs text-muted-foreground">{reviewLockReason}</p>
      ) : null}
    </li>
  );
}

function StatusBadge({ status }: { status: HasilSummary['status'] }) {
  const map = {
    berlangsung: { label: 'Berlangsung', cn: 'bg-amber-100 text-amber-800' },
    selesai: { label: 'Selesai', cn: 'bg-emerald-100 text-emerald-800' },
    expired: { label: 'Expired', cn: 'bg-rose-100 text-rose-800' },
    dibatalkan: { label: 'Dibatalkan', cn: 'bg-muted text-muted-foreground' },
  } as const;
  const cfg = map[status] ?? map.berlangsung;
  return (
    <span className={cn('rounded-full px-2 py-0.5 text-[11px] font-medium', cfg.cn)}>
      {cfg.label}
    </span>
  );
}

function canReview(hasil: HasilSummary, setting: SiswaLobbyView): boolean {
  if (hasil.status !== 'selesai') return false;
  if (!setting.izinkan_review_setelah_submit) return false;
  if (setting.waktu_buka_review) {
    return new Date(setting.waktu_buka_review).getTime() <= Date.now();
  }
  return true;
}

function reviewLockMessage(hasil: HasilSummary, setting: SiswaLobbyView): string | null {
  if (hasil.status !== 'selesai') return null;
  if (!setting.izinkan_review_setelah_submit) {
    return 'Guru tidak mengaktifkan review untuk ulangan ini.';
  }
  if (setting.waktu_buka_review) {
    const t = new Date(setting.waktu_buka_review);
    if (t.getTime() > Date.now()) {
      return `Pembahasan dibuka ${t.toLocaleString('id-ID', {
        dateStyle: 'medium',
        timeStyle: 'short',
      })}.`;
    }
  }
  return null;
}
