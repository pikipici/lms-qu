'use client';

/**
 * Guru tugas review page — `?id=:kelasID&tid=:tugasID`.
 *
 * Header: tugas info (judul, status, deadline, late penalty, wajib lampiran).
 * Body: <SubmissionReviewList /> dengan filter status + grading dialog.
 *
 * Note: ownership guard di backend (kelas.guru_id == caller atau admin) —
 * kalau bukan owner, getTugas akan reject 403.
 */

import * as React from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, Calendar, Clock, Loader2, Paperclip } from 'lucide-react';

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { ApiError } from '@/lib/api';
import {
  formatDeadline,
  friendlyTugasError,
  getTugas,
} from '@/lib/tugas-api';

import { SubmissionReviewList } from '@/components/submission/SubmissionReviewList';

export default function GuruTugasReviewPage() {
  const searchParams = useSearchParams();
  const kelasID = searchParams?.get('id') ?? '';
  const tugasID = searchParams?.get('tid') ?? '';

  if (!kelasID || !tugasID) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Parameter tidak lengkap</CardTitle>
          <CardDescription>
            URL ini butuh <code>?id=:kelasID&tid=:tugasID</code>.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline" size="sm">
            <Link href="/guru/kelas">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  return <GuruTugasReviewContent kelasID={kelasID} tugasID={tugasID} />;
}

function GuruTugasReviewContent({
  kelasID,
  tugasID,
}: {
  kelasID: string;
  tugasID: string;
}) {
  const tugasQuery = useQuery({
    queryKey: ['guru', 'tugas', 'detail', tugasID],
    queryFn: () => getTugas(tugasID),
  });

  const tugas = tugasQuery.data?.tugas;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <Button asChild variant="ghost" size="sm">
          <Link href={`/guru/kelas/detail?id=${kelasID}`}>
            <ArrowLeft className="size-4" />
            Kembali ke kelas
          </Link>
        </Button>
      </div>

      {tugasQuery.isPending && (
        <Card>
          <CardContent className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
            Memuat tugas…
          </CardContent>
        </Card>
      )}

      {tugasQuery.isError && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Gagal memuat tugas</CardTitle>
            <CardDescription>
              {tugasQuery.error instanceof ApiError
                ? friendlyTugasError(tugasQuery.error, 'get')
                : (tugasQuery.error as Error).message}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => tugasQuery.refetch()}
            >
              Coba lagi
            </Button>
          </CardContent>
        </Card>
      )}

      {tugas && (
        <>
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{tugas.judul}</CardTitle>
              <CardDescription className="flex flex-wrap items-center gap-3 text-xs">
                <span
                  className={`rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide ${
                    tugas.status === 'published'
                      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
                      : tugas.status === 'draft'
                        ? 'bg-muted text-muted-foreground'
                        : 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
                  }`}
                >
                  {tugas.status}
                </span>
                <span className="inline-flex items-center gap-1">
                  <Calendar className="size-3.5" />
                  Deadline: {formatDeadline(tugas.deadline)}
                </span>
                {tugas.izinkan_late && tugas.penalty_persen > 0 && (
                  <span className="inline-flex items-center gap-1 text-amber-600">
                    <Clock className="size-3.5" />
                    Late penalty {tugas.penalty_persen}%
                  </span>
                )}
                {tugas.wajib_attachment && (
                  <span className="inline-flex items-center gap-1 text-muted-foreground">
                    <Paperclip className="size-3.5" />
                    Wajib lampiran
                  </span>
                )}
              </CardDescription>
            </CardHeader>
            {tugas.deskripsi && (
              <CardContent className="text-sm whitespace-pre-wrap">
                {tugas.deskripsi}
              </CardContent>
            )}
          </Card>

          <SubmissionReviewList
            tugasID={tugasID}
            tugasJudul={tugas.judul}
            penaltyPersen={tugas.penalty_persen}
          />
        </>
      )}
    </div>
  );
}
