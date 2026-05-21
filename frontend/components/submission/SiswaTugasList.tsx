'use client';

/**
 * SiswaTugasList — list tugas published per kelas atau bab untuk siswa.
 *
 * - Pakai listSiswaTugas (server force status='published').
 * - babID undefined = semua tugas di kelas (campur kelas-wide + bab-scoped).
 * - babID null     = pin bab_id IS NULL (kelas-wide aja).
 * - babID <uuid>   = pin bab tertentu.
 *
 * Card: judul + meta (deadline + bab badge kalau perlu) + tombol "Buka"
 * → /siswa/kelas/detail/tugas?id=:kelasID&tid=:tugasID.
 *
 * Empty state: explicit jelasin scope (kelas vs bab).
 */

import * as React from 'react';
import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import {
  ArrowRight,
  ClipboardList,
  Clock,
  RotateCcw,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import { listSiswaTugas, type Tugas } from '@/lib/tugas-api';
import {
  formatDeadline,
  isTugasOverdue,
} from '@/lib/submission-api';

import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardHeader,
} from '@/components/ui/card';

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
    queryKey: ['siswa', 'tugas', kelasID, babID === undefined ? 'all' : babID ?? 'kelas'],
    queryFn: () => listSiswaTugas(kelasID, { babID }),
  });

  if (query.isPending) {
    return (
      <div className="space-y-2">
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            className="h-20 animate-pulse rounded-md border bg-muted/40"
          />
        ))}
      </div>
    );
  }

  if (query.isError) {
    const err = query.error;
    const apiErr = err instanceof ApiError ? err : null;
    return (
      <Card>
        <CardContent className="space-y-2 py-4 text-sm">
          <p className="text-destructive">
            Gagal memuat daftar tugas
            {apiErr?.requestId ? ` (req ${apiErr.requestId.slice(0, 8)}…)` : ''}.
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => query.refetch()}
          >
            <RotateCcw className="size-4" />
            Coba lagi
          </Button>
        </CardContent>
      </Card>
    );
  }

  const items = query.data?.items ?? [];
  if (items.length === 0) {
    return (
      <Card>
        <CardContent className="py-6 text-center text-sm text-muted-foreground">
          {emptyState ??
            (babID
              ? 'Belum ada tugas di bab ini.'
              : babID === null
                ? 'Belum ada tugas kelas-wide.'
                : 'Belum ada tugas dipublikasikan.')}
        </CardContent>
      </Card>
    );
  }

  return (
    <ul className="space-y-2">
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
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3 pb-2">
          <div className="space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <ClipboardList className="size-4 text-muted-foreground" />
              <h3 className="text-sm font-semibold">{tugas.judul}</h3>
              {overdueBlock && (
                <span className="inline-flex items-center gap-1 rounded-full bg-destructive/10 px-2 py-0.5 text-xs font-medium text-destructive">
                  Tutup
                </span>
              )}
              {overdue && tugas.izinkan_late && (
                <span className="inline-flex items-center gap-1 rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">
                  Lewat deadline
                </span>
              )}
            </div>
            <p className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
              <span className="inline-flex items-center gap-1">
                <Clock className="size-3.5" />
                {formatDeadline(tugas.deadline)}
              </span>
              {tugas.izinkan_late && tugas.penalty_persen > 0 && (
                <span>Late penalty {tugas.penalty_persen}%</span>
              )}
            </p>
          </div>
          <Button asChild variant="outline" size="sm">
            <Link
              href={`/siswa/kelas/detail/tugas?id=${kelasID}&tid=${tugas.id}`}
            >
              Buka
              <ArrowRight className="size-3.5" />
            </Link>
          </Button>
        </CardHeader>
        {tugas.deskripsi && (
          <CardContent className="pt-0 text-sm text-muted-foreground line-clamp-2">
            {tugas.deskripsi}
          </CardContent>
        )}
      </Card>
    </li>
  );
}
