'use client';

/**
 * /siswa/tugas — riwayat semua submission siswa lintas kelas (Task 4.D.2).
 *
 * Sumber data: GET /siswa/submissions (handler ListMine, locked #74 BE +
 * Task 4.D.2 lock). JOIN-backed di backend, satu fetch sudah lengkap dengan
 * tugas snapshot — gak butuh per-row tugas fetch.
 *
 * UI:
 *   - Header dengan ringkasan total + filter status (all/submitted/graded)
 *   - Group by kelas — title kelas + list submission card di bawahnya
 *   - Card per submission: judul tugas, status, nilai (kalau graded),
 *     late badge, link ke /siswa/kelas/detail/tugas?id={kelas}&tid={tugas}
 *
 * Note: status filter "all/submitted/graded" — `returned` di-defer MVP
 * (locked #73 BE).
 */

import * as React from 'react';
import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import {
  ArrowRight,
  CheckCircle2,
  Clock,
  ClipboardList,
  Hourglass,
} from 'lucide-react';

import {
  listMySubmissions,
  formatSubmissionTimestamp,
  statusLabel,
  type MySubmissionItem,
  type SubmissionStatus,
} from '@/lib/submission-api';
import { listMyKelas } from '@/lib/siswa-api';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

type StatusFilter = 'all' | 'submitted' | 'graded';

const TABS: { key: StatusFilter; label: string }[] = [
  { key: 'all', label: 'Semua' },
  { key: 'submitted', label: 'Menunggu nilai' },
  { key: 'graded', label: 'Sudah dinilai' },
];

function statusBadgeClass(status: SubmissionStatus, isLate: boolean): string {
  const base =
    'inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium';
  if (status === 'graded') {
    return cn(base, 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300');
  }
  if (status === 'returned') {
    return cn(base, 'bg-amber-500/15 text-amber-700 dark:text-amber-300');
  }
  // submitted
  if (isLate) {
    return cn(base, 'bg-rose-500/15 text-rose-700 dark:text-rose-300');
  }
  return cn(base, 'bg-blue-500/15 text-blue-700 dark:text-blue-300');
}

function formatNilai(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—';
  return Number.isInteger(n) ? String(n) : n.toFixed(2);
}

function groupByKelas(
  rows: MySubmissionItem[],
  kelasName: Record<string, string>,
): { kelasID: string; kelasName: string; rows: MySubmissionItem[] }[] {
  const map = new Map<string, MySubmissionItem[]>();
  for (const r of rows) {
    if (!map.has(r.kelas_id)) map.set(r.kelas_id, []);
    map.get(r.kelas_id)!.push(r);
  }
  // Sort group by latest submitted_at desc.
  const groups = Array.from(map.entries()).map(([kelasID, list]) => {
    list.sort((a, b) => b.submitted_at.localeCompare(a.submitted_at));
    return {
      kelasID,
      kelasName: kelasName[kelasID] ?? 'Kelas',
      rows: list,
    };
  });
  groups.sort((a, b) =>
    b.rows[0]!.submitted_at.localeCompare(a.rows[0]!.submitted_at),
  );
  return groups;
}

export default function SiswaTugasPage() {
  const [filter, setFilter] = React.useState<StatusFilter>('all');

  const subQ = useQuery({
    queryKey: ['siswa', 'submissions', 'list'],
    queryFn: () => listMySubmissions({ limit: 200 }),
    staleTime: 15_000,
  });

  // Hydrate kelas name (server endpoint flat, no kelas join — simpler kalau
  // FE merge dari list-my-kelas yang udah cached).
  const kelasQ = useQuery({
    queryKey: ['siswa', 'kelas', 'list'],
    queryFn: () => listMyKelas({ page: 1, pageSize: 50 }),
    staleTime: 60_000,
  });

  const kelasName = React.useMemo(() => {
    const out: Record<string, string> = {};
    for (const it of kelasQ.data?.items ?? []) {
      out[it.kelas.id] = it.kelas.nama;
    }
    return out;
  }, [kelasQ.data?.items]);

  const all = subQ.data?.items ?? [];
  const filtered = React.useMemo(() => {
    if (filter === 'all') return all;
    return all.filter((r) => r.status === filter);
  }, [all, filter]);

  const groups = React.useMemo(
    () => groupByKelas(filtered, kelasName),
    [filtered, kelasName],
  );

  const counts = React.useMemo(() => {
    let submitted = 0;
    let graded = 0;
    for (const r of all) {
      if (r.status === 'submitted') submitted++;
      else if (r.status === 'graded') graded++;
    }
    return { submitted, graded, total: all.length };
  }, [all]);

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">Tugas saya</h1>
          <p className="text-sm text-muted-foreground">
            Riwayat submission lu dari semua kelas. Klik salah satu untuk buka
            detail tugas + lampiran lu.
          </p>
        </div>
      </header>

      <div className="grid gap-4 sm:grid-cols-3">
        <Card>
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-sm">Total submission</CardTitle>
              <CardDescription>Lintas kelas</CardDescription>
            </div>
            <ClipboardList className="size-5 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <span className="text-2xl font-semibold">{counts.total}</span>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-sm">Menunggu nilai</CardTitle>
              <CardDescription>Belum di-grade</CardDescription>
            </div>
            <Hourglass className="size-5 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <span className="text-2xl font-semibold">{counts.submitted}</span>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-sm">Sudah dinilai</CardTitle>
              <CardDescription>Final dari guru</CardDescription>
            </div>
            <CheckCircle2 className="size-5 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <span className="text-2xl font-semibold">{counts.graded}</span>
          </CardContent>
        </Card>
      </div>

      <div className="flex flex-wrap items-center gap-2 border-b pb-2">
        {TABS.map((tab) => {
          const active = filter === tab.key;
          return (
            <button
              key={tab.key}
              type="button"
              onClick={() => setFilter(tab.key)}
              className={cn(
                'rounded-md px-3 py-1.5 text-sm transition-colors',
                active
                  ? 'bg-accent text-accent-foreground font-medium'
                  : 'text-muted-foreground hover:bg-accent/50',
              )}
            >
              {tab.label}
            </button>
          );
        })}
      </div>

      {subQ.isPending ? (
        <div className="space-y-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <div
              key={i}
              className="h-16 animate-pulse rounded-md border bg-muted/40"
            />
          ))}
        </div>
      ) : subQ.isError ? (
        <Card>
          <CardContent className="p-6 text-center">
            <p className="text-sm text-destructive">
              Gagal memuat riwayat tugas. Coba refresh halaman.
            </p>
          </CardContent>
        </Card>
      ) : groups.length === 0 ? (
        <Card>
          <CardContent className="p-8 text-center">
            <p className="text-sm text-muted-foreground">
              {filter === 'all'
                ? 'Lu belum pernah submit tugas. Buka kelas lu untuk lihat tugas yang tersedia.'
                : 'Belum ada submission yang masuk filter ini.'}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-6">
          {groups.map((g) => (
            <section key={g.kelasID} className="space-y-3">
              <div className="flex items-center justify-between gap-2">
                <h2 className="text-base font-semibold tracking-tight">
                  {g.kelasName}
                </h2>
                <Button asChild variant="ghost" size="sm">
                  <Link href={`/siswa/kelas/detail?id=${g.kelasID}`}>
                    Buka kelas
                    <ArrowRight className="size-4" />
                  </Link>
                </Button>
              </div>
              <ul className="divide-y rounded-md border">
                {g.rows.map((r) => (
                  <li
                    key={r.submission_id}
                    className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:justify-between"
                  >
                    <div className="min-w-0 space-y-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="truncate text-sm font-medium">
                          {r.judul}
                        </span>
                        <span className={statusBadgeClass(r.status, r.is_late)}>
                          {statusLabel(r.status)}
                        </span>
                        {r.is_late && (
                          <span className="inline-flex items-center gap-1 rounded bg-rose-500/15 px-1.5 py-0.5 text-[10px] font-medium text-rose-700 dark:text-rose-300">
                            LATE
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-muted-foreground">
                        Dikirim {formatSubmissionTimestamp(r.submitted_at)}
                        {r.status === 'graded' && r.graded_at && (
                          <>
                            {' · '}
                            Dinilai {formatSubmissionTimestamp(r.graded_at)}
                          </>
                        )}
                      </p>
                      {r.status === 'graded' && (
                        <p className="text-xs text-muted-foreground">
                          Nilai:{' '}
                          <span className="font-semibold text-foreground">
                            {formatNilai(r.nilai_setelah_penalty)}
                          </span>
                          {(r.penalty_persen_applied ?? 0) > 0 && (
                            <>
                              {' '}
                              <span className="text-rose-600">
                                (asli {formatNilai(r.nilai_asli)} − penalty{' '}
                                {r.penalty_persen_applied}%)
                              </span>
                            </>
                          )}
                          {r.feedback && (
                            <span className="ml-1 italic">
                              · &quot;{r.feedback}&quot;
                            </span>
                          )}
                        </p>
                      )}
                      {r.deadline && (
                        <p className="text-xs text-muted-foreground">
                          <Clock className="mr-1 inline size-3" />
                          Deadline {formatSubmissionTimestamp(r.deadline)}
                        </p>
                      )}
                    </div>
                    <Button asChild variant="ghost" size="sm">
                      <Link
                        href={`/siswa/kelas/detail/tugas?id=${r.kelas_id}&tid=${r.tugas_id}`}
                      >
                        Buka tugas
                        <ArrowRight className="size-4" />
                      </Link>
                    </Button>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}
