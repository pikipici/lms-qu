'use client';

/**
 * /admin/pengguna — list users (Fase 1.H.2).
 *
 * Backend contract: GET /api/v1/admin/users?role&status&q&page&page_size
 *   -> { users, page, page_size, total, total_pages }
 *
 * Pagination: server returns 1-indexed page + total. We use Prev/Next
 * buttons on top of TanStack Query keepPreviousData so the list stays
 * stable while pages swap.
 *
 * Search input is debounced 300ms before it hits the queryKey to avoid a
 * request per keystroke.
 */

import * as React from 'react';
import Link from 'next/link';
import { useQuery, keepPreviousData } from '@tanstack/react-query';
import { Plus, RotateCcw, Search } from 'lucide-react';

import { api, ApiError } from '@/lib/api';
import { type Role } from '@/lib/auth';
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

type Status = 'active' | 'suspended' | 'locked';
type RoleFilter = Role | '';
type StatusFilter = Status | '';

interface AdminUser {
  id: string;
  name: string;
  email: string;
  role: Role;
  status: Status;
  must_change_password: boolean;
  failed_login_count: number;
  last_login_at?: string | null;
  created_at: string;
  updated_at: string;
}

interface ListResponse {
  users: AdminUser[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

const PAGE_SIZE = 20;

const roleLabel: Record<Role, string> = {
  admin: 'Admin',
  guru: 'Guru',
  siswa: 'Siswa',
};

const statusLabel: Record<Status, string> = {
  active: 'Aktif',
  suspended: 'Suspended',
  locked: 'Terkunci',
};

const statusTone: Record<Status, string> = {
  active: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400',
  suspended: 'bg-amber-500/15 text-amber-700 dark:text-amber-400',
  locked: 'bg-rose-500/15 text-rose-700 dark:text-rose-400',
};

const roleTone: Record<Role, string> = {
  admin: 'bg-violet-500/15 text-violet-700 dark:text-violet-400',
  guru: 'bg-sky-500/15 text-sky-700 dark:text-sky-400',
  siswa: 'bg-slate-500/15 text-slate-700 dark:text-slate-300',
};

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

const selectClass =
  'h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50';

export default function AdminPenggunaListPage() {
  const [role, setRole] = React.useState<RoleFilter>('');
  const [status, setStatus] = React.useState<StatusFilter>('');
  const [query, setQuery] = React.useState('');
  const [page, setPage] = React.useState(1);

  const debouncedQuery = useDebounced(query, 300);

  // Reset page whenever a filter changes.
  React.useEffect(() => {
    setPage(1);
  }, [role, status, debouncedQuery]);

  const usersQuery = useQuery({
    queryKey: ['admin', 'users', { role, status, q: debouncedQuery, page }],
    queryFn: () => {
      const params = new URLSearchParams();
      if (role) params.set('role', role);
      if (status) params.set('status', status);
      if (debouncedQuery) params.set('q', debouncedQuery);
      params.set('page', String(page));
      params.set('page_size', String(PAGE_SIZE));
      return api<ListResponse>(`/admin/users?${params.toString()}`);
    },
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });

  const data = usersQuery.data;
  const users = data?.users ?? [];
  const total = data?.total ?? 0;
  const totalPages = data?.total_pages ?? 0;
  const filtersActive = role !== '' || status !== '' || query !== '';

  const onReset = () => {
    setRole('');
    setStatus('');
    setQuery('');
    setPage(1);
  };

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">Pengguna</h1>
          <p className="text-sm text-muted-foreground">
            Kelola admin, guru, dan siswa. Reset password, suspend, atau ubah
            role dari halaman detail.
          </p>
        </div>
        <Button asChild size="sm">
          <Link href="/admin/pengguna/baru">
            <Plus className="size-4" />
            Tambah pengguna
          </Link>
        </Button>
      </header>

      <Card>
        <CardHeader className="space-y-4">
          <div className="space-y-1">
            <CardTitle className="text-base">Filter</CardTitle>
            <CardDescription>
              Persempit daftar berdasarkan role, status, atau cari nama/email.
            </CardDescription>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-[1fr_180px_180px_auto]">
            <div className="space-y-1">
              <Label htmlFor="user-search" className="text-xs">
                Cari
              </Label>
              <div className="relative">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  id="user-search"
                  placeholder="Nama atau email…"
                  className="pl-8"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                />
              </div>
            </div>
            <div className="space-y-1">
              <Label htmlFor="user-role" className="text-xs">
                Role
              </Label>
              <select
                id="user-role"
                className={selectClass}
                value={role}
                onChange={(e) => setRole(e.target.value as RoleFilter)}
              >
                <option value="">Semua role</option>
                <option value="admin">Admin</option>
                <option value="guru">Guru</option>
                <option value="siswa">Siswa</option>
              </select>
            </div>
            <div className="space-y-1">
              <Label htmlFor="user-status" className="text-xs">
                Status
              </Label>
              <select
                id="user-status"
                className={selectClass}
                value={status}
                onChange={(e) => setStatus(e.target.value as StatusFilter)}
              >
                <option value="">Semua status</option>
                <option value="active">Aktif</option>
                <option value="suspended">Suspended</option>
                <option value="locked">Terkunci</option>
              </select>
            </div>
            <div className="flex items-end">
              <Button
                variant="outline"
                size="sm"
                onClick={onReset}
                disabled={!filtersActive}
                className="w-full lg:w-auto"
              >
                <RotateCcw className="size-4" />
                Reset
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto rounded-md border">
            <table className="w-full text-sm">
              <thead className="bg-muted/40 text-left text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="px-3 py-2 font-medium">Nama</th>
                  <th className="px-3 py-2 font-medium">Email</th>
                  <th className="px-3 py-2 font-medium">Role</th>
                  <th className="px-3 py-2 font-medium">Status</th>
                  <th className="px-3 py-2 font-medium">Login terakhir</th>
                  <th className="px-3 py-2 font-medium text-right">Aksi</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {usersQuery.isPending ? (
                  Array.from({ length: 5 }).map((_, i) => (
                    <tr key={`skeleton-${i}`}>
                      <td className="px-3 py-3">
                        <div className="h-3 w-32 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-3 w-48 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-5 w-14 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-5 w-16 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3">
                        <div className="h-3 w-28 animate-pulse rounded bg-muted" />
                      </td>
                      <td className="px-3 py-3 text-right">
                        <div className="ml-auto h-6 w-14 animate-pulse rounded bg-muted" />
                      </td>
                    </tr>
                  ))
                ) : usersQuery.isError ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-3 py-8 text-center text-sm text-destructive"
                    >
                      {usersQuery.error instanceof ApiError &&
                      usersQuery.error.requestId
                        ? `Gagal memuat daftar pengguna (req: ${usersQuery.error.requestId}).`
                        : 'Gagal memuat daftar pengguna.'}
                    </td>
                  </tr>
                ) : users.length === 0 ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-3 py-10 text-center text-sm text-muted-foreground"
                    >
                      {filtersActive
                        ? 'Tidak ada pengguna yang cocok dengan filter.'
                        : 'Belum ada pengguna terdaftar.'}
                    </td>
                  </tr>
                ) : (
                  users.map((u) => (
                    <tr key={u.id} className="hover:bg-muted/30">
                      <td className="px-3 py-2 font-medium">{u.name}</td>
                      <td className="px-3 py-2 break-all text-muted-foreground">
                        {u.email}
                      </td>
                      <td className="px-3 py-2">
                        <span
                          className={cn(
                            'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                            roleTone[u.role],
                          )}
                        >
                          {roleLabel[u.role]}
                        </span>
                      </td>
                      <td className="px-3 py-2">
                        <span
                          className={cn(
                            'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                            statusTone[u.status],
                          )}
                        >
                          {statusLabel[u.status]}
                        </span>
                      </td>
                      <td className="px-3 py-2 text-muted-foreground">
                        {formatDate(u.last_login_at)}
                      </td>
                      <td className="px-3 py-2 text-right">
                        <Button asChild variant="ghost" size="sm">
                          <Link href={`/admin/pengguna/${u.id}`}>Detail</Link>
                        </Button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          <div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-sm text-muted-foreground">
            <div>
              {usersQuery.isPending ? (
                <span className="text-muted-foreground">Memuat…</span>
              ) : (
                <>
                  Total {total} pengguna
                  {totalPages > 1 ? ` • Halaman ${page} / ${totalPages}` : ''}
                </>
              )}
            </div>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={page <= 1 || usersQuery.isFetching}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                Prev
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={
                  totalPages > 0 ? page >= totalPages : users.length < PAGE_SIZE
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
