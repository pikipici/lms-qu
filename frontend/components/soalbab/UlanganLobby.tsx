'use client';

/**
 * UlanganLobby — siswa ulangan pre-flight (Task 5.G.2).
 *
 * Visual: neo-brutalism + pastel pop. Setting card tone "ulangan", history
 * card tone surface, pre-flight tiles dengan pastel accent.
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
import type { LucideIcon } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  friendlyUlanganError,
  type HasilSummary,
  type SiswaHasilListResult,
  type SiswaLobbyView,
} from '@/lib/soalbab-ulangan-api';
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

export interface UlanganLobbyProps {
  setting: SiswaLobbyView;
  hasilList: SiswaHasilListResult;
  onStart: () => void;
  onResume: (hasilID: string) => void;
  onReview: (hasilID: string) => void;
  starting?: boolean;
  startError?: ApiError | null;
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

  const sisaAttempt = Math.max(
    0,
    setting.batas_attempt - hasilList.attempt_count,
  );
  const batasReached = !inflight && sisaAttempt <= 0;

  return (
    <div className="space-y-4">
      <SiswaCard tone="ulangan" shadow="md">
        <SiswaCardHeader>
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div>
              <div className="flex items-center gap-2">
                <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                  <ShieldCheck className="size-4" strokeWidth={2.5} />
                </span>
                <SiswaCardTitle>Ulangan Bab</SiswaCardTitle>
              </div>
              <SiswaCardDescription>
                Ulangan dinilai. Tidak ada feedback per soal — nilai keluar
                setelah submit. Pastikan koneksi stabil sebelum mulai.
              </SiswaCardDescription>
            </div>
            {onRefresh ? (
              <SiswaButton
                type="button"
                tone="surface"
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
              </SiswaButton>
            ) : null}
          </div>
        </SiswaCardHeader>
        <SiswaCardBody className="space-y-4">
          {!setting.configured ? (
            <div className="flex items-start gap-2 rounded-siswa border-2 border-siswa-border-soft bg-siswa-cream p-3 text-sm">
              <AlertCircle
                className="mt-0.5 size-4 shrink-0 text-siswa-warning"
                strokeWidth={2.5}
              />
              <span className="text-siswa-text">
                Guru belum mengaktifkan ulangan untuk bab ini. Tombol mulai
                akan tersedia setelah setting dipublish.
              </span>
            </div>
          ) : null}

          <div className="grid gap-3 sm:grid-cols-3">
            <InfoTile
              icon={Timer}
              label="Durasi"
              value={`${setting.durasi_menit} menit`}
            />
            <InfoTile
              icon={Repeat}
              label="Batas attempt"
              value={`${setting.batas_attempt}×`}
            />
            <InfoTile
              icon={Clock3}
              label="Sisa attempt"
              value={
                inflight
                  ? '— (masih ada attempt berjalan)'
                  : `${sisaAttempt}×`
              }
              accent={!inflight && sisaAttempt === 0 ? 'rose' : 'default'}
            />
          </div>

          <ReviewPolicyNote setting={setting} />

          {startError ? (
            <div className="flex items-start gap-2 rounded-siswa border-2 border-siswa-danger bg-rose-50 p-3 text-sm font-semibold">
              <AlertCircle className="mt-0.5 size-4 shrink-0" strokeWidth={2.5} />
              <span>{friendlyUlanganError(startError, 'start')}</span>
            </div>
          ) : null}

          <div className="flex flex-wrap gap-2">
            {inflight ? (
              <SiswaButton
                type="button"
                tone="primary"
                onClick={() => onResume(inflight.hasil_id)}
                disabled={disabled || starting}
              >
                {starting ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <PlayCircle className="size-4" strokeWidth={2.5} />
                )}
                Lanjutkan ulangan
              </SiswaButton>
            ) : (
              <SiswaButton
                type="button"
                tone="primary"
                onClick={onStart}
                disabled={
                  disabled || starting || !setting.configured || batasReached
                }
              >
                {starting ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <PlayCircle className="size-4" strokeWidth={2.5} />
                )}
                {batasReached ? 'Batas attempt tercapai' : 'Mulai ulangan'}
              </SiswaButton>
            )}
          </div>
        </SiswaCardBody>
      </SiswaCard>

      <SiswaCard tone="surface" shadow="sm">
        <SiswaCardHeader>
          <SiswaCardTitle>Riwayat attempt</SiswaCardTitle>
          <SiswaCardDescription>
            {ulanganItems.length === 0
              ? 'Belum ada attempt ulangan untuk bab ini.'
              : `Total ${hasilList.attempt_count} attempt selesai${
                  typeof hasilList.nilai_terbaik === 'number'
                    ? ` · nilai terbaik ${hasilList.nilai_terbaik}`
                    : ''
                }.`}
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          {ulanganItems.length === 0 ? (
            <p className="text-sm text-siswa-text-muted">
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
        </SiswaCardBody>
      </SiswaCard>
    </div>
  );
}

interface InfoTileProps {
  icon: LucideIcon;
  label: string;
  value: string;
  accent?: 'default' | 'rose';
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

function ReviewPolicyNote({ setting }: { setting: SiswaLobbyView }) {
  if (!setting.izinkan_review_setelah_submit) {
    return (
      <p className="rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface/70 p-2 text-xs text-siswa-text-muted">
        Guru tidak mengaktifkan review setelah submit untuk ulangan ini.
      </p>
    );
  }
  if (setting.waktu_buka_review) {
    const t = new Date(setting.waktu_buka_review);
    return (
      <p className="rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface/70 p-2 text-xs text-siswa-text-muted">
        Pembahasan jawaban dibuka mulai{' '}
        <strong className="text-siswa-text">
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
    <p className="rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface/70 p-2 text-xs text-siswa-text-muted">
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
              })}`
            : '—'}
          {hasil.status === 'selesai' &&
          hasil.jawaban_total != null &&
          hasil.jawaban_benar_count != null
            ? ` · ${hasil.jawaban_benar_count}/${hasil.jawaban_total} benar`
            : ''}
          {hasil.status === 'selesai' && hasil.nilai_total != null
            ? ` · nilai ${hasil.nilai_total}`
            : ''}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
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

function StatusBadge({ status }: { status: HasilSummary['status'] }) {
  const map: Record<
    HasilSummary['status'],
    { label: string; tone: React.ComponentProps<typeof SiswaBadge>['tone'] }
  > = {
    berlangsung: { label: 'Berlangsung', tone: 'warning' },
    selesai: { label: 'Selesai', tone: 'success' },
    dibatalkan: { label: 'Dibatalkan', tone: 'neutral' },
  };
  const cfg = map[status] ?? map.berlangsung;
  return <SiswaBadge tone={cfg.tone}>{cfg.label}</SiswaBadge>;
}

function canReview(hasil: HasilSummary, setting: SiswaLobbyView): boolean {
  if (hasil.status !== 'selesai') return false;
  if (!setting.izinkan_review_setelah_submit) return false;
  if (setting.waktu_buka_review) {
    return new Date(setting.waktu_buka_review).getTime() <= Date.now();
  }
  return true;
}

function reviewLockMessage(
  hasil: HasilSummary,
  setting: SiswaLobbyView,
): string | null {
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
