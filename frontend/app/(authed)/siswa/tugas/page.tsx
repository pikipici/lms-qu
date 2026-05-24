'use client';

/**
 * /siswa/tugas — riwayat semua submission siswa lintas kelas (Task 4.D.2).
 *
 * Visual: neo-brutalism + pastel pop. Stat cards 3-up dengan section accent
 * (total tugas/menunggu/graded), filter pill switcher, group by kelas,
 * row card per submission dengan badge status + late marker.
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
import {
  SiswaBadge,
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaPageHeader,
  SiswaStat,
} from '@/components/siswa-ui';

type StatusFilter = 'all' | 'submitted' | 'graded';

const TABS: { key: StatusFilter; label: string }[] = [
  { key: 'all', label: 'Semua' },
  { key: 'submitted', label: 'Menunggu nilai' },
  { key: 'graded', label: 'Sudah dinilai' },
];

function statusBadgeTone(
  status: SubmissionStatus,
  isLate: boolean,
): React.ComponentProps<typeof SiswaBadge>['tone'] {
  if (status === 'graded') return 'success';
  if (status === 'returned') return 'warning';
  if (isLate) return 'danger';
  return 'blue';
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

  const all = React.useMemo(() => subQ.data?.items ?? [], [subQ.data?.items]);
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
      <SiswaPageHeader
        eyebrow="Tugas saya"
        title="Riwayat tugas"
        description="Submission lu dari semua kelas. Klik salah satu untuk buka detail tugas + lampiran."
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <SiswaStat
          label="Total submission"
          value={counts.total}
          hint="Lintas kelas"
          Icon={ClipboardList}
          tone="tugas"
          loading={subQ.isPending}
        />
        <SiswaStat
          label="Menunggu nilai"
          value={counts.submitted}
          hint="Belum di-grade"
          Icon={Hourglass}
          tone="ulangan"
          loading={subQ.isPending}
        />
        <SiswaStat
          label="Sudah dinilai"
          value={counts.graded}
          hint="Final dari guru"
          Icon={CheckCircle2}
          tone="latihan"
          loading={subQ.isPending}
        />
      </div>

      <div className="flex flex-wrap gap-2 rounded-siswa siswa-border bg-siswa-surface p-2 siswa-shadow-sm">
        {TABS.map((tab) => {
          const active = filter === tab.key;
          return (
            <button
              key={tab.key}
              type="button"
              onClick={() => setFilter(tab.key)}
              className={cn(
                'rounded-[calc(var(--siswa-radius)-4px)] px-4 py-1.5 text-sm font-semibold transition-colors',
                active
                  ? 'border-2 border-siswa-border bg-siswa-pink siswa-shadow-sm'
                  : 'border-2 border-transparent text-siswa-text/70 hover:bg-siswa-cream/60',
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
              className="h-20 animate-pulse rounded-siswa siswa-border bg-siswa-surface/60"
            />
          ))}
        </div>
      ) : subQ.isError ? (
        <SiswaCard tone="surface" shadow="md">
          <SiswaCardBody className="p-6 text-center">
            <p className="text-sm font-semibold text-siswa-danger">
              Gagal memuat riwayat tugas. Coba refresh halaman.
            </p>
          </SiswaCardBody>
        </SiswaCard>
      ) : groups.length === 0 ? (
        <SiswaCard tone="surface" shadow="md">
          <SiswaCardBody className="p-8 text-center">
            <p className="text-sm text-siswa-text-muted">
              {filter === 'all'
                ? 'Lu belum pernah submit tugas. Buka kelas lu untuk lihat tugas yang tersedia.'
                : 'Belum ada submission yang masuk filter ini.'}
            </p>
          </SiswaCardBody>
        </SiswaCard>
      ) : (
        <div className="space-y-6">
          {groups.map((g) => (
            <section key={g.kelasID} className="space-y-3">
              <div className="flex items-center justify-between gap-2">
                <h2 className="siswa-display text-lg font-bold tracking-tight">
                  {g.kelasName}
                </h2>
                <SiswaButton asChild tone="ghost" size="sm">
                  <Link href={`/siswa/kelas/detail?id=${g.kelasID}`}>
                    Buka kelas
                    <ArrowRight className="size-4" strokeWidth={2.5} />
                  </Link>
                </SiswaButton>
              </div>
              <ul className="overflow-hidden rounded-siswa siswa-border bg-siswa-surface siswa-shadow-sm">
                {g.rows.map((r, idx) => (
                  <li
                    key={r.submission_id}
                    className={cn(
                      'flex flex-col gap-2 p-4 sm:flex-row sm:items-center sm:justify-between',
                      idx > 0 && 'border-t-2 border-siswa-border-soft',
                    )}
                  >
                    <div className="min-w-0 space-y-1.5">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="siswa-display truncate text-sm font-bold">
                          {r.judul}
                        </span>
                        <SiswaBadge tone={statusBadgeTone(r.status, r.is_late)}>
                          {statusLabel(r.status)}
                        </SiswaBadge>
                        {r.is_late ? (
                          <SiswaBadge tone="danger">LATE</SiswaBadge>
                        ) : null}
                      </div>
                      <p className="text-xs text-siswa-text-muted">
                        Dikirim {formatSubmissionTimestamp(r.submitted_at)}
                        {r.status === 'graded' && r.graded_at ? (
                          <>
                            {' · '}
                            Dinilai {formatSubmissionTimestamp(r.graded_at)}
                          </>
                        ) : null}
                      </p>
                      {r.status === 'graded' ? (
                        <p className="text-xs text-siswa-text-muted">
                          Nilai:{' '}
                          <span className="font-bold text-siswa-text">
                            {formatNilai(r.nilai_setelah_penalty)}
                          </span>
                          {(r.penalty_persen_applied ?? 0) > 0 ? (
                            <>
                              {' '}
                              <span className="text-rose-600">
                                (asli {formatNilai(r.nilai_asli)} − penalty{' '}
                                {r.penalty_persen_applied}%)
                              </span>
                            </>
                          ) : null}
                          {r.feedback ? (
                            <span className="ml-1 italic">
                              · &quot;{r.feedback}&quot;
                            </span>
                          ) : null}
                        </p>
                      ) : null}
                      {r.deadline ? (
                        <p className="text-xs text-siswa-text-muted">
                          <Clock className="mr-1 inline size-3" />
                          Deadline {formatSubmissionTimestamp(r.deadline)}
                        </p>
                      ) : null}
                    </div>
                    <SiswaButton asChild tone="surface" size="sm">
                      <Link
                        href={`/siswa/kelas/detail/tugas?id=${r.kelas_id}&tid=${r.tugas_id}`}
                      >
                        Buka tugas
                        <ArrowRight className="size-4" strokeWidth={2.5} />
                      </Link>
                    </SiswaButton>
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
