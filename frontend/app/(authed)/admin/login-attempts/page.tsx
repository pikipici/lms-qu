'use client';

/**
 * /admin/login-attempts — top-level login attempts browser (Fase 1.H.5).
 *
 * Backend contract: GET /api/v1/admin/login-attempts
 *   query: email, success (true/false), since, until (RFC3339), page, page_size
 *   resp:  { attempts, page, page_size, total, total_pages }
 *
 * Filter UX:
 *   - email: free-text, debounced 300ms (server lowercases for match)
 *   - success: native select (semua / sukses / gagal)
 *   - date range: HTML date inputs → RFC3339 (UTC start/end of day)
 */

import * as React from 'react';
import { useQuery, keepPreviousData } from '@tanstack/react-query';
import { RotateCcw } from 'lucide-react';

import { api, ApiError } from '@/lib/api';
import { cn } from '@/lib/utils';
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

interface LoginAttempt {
  id: string;
  email: string;
  success: boolean;
  failure_reason?: string | null;
  ip?: string | null;
  user_agent?: string | null;
  created_at: string;
}

interface LoginAttemptsResponse {
  attempts: LoginAttempt[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

type SuccessFilter = 'all' | 'true' | 'false';

const PAGE_SIZE = 20;

const selectClass =
  'h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50';

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

function summarizeUserAgent(ua?: string | null): string {
  if (!ua) return '—';
  const lower = ua.toLowerCase();
  let os = 'Desktop';
  if (lower.includes('android')) os = 'Android';
  else if (lower.includes('iphone') || lower.includes('ipad')) os = 'iOS';
  else if (lower.includes('windows')) os = 'Windows';
  else if (lower.includes('mac os') || lower.includes('macintosh')) os = 'macOS';
  else if (lower.includes('linux')) os = 'Linux';
  let browser = 'Browser';
  if (lower.includes('edg/')) browser = 'Edge';
  else if (lower.includes('chrome/') && !lower.includes('chromium')) browser = 'Chrome';
  else if (lower.includes('firefox/')) browser = 'Firefox';
  else if (lower.includes('safari/') && !lower.includes('chrome')) browser = 'Safari';
  else if (lower.includes('curl/')) browser = 'curl';
  return `${browser} • ${os}`;
}

function useDebounced<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = React.useState(value);
  React.useEffect(() => {
    const id = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(id);
  }, [value, delay]);
  return debounced;
}

function dateToSinceISO(date: string): string | null {
  if (!date) return null;
  return `${date}T00:00:00Z`;
}

function dateToUntilISO(date: string): string | null {
  if (!date) return null;
  return `${date}T23:59:59Z`;
}

const successTone: Record<'true' | 'false', string> = {
  true: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400',
  false: 'bg-rose-500/15 text-rose-700 dark:text-rose-400',
};

export default function AdminLoginAttemptsPage() {
  const [email, setEmail] = React.useState('');
  const [success, setSuccess] = React.useState<SuccessFilter>('all');
  const [since, setSince] = React.useState('');
  const [until, setUntil] = React.useState('');
  const [page, setPage] = React.useState(1);

  const debouncedEmail = useDebounced(email.trim(), 300);

  React.useEffect(() => {
    setPage(1);
  }, [debouncedEmail, success, since, until]);

  const attemptsQuery = useQuery({
    queryKey: [
      'admin',
      'login-attempts',
      'list',
      { email: debouncedEmail, success, since, until, page },
    ],
    queryFn: () => {
      const params = new URLSearchParams();
      if (debouncedEmail) params.set('email', debouncedEmail);
      if (success !== 'all') params.set('success', success);
      const sinceIso = dateToSinceISO(since);
      const untilIso = dateToUntilISO(until);
      if (sinceIso) params.set('since', sinceIso);
      if (untilIso) params.set('until', untilIso);
      params.set('page', String(page));
      params.set('page_size', String(PAGE_SIZE));
      return api<LoginAttemptsResponse>(
        `/admin/login-attempts?${params.toString()}`,
      );
    },
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });

  const data = attemptsQuery.data;
  const attempts = data?.attempts ?? [];
  const totalPages = data?.total_pages ?? 0;
  const total = data?.total ?? 0;
  const filtersActive =
    email !== '' || success !== 'all' || since !== '' || until !== '';

  const onReset = () => {
    setEmail('');
    setSuccess('all');
    setSince('');
    setUntil('');
    setPage(1);
  };

  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Login Attempts</h1>
        <p className="text-sm text-muted-foreground">
          Riwayat percobaan login (sukses dan gagal). Berguna untuk audit
          keamanan dan investigasi akun terkunci.
        </p>
      </header>

      <Card>
        <CardHeader className="space-y-4">
          <div className="space-y-1">
            <CardTitle className="text-base">Filter</CardTitle>
            <CardDescription>
              Filter berdasarkan email, status, atau rentang tanggal.
            </CardDescription>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div className="space-y-1">
              <Label htmlFor="la-email" className="text-xs">
                Email
              </Label>
              <Input
                id="la-email"
                placeholder="nama@sekolah.id"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="la-success" className="text-xs">
                Hasil
              </Label>
              <select
                id="la-success"
                className={selectClass}
                value={success}
                onChange={(e) => setSuccess(e.target.value as SuccessFilter)}
              >
                <option value="all">Semua</option>
                <option value="true">Sukses</option>
                <option value="false">Gagal</option>
              </select>
            </div>
            <div className="space-y-1">
              <Label htmlFor="la-since" className="text-xs">
                Sejak
              </Label>
              <Input
                id="la-since"
                type="date"
                value={since}
                onChange={(e) => setSince(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="la-until" className="text-xs">
                Sampai
              </Label>
              <Input
                id="la-until"
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
                  <th className="px-3 py-2 font-medium">Email</th>
                  <th className="px-3 py-2 font-medium">Hasil</th>
                  <th className="px-3 py-2 font-medium">IP</th>
                  <th className="px-3 py-2 font-medium">Perangkat</th>
                  <th className="px-3 py-2 font-medium">Alasan gagal</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {attemptsQuery.isPending ? (
                  Array.from({ length: 5 }).map((_, i) => (
                    <tr key={`skeleton-${i}`}>
                      <td className="px-3 py-3">
                        <div className="h-3 w-32 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-3 w-40 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-5 w-14 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-3 w-24 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-3 w-28 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-3 w-32 animate-pulse rounded bg-muted" />
                      </td>
                    </tr>
                  ))
                ) : attemptsQuery.isError ? (
                  <tr>
                    <td colSpan={6} className="px-3 py-8 text-center text-sm text-destructive">
                      {attemptsQuery.error instanceof ApiError &&
                      attemptsQuery.error.requestId
                        ? `Gagal memuat login attempts (req: ${attemptsQuery.error.requestId}).`
                        : 'Gagal memuat login attempts.'}
                    </td>
                  </tr>
                ) : attempts.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="px-3 py-10 text-center text-sm text-muted-foreground">
                      {filtersActive
                        ? 'Tidak ada percobaan login yang cocok dengan filter.'
                        : 'Belum ada percobaan login tercatat.'}
                    </td>
                  </tr>
                ) : (
                  attempts.map((a) => (
                    <tr key={a.id}>
                      <td className="whitespace-nowrap px-3 py-2 text-muted-foreground">
                        {formatDate(a.created_at)}
                      </td>
                      <td className="break-all px-3 py-2">{a.email}</td>
                      <td className="px-3 py-2">
                        <span
                          className={cn(
                            'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                            successTone[a.success ? 'true' : 'false'],
                          )}
                        >
                          {a.success ? 'Sukses' : 'Gagal'}
                        </span>
                      </td>
                      <td className="px-3 py-2 text-muted-foreground">
                        {a.ip ?? '—'}
                      </td>
                      <td className="px-3 py-2 text-muted-foreground">
                        {summarizeUserAgent(a.user_agent)}
                      </td>
                      <td className="px-3 py-2 text-muted-foreground">
                        {a.failure_reason ?? '—'}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3 text-sm text-muted-foreground">
            <div>
              {attemptsQuery.isPending ? (
                <span>Memuat…</span>
              ) : (
                <>
                  Total {total} percobaan
                  {totalPages > 1 ? ` • Halaman ${page} / ${totalPages}` : ''}
                </>
              )}
            </div>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={page <= 1 || attemptsQuery.isFetching}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                Prev
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={
                  totalPages > 0 ? page >= totalPages : attempts.length < PAGE_SIZE
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
