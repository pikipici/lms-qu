'use client';

/**
 * /admin/audit-log — top-level audit log browser (Fase 1.H.5).
 *
 * Backend contract: GET /api/v1/admin/audit-log
 *   query: action, actor_id, target_id, since, until (RFC3339), page, page_size
 *   resp:  { events, page, page_size, total, total_pages }
 *
 * Filter UX:
 *   - action: free-text, debounced 300ms
 *   - actor_id / target_id: free-text UUID, validated client-side ringan
 *   - date range: HTML date inputs converted to RFC3339 (start-of-day Z /
 *     end-of-day Z) before query
 *   - pagination: server-driven via total_pages
 */

import * as React from 'react';
import { useQuery, keepPreviousData } from '@tanstack/react-query';
import { RotateCcw } from 'lucide-react';

import { api, ApiError } from '@/lib/api';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

interface AuditEvent {
  id: string;
  action: string;
  actor_id?: string | null;
  target_id?: string | null;
  meta?: Record<string, unknown> | null;
  ip?: string | null;
  user_agent?: string | null;
  created_at: string;
}

interface AuditResponse {
  events: AuditEvent[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

const PAGE_SIZE = 20;

function formatDate(input?: string | null): string {
  if (!input) return '—';
  try {
    return new Date(input).toLocaleString('id-ID', {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: 'Asia/Jakarta',
    });
  } catch {
    return input;
  }
}

function useDebounced<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = React.useState(value);
  React.useEffect(() => {
    const id = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(id);
  }, [value, delay]);
  return debounced;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function dateToSinceISO(date: string): string | null {
  // <input type="date"> gives YYYY-MM-DD in user's local time. We send the
  // start of that day at UTC. Acceptable approximation for the backend
  // RFC3339 parser; admin date filters do not need timezone fidelity.
  if (!date) return null;
  return `${date}T00:00:00Z`;
}

function dateToUntilISO(date: string): string | null {
  if (!date) return null;
  return `${date}T23:59:59Z`;
}

function ExpandableMeta({ meta }: { meta: Record<string, unknown> }) {
  const [open, setOpen] = React.useState(false);
  const json = JSON.stringify(meta, null, 2);
  const isShort = json.length <= 80;
  if (isShort) {
    return (
      <pre className="overflow-x-auto rounded bg-muted/40 p-2 text-xs">
        {json}
      </pre>
    );
  }
  return (
    <div className="space-y-1">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="text-xs text-primary underline-offset-2 hover:underline"
      >
        {open ? 'Tutup detail' : 'Lihat detail meta'}
      </button>
      {open ? (
        <pre className="overflow-x-auto rounded bg-muted/40 p-2 text-xs">
          {json}
        </pre>
      ) : null}
    </div>
  );
}

export default function AdminAuditLogPage() {
  const [action, setAction] = React.useState('');
  const [actorId, setActorId] = React.useState('');
  const [targetId, setTargetId] = React.useState('');
  const [since, setSince] = React.useState('');
  const [until, setUntil] = React.useState('');
  const [page, setPage] = React.useState(1);

  const debouncedAction = useDebounced(action, 300);
  const debouncedActor = useDebounced(actorId.trim(), 300);
  const debouncedTarget = useDebounced(targetId.trim(), 300);

  const actorValid = debouncedActor === '' || UUID_RE.test(debouncedActor);
  const targetValid = debouncedTarget === '' || UUID_RE.test(debouncedTarget);

  React.useEffect(() => {
    setPage(1);
  }, [debouncedAction, debouncedActor, debouncedTarget, since, until]);

  const auditQuery = useQuery({
    queryKey: [
      'admin',
      'audit-log',
      'list',
      {
        action: debouncedAction,
        actor_id: actorValid ? debouncedActor : '',
        target_id: targetValid ? debouncedTarget : '',
        since,
        until,
        page,
      },
    ],
    queryFn: () => {
      const params = new URLSearchParams();
      if (debouncedAction) params.set('action', debouncedAction);
      if (actorValid && debouncedActor) params.set('actor_id', debouncedActor);
      if (targetValid && debouncedTarget) params.set('target_id', debouncedTarget);
      const sinceIso = dateToSinceISO(since);
      const untilIso = dateToUntilISO(until);
      if (sinceIso) params.set('since', sinceIso);
      if (untilIso) params.set('until', untilIso);
      params.set('page', String(page));
      params.set('page_size', String(PAGE_SIZE));
      return api<AuditResponse>(`/admin/audit-log?${params.toString()}`);
    },
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });

  const data = auditQuery.data;
  const events = data?.events ?? [];
  const totalPages = data?.total_pages ?? 0;
  const total = data?.total ?? 0;
  const filtersActive =
    action !== '' ||
    actorId !== '' ||
    targetId !== '' ||
    since !== '' ||
    until !== '';

  const onReset = () => {
    setAction('');
    setActorId('');
    setTargetId('');
    setSince('');
    setUntil('');
    setPage(1);
  };

  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Audit Log</h1>
        <p className="text-sm text-muted-foreground">
          Riwayat semua aksi admin terhadap akun pengguna dan sesi. Filter
          berdasarkan action, actor, target, atau rentang tanggal.
        </p>
      </header>

      <Card>
        <CardHeader className="space-y-4">
          <div className="space-y-1">
            <CardTitle className="text-base">Filter</CardTitle>
            <CardDescription>
              Action contoh: admin_user_created, admin_user_role_changed,
              admin_user_sessions_revoked, admin_user_password_reset.
            </CardDescription>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
            <div className="space-y-1 xl:col-span-2">
              <Label htmlFor="audit-action" className="text-xs">
                Action
              </Label>
              <Input
                id="audit-action"
                placeholder="mis. admin_user_role_changed"
                value={action}
                onChange={(e) => setAction(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="audit-actor" className="text-xs">
                Actor ID (UUID)
              </Label>
              <Input
                id="audit-actor"
                placeholder="uuid"
                value={actorId}
                onChange={(e) => setActorId(e.target.value)}
                aria-invalid={actorId !== '' && !actorValid}
              />
              {actorId !== '' && !actorValid ? (
                <p className="text-xs text-destructive">Format UUID tidak valid.</p>
              ) : null}
            </div>
            <div className="space-y-1">
              <Label htmlFor="audit-target" className="text-xs">
                Target ID (UUID)
              </Label>
              <Input
                id="audit-target"
                placeholder="uuid"
                value={targetId}
                onChange={(e) => setTargetId(e.target.value)}
                aria-invalid={targetId !== '' && !targetValid}
              />
              {targetId !== '' && !targetValid ? (
                <p className="text-xs text-destructive">Format UUID tidak valid.</p>
              ) : null}
            </div>
            <div className="space-y-1">
              <Label htmlFor="audit-since" className="text-xs">
                Sejak
              </Label>
              <Input
                id="audit-since"
                type="date"
                value={since}
                onChange={(e) => setSince(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="audit-until" className="text-xs">
                Sampai
              </Label>
              <Input
                id="audit-until"
                type="date"
                value={until}
                onChange={(e) => setUntil(e.target.value)}
              />
            </div>
          </div>
          <div className="flex justify-end">
            <Button
              variant="outline"
              size="sm"
              onClick={onReset}
              disabled={!filtersActive}
            >
              <RotateCcw className="size-4" />
              Reset filter
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="overflow-x-auto rounded-md border">
            <table className="w-full text-sm">
              <thead className="bg-muted/40 text-left text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="px-3 py-2 font-medium">Waktu</th>
                  <th className="px-3 py-2 font-medium">Action</th>
                  <th className="px-3 py-2 font-medium">Actor</th>
                  <th className="px-3 py-2 font-medium">Target</th>
                  <th className="px-3 py-2 font-medium">Meta</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {auditQuery.isPending ? (
                  Array.from({ length: 5 }).map((_, i) => (
                    <tr key={`skeleton-${i}`}>
                      <td className="px-3 py-3">
                        <div className="h-3 w-32 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-3 w-44 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-3 w-28 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-3 w-28 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-3 w-40 animate-pulse rounded bg-muted" />
                      </td>
                    </tr>
                  ))
                ) : auditQuery.isError ? (
                  <tr>
                    <td colSpan={5} className="px-3 py-8 text-center text-sm text-destructive">
                      {auditQuery.error instanceof ApiError &&
                      auditQuery.error.requestId
                        ? `Gagal memuat audit log (req: ${auditQuery.error.requestId}).`
                        : 'Gagal memuat audit log.'}
                    </td>
                  </tr>
                ) : events.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="px-3 py-10 text-center text-sm text-muted-foreground">
                      {filtersActive
                        ? 'Tidak ada event yang cocok dengan filter.'
                        : 'Belum ada event audit tercatat.'}
                    </td>
                  </tr>
                ) : (
                  events.map((e) => (
                    <tr key={e.id} className="align-top">
                      <td className="whitespace-nowrap px-3 py-2 text-muted-foreground">
                        {formatDate(e.created_at)}
                      </td>
                      <td className="px-3 py-2">
                        <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
                          {e.action}
                        </code>
                      </td>
                      <td className="px-3 py-2">
                        <code className="break-all font-mono text-xs text-muted-foreground">
                          {e.actor_id ?? '—'}
                        </code>
                      </td>
                      <td className="px-3 py-2">
                        <code className="break-all font-mono text-xs text-muted-foreground">
                          {e.target_id ?? '—'}
                        </code>
                      </td>
                      <td className="px-3 py-2">
                        {e.meta && Object.keys(e.meta).length > 0 ? (
                          <ExpandableMeta meta={e.meta} />
                        ) : (
                          <span className="text-xs text-muted-foreground">—</span>
                        )}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3 text-sm text-muted-foreground">
            <div>
              {auditQuery.isPending ? (
                <span>Memuat…</span>
              ) : (
                <>
                  Total {total} event
                  {totalPages > 1 ? ` • Halaman ${page} / ${totalPages}` : ''}
                </>
              )}
            </div>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={page <= 1 || auditQuery.isFetching}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                Prev
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={
                  totalPages > 0 ? page >= totalPages : events.length < PAGE_SIZE
                }
                onClick={() => setPage((p) => p + 1)}
              >
                Next
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
