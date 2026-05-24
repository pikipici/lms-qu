'use client';

/**
 * SiswaTugasList — list tugas published per kelas atau bab untuk siswa.
 *
 * Visual: neo-brutalism + pastel pop (siswa-only).
 * - Pakai listSiswaTugas (server force status='published').
 * - babID undefined = semua tugas di kelas (campur kelas-wide + bab-scoped).
 * - babID null     = pin bab_id IS NULL (kelas-wide aja).
 * - babID <uuid>   = pin bab tertentu.
 */

import * as React from 'react';
import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import { ArrowRight, ClipboardList, Clock, RotateCcw } from 'lucide-react';

import { ApiError } from '@/lib/api';
import { listSiswaTugas, type Tugas, formatDeadline } from '@/lib/tugas-api';
import { isTugasOverdue } from '@/lib/submission-api';
import {
  SiswaBadge,
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardHeader,
} from '@/components/siswa-ui';

interface SiswaTugasListProps {
  kelasID: string;
  /** undefined: all in kelas. null: kelas-wide only. uuid: bab-specific. */
  babID?: string | null;
  emptyState?: string;
}

export function SiswaTugasList({
  kelasID,
  babID,
  emptyState,
}: SiswaTugasListProps) {
  const query = useQuery({
    queryKey: [
      'siswa',
      'tugas',
      kelasID,
      babID === undefined ? 'all' : babID ?? 'kelas',
    ],
    queryFn: () => listSiswaTugas(kelasID, { babID }),
  });

  if (query.isPending) {
    return (
      <div className="space-y-3">
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            className="h-20 animate-pulse rounded-siswa siswa-border bg-siswa-surface/60"
          />
        ))}
      </div>
    );
  }

  if (query.isError) {
    const err = query.error;
    const apiErr = err instanceof ApiError ? err : null;
    return (
      <div className="space-y-2 rounded-siswa border-2 border-siswa-danger bg-siswa-surface p-4 text-sm">
        <p className="font-bold">
          Gagal memuat daftar tugas
          {apiErr?.requestId ? ` (req ${apiErr.requestId.slice(0, 8)}…)` : ''}.
        </p>
        <SiswaButton
          type="button"
          tone="surface"
          size="sm"
          onClick={() => query.refetch()}
        >
          <RotateCcw className="size-4" />
          Coba lagi
        </SiswaButton>
      </div>
    );
  }

  const items = query.data?.items ?? [];
  if (items.length === 0) {
    return (
      <div className="rounded-siswa border-2 border-dashed border-siswa-border-soft bg-siswa-surface/60 p-8 text-center">
        <ClipboardList
          className="mx-auto mb-2 size-8 text-siswa-text-muted"
          strokeWidth={2.5}
        />
        <p className="text-sm text-siswa-text-muted">
          {emptyState ??
            (babID
              ? 'Belum ada tugas di bab ini.'
              : babID === null
                ? 'Belum ada tugas kelas-wide.'
                : 'Belum ada tugas dipublikasikan.')}
        </p>
      </div>
    );
  }

  return (
    <ul className="space-y-3">
      {items.map((t) => (
        <SiswaTugasRow key={t.id} kelasID={kelasID} tugas={t} />
      ))}
    </ul>
  );
}

function SiswaTugasRow({
  kelasID,
  tugas,
}: {
  kelasID: string;
  tugas: Tugas;
}) {
  const overdue = isTugasOverdue(tugas);
  const overdueBlock = overdue && !tugas.izinkan_late;

  return (
    <li>
      <SiswaCard tone="surface" shadow="sm">
        <SiswaCardHeader className="pb-2">
          <div className="flex flex-row items-start justify-between gap-3">
            <div className="min-w-0 space-y-1.5">
              <div className="flex flex-wrap items-center gap-2">
                <span className="grid size-7 shrink-0 place-items-center rounded-siswa siswa-border bg-siswa-tugas/40">
                  <ClipboardList className="size-3.5" strokeWidth={2.5} />
                </span>
                <h3 className="siswa-display truncate text-sm font-bold">
                  {tugas.judul}
                </h3>
                {overdueBlock ? (
                  <SiswaBadge tone="danger">Tutup</SiswaBadge>
                ) : null}
                {overdue && tugas.izinkan_late ? (
                  <SiswaBadge tone="warning">Lewat deadline</SiswaBadge>
                ) : null}
              </div>
              <p className="flex flex-wrap items-center gap-3 text-xs text-siswa-text-muted">
                <span className="inline-flex items-center gap-1">
                  <Clock className="size-3.5" />
                  {formatDeadline(tugas.deadline)}
                </span>
                {tugas.izinkan_late && tugas.penalty_persen > 0 ? (
                  <span className="font-semibold">
                    Late penalty {tugas.penalty_persen}%
                  </span>
                ) : null}
              </p>
            </div>
            <SiswaButton asChild tone="surface" size="sm">
              <Link
                href={`/siswa/kelas/detail/tugas?id=${kelasID}&tid=${tugas.id}`}
              >
                Buka
                <ArrowRight className="size-3.5" strokeWidth={2.5} />
              </Link>
            </SiswaButton>
          </div>
        </SiswaCardHeader>
        {tugas.deskripsi ? (
          <SiswaCardBody className="pt-0 text-sm text-siswa-text-muted line-clamp-2">
            {tugas.deskripsi}
          </SiswaCardBody>
        ) : null}
      </SiswaCard>
    </li>
  );
}
