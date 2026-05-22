'use client';

/**
 * /guru/kelas/detail/audit?id=:id — Guru audit log page (Task 7.E FE,
 * locked #59).
 *
 * Konsumen GET /guru/kelas/:id/audit + GET /guru/audit-actions. Filter
 * dropdown action dipopulate dari backend allowlist (single source of
 * truth — kalau allowlist BE expand, FE auto-pick).
 *
 * Pagination simple offset/limit (default 50/page) — audit volume per
 * kelas relatively low; tidak perlu cursor seperti activity feed.
 *
 * Auth: kelasGroup admin/guru only — siswa kena 403 di backend.
 */

import * as React from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, ChevronLeft, ChevronRight, History } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  listAuditActions,
  listKelasAudit,
  type AuditEntry,
} from '@/lib/audit-api';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

const PAGE_SIZE = 50;

function fmtAt(at: string): string {
  try {
    const d = new Date(at);
    return d.toLocaleString('id-ID', {
      timeZone: 'Asia/Jakarta',
      day: '2-digit',
      month: 'short',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  } catch {
    return at;
  }
}

/** Friendly Bahasa label per action — fallback to raw key. */
const ACTION_LABEL: Record<string, string> = {
  kelas_created: 'Kelas dibuat',
  kelas_updated: 'Kelas diubah',
  kelas_archived: 'Kelas diarsipkan',
  kelas_duplicated: 'Kelas diduplikasi',
  bab_created: 'Bab dibuat',
  bab_status_changed: 'Status bab berubah',
  bab_archived: 'Bab diarsipkan',
  bab_published: 'Bab dipublikasi',
  materi_created: 'Materi dibuat',
  materi_updated: 'Materi diubah',
  materi_deleted: 'Materi dihapus',
  soalbab_created: 'Soal bab dibuat',
  soalbab_bulk_created: 'Soal bab bulk dibuat',
  soalbab_deleted: 'Soal bab dihapus',
  soalbab_image_uploaded: 'Gambar soal diupload',
  soalbab_updated: 'Soal bab diubah',
  ulangan_setting_updated: 'Setting ulangan diubah',
  ulangan_bab_started: 'Ulangan bab dimulai (siswa)',
  ulangan_bab_submitted: 'Ulangan bab disubmit (siswa)',
  ulangan_bab_cancelled: 'Ulangan bab dibatalkan',
  ulangan_bab_auto_graded: 'Ulangan bab auto-grade (timer)',
  tugas_created: 'Tugas dibuat',
  tugas_status_changed: 'Status tugas berubah',
  tugas_deleted: 'Tugas dihapus',
  tugas_duplicated: 'Tugas diduplikasi',
  tugas_graded: 'Tugas dinilai',
  submission_submitted: 'Submission masuk',
  submission_graded: 'Submission dinilai',
  submission_returned: 'Submission dikembalikan',
  ujian_started: 'Ujian dimulai (siswa)',
  ujian_submitted: 'Ujian disubmit (siswa)',
  ujian_cancelled: 'Ujian dibatalkan',
  ujian_deleted: 'Ujian dihapus',
  ujian_duplicated: 'Ujian diduplikasi',
  ujian_auto_graded: 'Ujian auto-grade (timer)',
  hasil_reset: 'Hasil di-reset',
  ujian_attempt_reset: 'Attempt ujian di-reset',
  siswa_joined_kelas: 'Siswa join kelas',
  siswa_join_kelas_noop: 'Siswa join (noop)',
  admin_assigned_siswa_to_kelas: 'Admin assign siswa',
  siswa_kicked: 'Siswa dikeluarkan',
  pengumuman_created: 'Pengumuman dibuat',
  pengumuman_updated: 'Pengumuman diubah',
  pengumuman_deleted: 'Pengumuman dihapus',
};

function actionLabel(a: string): string {
  return ACTION_LABEL[a] ?? a;
}

function GuruAuditContent({ kelasID }: { kelasID: string }) {
  const [action, setAction] = React.useState<string>('');
  const [page, setPage] = React.useState<number>(1);

  // Reset to page 1 when filter changes.
  React.useEffect(() => {
    setPage(1);
  }, [action]);

  const actionsQ = useQuery({
    queryKey: ['guru', 'audit-actions'],
    queryFn: listAuditActions,
    staleTime: 5 * 60_000,
  });

  const offset = (page - 1) * PAGE_SIZE;
  const auditQ = useQuery({
    queryKey: ['guru', 'kelas', 'audit', kelasID, action, page],
    queryFn: () =>
      listKelasAudit({
        kelasId: kelasID,
        action: action || undefined,
        limit: PAGE_SIZE,
        offset,
      }),
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError) {
        if (err.status === 403 || err.status === 404 || err.status === 400) {
          return false;
        }
      }
      return failureCount < 2;
    },
  });

  const total = auditQ.data?.total ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE));

  if (auditQ.isError) {
    const err = auditQ.error;
    const msg =
      err instanceof ApiError
        ? err.status === 403
          ? 'Bukan kelas lu — akses ditolak.'
          : err.status === 404
          ? 'Kelas tidak ditemukan.'
          : err.message
        : 'Gagal memuat audit log.';
    return (
      <Card>
        <CardHeader>
          <CardTitle>Audit log</CardTitle>
          <CardDescription>Akses ditolak atau error.</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">{msg}</p>
          <div className="mt-4">
            <Button asChild variant="outline" size="sm">
              <Link href={`/guru/kelas/detail?id=${kelasID}`}>
                <ArrowLeft className="size-4" />
                Kembali ke kelas
              </Link>
            </Button>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Button asChild variant="ghost" size="sm">
            <Link href={`/guru/kelas/detail?id=${kelasID}`}>
              <ArrowLeft className="size-4" />
              Kembali
            </Link>
          </Button>
          <div>
            <h1 className="flex items-center gap-2 text-xl font-semibold tracking-tight">
              <History className="size-5" />
              Audit log kelas
            </h1>
            <p className="text-sm text-muted-foreground">
              Riwayat aktivitas yang berkaitan dgn kelas ini — admin atau guru
              lain yang melakukan perubahan terlihat di sini.
            </p>
          </div>
        </div>
      </header>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-3 pb-3">
          <div className="flex flex-1 flex-wrap items-center gap-2">
            <select
              value={action}
              onChange={(e) => setAction(e.target.value)}
              className="h-9 w-[280px] rounded-md border bg-background px-3 text-sm"
            >
              <option value="">Semua action</option>
              {(actionsQ.data?.actions ?? []).map((a) => (
                <option key={a} value={a}>
                  {actionLabel(a)}
                </option>
              ))}
            </select>
            <p className="text-xs text-muted-foreground">
              Total: {total} entry
            </p>
          </div>
          <div className="flex items-center gap-1">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1 || auditQ.isPending}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              type="button"
            >
              <ChevronLeft className="size-4" />
            </Button>
            <span className="text-xs tabular-nums text-muted-foreground">
              {page}/{pageCount}
            </span>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= pageCount || auditQ.isPending}
              onClick={() => setPage((p) => Math.min(pageCount, p + 1))}
              type="button"
            >
              <ChevronRight className="size-4" />
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {auditQ.isPending && !auditQ.data ? (
            <div className="space-y-2">
              {Array.from({ length: 5 }).map((_, i) => (
                <div
                  key={i}
                  className="h-12 animate-pulse rounded-md border bg-muted/40"
                />
              ))}
            </div>
          ) : (auditQ.data?.events.length ?? 0) === 0 ? (
            <p className="text-sm text-muted-foreground">
              Belum ada entry audit untuk filter ini.
            </p>
          ) : (
            <ul className="divide-y">
              {auditQ.data!.events.map((e) => (
                <AuditRow key={e.id} entry={e} />
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function AuditRow({ entry }: { entry: AuditEntry }) {
  const actor = entry.actor_name || entry.actor_id?.slice(0, 8) || 'Sistem';
  const role = entry.actor_role ? ` (${entry.actor_role})` : '';
  return (
    <li className="flex flex-col gap-1 py-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium">{actionLabel(entry.action)}</span>
        <span className="text-xs tabular-nums text-muted-foreground">
          {fmtAt(entry.at)}
        </span>
      </div>
      <p className="text-xs text-muted-foreground">
        Oleh: <span className="font-medium text-foreground">{actor}</span>
        {role}
      </p>
      {entry.target_type && entry.target_id ? (
        <p className="text-xs text-muted-foreground">
          Target: {entry.target_type} <span className="font-mono">{entry.target_id.slice(0, 8)}</span>
        </p>
      ) : null}
    </li>
  );
}

export default function GuruKelasAuditPage() {
  const sp = useSearchParams();
  const id = sp.get('id') ?? '';

  if (!id) {
    return (
      <div className="space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>Audit log</CardTitle>
            <CardDescription>Parameter `id` kelas tidak ditemukan.</CardDescription>
          </CardHeader>
          <CardContent>
            <Button asChild variant="outline" size="sm">
              <Link href="/guru/kelas">
                <ArrowLeft className="size-4" />
                Kembali ke daftar kelas
              </Link>
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <GuruAuditContent kelasID={id} />;
}
