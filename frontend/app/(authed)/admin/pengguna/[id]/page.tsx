'use client';

/**
 * /admin/pengguna/[id] — admin user detail page (Fase 1.H.4).
 *
 * Backend contracts:
 *   - GET    /admin/users/:id                 -> { user }
 *   - PATCH  /admin/users/:id                 body { name } -> { user }
 *   - POST   /admin/users/:id/role            body { new_role, current_password } -> { user }
 *   - POST   /admin/users/:id/reset-password  body { password_strategy, password? } -> { user, generated_password? }
 *   - POST   /admin/users/:id/suspend         body { reason? } -> { user }
 *   - POST   /admin/users/:id/unsuspend       -> { user }
 *   - POST   /admin/users/:id/unlock          -> { user }
 *   - GET    /admin/users/:id/sessions        -> { sessions }
 *   - POST   /admin/users/:id/revoke-sessions body { reason? } -> { revoked_count }
 *   - GET    /admin/audit-log?actor_id=:id|target_id=:id&page&page_size
 *   - GET    /admin/login-attempts?email=:email&page&page_size
 *
 * Locked decisions:
 *   - #31 Password reveal once: reset-password success view shows the
 *         plaintext password ONCE; closing the dialog wipes it.
 *   - #52 Re-auth on promote/demote: ChangeRoleDialog requires the admin's
 *         current_password before submitting.
 *   - #60 API base baked at build time via NEXT_PUBLIC_API_BASE.
 */

import * as React from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  useQuery,
  useMutation,
  useQueryClient,
  keepPreviousData,
} from '@tanstack/react-query';
import {
  ArrowLeft,
  ClipboardCheck,
  ClipboardCopy,
  KeyRound,
  Pencil,
  ShieldAlert,
  ShieldCheck,
  ShieldOff,
  Unlock,
  UserCog,
  Users,
} from 'lucide-react';

import { api, ApiError } from '@/lib/api';
import { type Role } from '@/lib/auth';
import { cn } from '@/lib/utils';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

// ---------- Types ----------

type Status = 'active' | 'suspended' | 'locked';

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

interface UserResponse {
  user: AdminUser;
}

interface UserMutationResponse {
  user: AdminUser;
}

interface ResetPasswordResponse {
  user: AdminUser;
  generated_password?: string | null;
}

interface Session {
  id: string;
  jti: string;
  user_id: string;
  issued_at: string;
  expires_at: string;
  revoked_at?: string | null;
  ip?: string | null;
  user_agent?: string | null;
}

interface SessionsResponse {
  sessions: Session[];
}

interface RevokeSessionsResponse {
  revoked_count: number;
}

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

// ---------- Constants & helpers ----------

const PAGE_SIZE = 10;

const roleLabel: Record<Role, string> = {
  admin: 'Administrator',
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

function summarizeUserAgent(ua?: string | null): string {
  if (!ua) return 'Perangkat tidak dikenal';
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

function maskJti(jti: string): string {
  if (jti.length <= 8) return jti;
  return `${jti.slice(0, 4)}…${jti.slice(-4)}`;
}

function friendlyError(err: ApiError, fallback = 'Terjadi kesalahan.'): string {
  switch (err.code) {
    case 'invalid_credentials':
    case 'invalid_password':
      return 'Password admin salah.';
    case 'cannot_self_demote':
      return 'Tidak bisa menurunkan role akun Anda sendiri.';
    case 'cannot_self_suspend':
      return 'Tidak bisa men-suspend akun Anda sendiri.';
    case 'invalid_role':
      return 'Role tidak dikenal.';
    case 'invalid_strategy':
      return 'Strategi password tidak dikenal.';
    case 'weak_password':
      return 'Password terlalu lemah (minimal 8 karakter).';
    case 'conflicting_password':
      return 'Strategi "Generate" tidak boleh diisi password manual.';
    case 'user_not_found':
      return 'Pengguna tidak ditemukan.';
    case 'not_locked':
      return 'Akun tidak dalam kondisi terkunci.';
    default:
      return err.message || fallback;
  }
}

function withReqId(err: ApiError, msg: string): string {
  return err.requestId ? `${msg} (req: ${err.requestId})` : msg;
}

// ---------- Small UI bits ----------

function Badge({
  className,
  children,
}: {
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
        className,
      )}
    >
      {children}
    </span>
  );
}

function CopyButton({ value, label }: { value: string; label?: string }) {
  const [copied, setCopied] = React.useState(false);
  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      const ta = document.createElement('textarea');
      ta.value = value;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      try {
        document.execCommand('copy');
        setCopied(true);
        window.setTimeout(() => setCopied(false), 2000);
      } finally {
        document.body.removeChild(ta);
      }
    }
  };
  return (
    <Button type="button" size="sm" variant="outline" onClick={onCopy} className="gap-2">
      {copied ? (
        <>
          <ClipboardCheck className="size-4" />
          Tersalin
        </>
      ) : (
        <>
          <ClipboardCopy className="size-4" />
          {label ?? 'Salin'}
        </>
      )}
    </Button>
  );
}

// ---------- Tabs (lightweight, no extra deps) ----------

type TabKey = 'detail' | 'sessions' | 'audit' | 'attempts';

const tabList: { key: TabKey; label: string }[] = [
  { key: 'detail', label: 'Detail' },
  { key: 'sessions', label: 'Sesi Aktif' },
  { key: 'audit', label: 'Riwayat Audit' },
  { key: 'attempts', label: 'Login Attempts' },
];

function TabBar({
  active,
  onChange,
}: {
  active: TabKey;
  onChange: (k: TabKey) => void;
}) {
  return (
    <div className="border-b">
      <nav className="-mb-px flex flex-wrap gap-x-2 gap-y-1" role="tablist">
        {tabList.map((t) => {
          const isActive = t.key === active;
          return (
            <button
              key={t.key}
              type="button"
              role="tab"
              aria-selected={isActive}
              onClick={() => onChange(t.key)}
              className={cn(
                'border-b-2 px-3 py-2 text-sm font-medium transition-colors',
                isActive
                  ? 'border-primary text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground',
              )}
            >
              {t.label}
            </button>
          );
        })}
      </nav>
    </div>
  );
}

// ---------- Detail tab ----------

function DetailTab({ user }: { user: AdminUser }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Profil pengguna</CardTitle>
        <CardDescription>
          Data identitas akun. Edit nama lewat tombol di header.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-[180px_1fr]">
          <dt className="text-muted-foreground">ID</dt>
          <dd className="break-all font-mono text-xs">{user.id}</dd>
          <dt className="text-muted-foreground">Nama</dt>
          <dd className="font-medium">{user.name}</dd>
          <dt className="text-muted-foreground">Email</dt>
          <dd className="break-all">{user.email}</dd>
          <dt className="text-muted-foreground">Role</dt>
          <dd>{roleLabel[user.role]}</dd>
          <dt className="text-muted-foreground">Status</dt>
          <dd>{statusLabel[user.status]}</dd>
          <dt className="text-muted-foreground">Wajib ganti password</dt>
          <dd>{user.must_change_password ? 'Ya' : 'Tidak'}</dd>
          <dt className="text-muted-foreground">Login gagal berturut-turut</dt>
          <dd>{user.failed_login_count}</dd>
          <dt className="text-muted-foreground">Login terakhir</dt>
          <dd>{formatDate(user.last_login_at)}</dd>
          <dt className="text-muted-foreground">Dibuat</dt>
          <dd>{formatDate(user.created_at)}</dd>
          <dt className="text-muted-foreground">Diperbarui</dt>
          <dd>{formatDate(user.updated_at)}</dd>
        </dl>
      </CardContent>
    </Card>
  );
}

// ---------- Sessions tab ----------

function SessionsTab({ userId }: { userId: string }) {
  const sessionsQuery = useQuery({
    queryKey: ['admin', 'users', userId, 'sessions'],
    queryFn: () => api<SessionsResponse>(`/admin/users/${userId}/sessions`),
    staleTime: 15_000,
  });

  const sessions = sessionsQuery.data?.sessions ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Sesi aktif</CardTitle>
        <CardDescription>
          Refresh token yang masih hidup. Cabut semua via tombol{' '}
          <span className="font-medium">Logout dari semua perangkat</span> di
          header.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {sessionsQuery.isPending ? (
          <p className="text-sm text-muted-foreground">Memuat…</p>
        ) : sessionsQuery.isError ? (
          <p className="text-sm text-destructive">
            {sessionsQuery.error instanceof ApiError &&
            sessionsQuery.error.requestId
              ? `Gagal memuat sesi (req: ${sessionsQuery.error.requestId}).`
              : 'Gagal memuat sesi.'}
          </p>
        ) : sessions.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Tidak ada sesi aktif tercatat.
          </p>
        ) : (
          <ul className="divide-y divide-border rounded-md border">
            {sessions.map((s) => (
              <li key={s.id} className="flex flex-col gap-1 p-4 text-sm">
                <div className="font-medium">{summarizeUserAgent(s.user_agent)}</div>
                <div className="text-xs text-muted-foreground">
                  IP {s.ip ?? '—'} • JTI {maskJti(s.jti)}
                </div>
                <div className="text-xs text-muted-foreground">
                  Mulai {formatDate(s.issued_at)} • Berakhir{' '}
                  {formatDate(s.expires_at)}
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

// ---------- Audit tab ----------

function AuditList({
  title,
  description,
  filterParam,
  userId,
}: {
  title: string;
  description: string;
  filterParam: 'actor_id' | 'target_id';
  userId: string;
}) {
  const [page, setPage] = React.useState(1);
  const auditQuery = useQuery({
    queryKey: ['admin', 'audit-log', filterParam, userId, page],
    queryFn: () => {
      const params = new URLSearchParams();
      params.set(filterParam, userId);
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

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {auditQuery.isPending ? (
          <p className="text-sm text-muted-foreground">Memuat…</p>
        ) : auditQuery.isError ? (
          <p className="text-sm text-destructive">Gagal memuat riwayat audit.</p>
        ) : events.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Belum ada catatan audit.
          </p>
        ) : (
          <ul className="divide-y divide-border rounded-md border">
            {events.map((e) => (
              <li key={e.id} className="flex flex-col gap-1 p-3 text-sm">
                <div className="flex flex-wrap items-center gap-2">
                  <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
                    {e.action}
                  </code>
                  <span className="text-xs text-muted-foreground">
                    {formatDate(e.created_at)}
                  </span>
                </div>
                {e.meta && Object.keys(e.meta).length > 0 ? (
                  <pre className="overflow-x-auto rounded bg-muted/40 p-2 text-xs">
                    {JSON.stringify(e.meta, null, 2)}
                  </pre>
                ) : null}
                {e.ip || e.user_agent ? (
                  <div className="text-xs text-muted-foreground">
                    IP {e.ip ?? '—'} • {summarizeUserAgent(e.user_agent)}
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        )}

        <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground">
          <div>
            Total {total}
            {totalPages > 1 ? ` • Halaman ${page} / ${totalPages}` : ''}
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
  );
}

function AuditTab({ userId }: { userId: string }) {
  return (
    <div className="space-y-4">
      <AuditList
        title="Sebagai Aktor"
        description="Aksi yang dilakukan oleh pengguna ini (ketika dia adalah admin yang bertindak)."
        filterParam="actor_id"
        userId={userId}
      />
      <AuditList
        title="Sebagai Target"
        description="Aksi yang dilakukan terhadap akun pengguna ini."
        filterParam="target_id"
        userId={userId}
      />
    </div>
  );
}

// ---------- Login attempts tab ----------

function LoginAttemptsTab({ email }: { email: string }) {
  const [page, setPage] = React.useState(1);
  const attemptsQuery = useQuery({
    queryKey: ['admin', 'login-attempts', email, page],
    queryFn: () => {
      const params = new URLSearchParams();
      params.set('email', email);
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

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Login attempts</CardTitle>
        <CardDescription>
          Riwayat percobaan login untuk email {email}.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {attemptsQuery.isPending ? (
          <p className="text-sm text-muted-foreground">Memuat…</p>
        ) : attemptsQuery.isError ? (
          <p className="text-sm text-destructive">
            Gagal memuat riwayat login.
          </p>
        ) : attempts.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Belum ada percobaan login.
          </p>
        ) : (
          <div className="overflow-x-auto rounded-md border">
            <table className="w-full text-sm">
              <thead className="bg-muted/40 text-left text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="px-3 py-2 font-medium">Waktu</th>
                  <th className="px-3 py-2 font-medium">Hasil</th>
                  <th className="px-3 py-2 font-medium">IP</th>
                  <th className="px-3 py-2 font-medium">Perangkat</th>
                  <th className="px-3 py-2 font-medium">Alasan gagal</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {attempts.map((a) => (
                  <tr key={a.id}>
                    <td className="px-3 py-2 whitespace-nowrap">
                      {formatDate(a.created_at)}
                    </td>
                    <td className="px-3 py-2">
                      {a.success ? (
                        <Badge className="bg-emerald-500/15 text-emerald-700 dark:text-emerald-400">
                          Sukses
                        </Badge>
                      ) : (
                        <Badge className="bg-rose-500/15 text-rose-700 dark:text-rose-400">
                          Gagal
                        </Badge>
                      )}
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
                ))}
              </tbody>
            </table>
          </div>
        )}

        <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground">
          <div>
            Total {total}
            {totalPages > 1 ? ` • Halaman ${page} / ${totalPages}` : ''}
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
  );
}

// ---------- Edit name dialog ----------

const editNameSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, { message: 'Nama wajib diisi.' })
    .max(120, { message: 'Maksimal 120 karakter.' }),
});

function EditNameDialog({
  user,
  open,
  onOpenChange,
}: {
  user: AdminUser;
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const form = useForm<z.infer<typeof editNameSchema>>({
    resolver: zodResolver(editNameSchema),
    defaultValues: { name: user.name },
  });

  React.useEffect(() => {
    if (open) form.reset({ name: user.name });
  }, [open, user.name, form]);

  const mutation = useMutation({
    mutationFn: (input: { name: string }) =>
      api<UserMutationResponse>(`/admin/users/${user.id}`, {
        method: 'PATCH',
        body: input,
      }),
    onSuccess: (data) => {
      qc.setQueryData<UserResponse>(['admin', 'users', user.id], { user: data.user });
      qc.invalidateQueries({ queryKey: ['admin', 'users'] });
      toast({ title: 'Nama berhasil diperbarui' });
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      const apiErr =
        err instanceof ApiError
          ? err
          : new ApiError({ status: 0, code: 'unknown', message: 'Gagal terhubung.' });
      toast({
        variant: 'destructive',
        title: 'Gagal memperbarui nama',
        description: withReqId(apiErr, friendlyError(apiErr)),
      });
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit nama pengguna</DialogTitle>
          <DialogDescription>
            Update nama tampilan untuk akun {user.email}.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit((v) => mutation.mutate(v))}
            className="space-y-4"
            noValidate
          >
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Nama lengkap</FormLabel>
                  <FormControl>
                    <Input autoFocus disabled={mutation.isPending} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button
                type="button"
                variant="ghost"
                onClick={() => onOpenChange(false)}
                disabled={mutation.isPending}
              >
                Batal
              </Button>
              <Button type="submit" disabled={mutation.isPending}>
                {mutation.isPending ? 'Menyimpan…' : 'Simpan'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

// ---------- Change role dialog (re-auth) ----------

const changeRoleSchema = z.object({
  new_role: z.enum(['admin', 'guru', 'siswa']),
  current_password: z
    .string()
    .min(1, { message: 'Password admin wajib diisi.' }),
});

function ChangeRoleDialog({
  user,
  open,
  onOpenChange,
}: {
  user: AdminUser;
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const form = useForm<z.infer<typeof changeRoleSchema>>({
    resolver: zodResolver(changeRoleSchema),
    defaultValues: {
      new_role: user.role === 'admin' ? 'guru' : 'admin',
      current_password: '',
    },
  });

  React.useEffect(() => {
    if (open) {
      form.reset({
        new_role: user.role === 'admin' ? 'guru' : 'admin',
        current_password: '',
      });
    }
  }, [open, user.role, form]);

  const mutation = useMutation({
    mutationFn: (input: z.infer<typeof changeRoleSchema>) =>
      api<UserMutationResponse>(`/admin/users/${user.id}/role`, {
        method: 'POST',
        body: input,
      }),
    onSuccess: (data) => {
      qc.setQueryData<UserResponse>(['admin', 'users', user.id], { user: data.user });
      qc.invalidateQueries({ queryKey: ['admin', 'users'] });
      qc.invalidateQueries({ queryKey: ['admin', 'audit-log'] });
      toast({ title: 'Role berhasil diubah' });
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      const apiErr =
        err instanceof ApiError
          ? err
          : new ApiError({ status: 0, code: 'unknown', message: 'Gagal terhubung.' });
      toast({
        variant: 'destructive',
        title: 'Gagal mengubah role',
        description: withReqId(apiErr, friendlyError(apiErr)),
      });
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Ubah peran pengguna</DialogTitle>
          <DialogDescription>
            Promote atau demote akun {user.email}. Aksi ini wajib diverifikasi
            dengan password admin Anda saat ini.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit((v) => mutation.mutate(v))}
            className="space-y-4"
            noValidate
          >
            <FormField
              control={form.control}
              name="new_role"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Role baru</FormLabel>
                  <FormControl>
                    <select
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                      disabled={mutation.isPending}
                      {...field}
                    >
                      {(['admin', 'guru', 'siswa'] as Role[])
                        .filter((r) => r !== user.role)
                        .map((r) => (
                          <option key={r} value={r}>
                            {roleLabel[r]}
                          </option>
                        ))}
                    </select>
                  </FormControl>
                  <FormDescription>
                    Role saat ini: {roleLabel[user.role]}.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="current_password"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Password admin Anda</FormLabel>
                  <FormControl>
                    <Input
                      type="password"
                      autoComplete="current-password"
                      disabled={mutation.isPending}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    Verifikasi ulang password admin yang sedang login.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button
                type="button"
                variant="ghost"
                onClick={() => onOpenChange(false)}
                disabled={mutation.isPending}
              >
                Batal
              </Button>
              <Button type="submit" disabled={mutation.isPending}>
                {mutation.isPending ? 'Memproses…' : 'Ubah role'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

// ---------- Reset password dialog ----------

const resetPasswordSchema = z
  .object({
    password_strategy: z.enum(['manual', 'generate']),
    password: z.string().default(''),
  })
  .superRefine((value, ctx) => {
    if (value.password_strategy === 'manual' && value.password.length < 8) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['password'],
        message: 'Password minimal 8 karakter.',
      });
    }
  });

interface ResetSuccess {
  password: string;
  strategy: 'manual' | 'generate';
}

function ResetPasswordDialog({
  user,
  open,
  onOpenChange,
}: {
  user: AdminUser;
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [success, setSuccess] = React.useState<ResetSuccess | null>(null);
  const form = useForm<z.infer<typeof resetPasswordSchema>>({
    resolver: zodResolver(resetPasswordSchema),
    defaultValues: { password_strategy: 'generate', password: '' },
  });
  const strategy = form.watch('password_strategy');

  React.useEffect(() => {
    if (open) {
      setSuccess(null);
      form.reset({ password_strategy: 'generate', password: '' });
    }
  }, [open, form]);

  const mutation = useMutation({
    mutationFn: (input: z.infer<typeof resetPasswordSchema>) =>
      api<ResetPasswordResponse>(`/admin/users/${user.id}/reset-password`, {
        method: 'POST',
        body: {
          password_strategy: input.password_strategy,
          password:
            input.password_strategy === 'manual' ? input.password : undefined,
        },
      }),
    onSuccess: (data, variables) => {
      const password =
        variables.password_strategy === 'generate'
          ? data.generated_password ?? ''
          : variables.password;
      setSuccess({ password, strategy: variables.password_strategy });
      qc.setQueryData<UserResponse>(['admin', 'users', user.id], { user: data.user });
      qc.invalidateQueries({ queryKey: ['admin', 'users'] });
    },
    onError: (err: unknown) => {
      const apiErr =
        err instanceof ApiError
          ? err
          : new ApiError({ status: 0, code: 'unknown', message: 'Gagal terhubung.' });
      toast({
        variant: 'destructive',
        title: 'Gagal reset password',
        description: withReqId(apiErr, friendlyError(apiErr)),
      });
    },
  });

  // Block close while form is open via outside click only when not pending. Default radix behavior fine.
  return (
    <Dialog open={open} onOpenChange={(v) => !mutation.isPending && onOpenChange(v)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Reset password</DialogTitle>
          <DialogDescription>
            Set ulang password awal untuk {user.email}. Pengguna akan diminta
            ganti password lagi saat login berikutnya.
          </DialogDescription>
        </DialogHeader>

        {success ? (
          <div className="space-y-4">
            <div className="rounded-md border border-emerald-500/50 bg-emerald-500/5 p-4">
              <Label className="text-xs uppercase tracking-wide text-muted-foreground">
                Password baru{' '}
                {success.strategy === 'generate' ? '(di-generate)' : '(manual)'}
              </Label>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <code className="flex-1 break-all rounded bg-muted px-3 py-2 font-mono text-base">
                  {success.password || '—'}
                </code>
                {success.password ? (
                  <CopyButton value={success.password} label="Salin" />
                ) : null}
              </div>
              <p className="mt-3 flex items-start gap-2 text-xs text-amber-700 dark:text-amber-400">
                <ShieldAlert className="mt-0.5 size-4 shrink-0" />
                Salin sekarang. Password ini tidak akan ditampilkan lagi.
              </p>
            </div>
            <DialogFooter>
              <Button onClick={() => onOpenChange(false)}>Selesai</Button>
            </DialogFooter>
          </div>
        ) : (
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit((v) => mutation.mutate(v))}
              className="space-y-4"
              noValidate
            >
              <FormField
                control={form.control}
                name="password_strategy"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Strategi</FormLabel>
                    <div className="grid gap-2 sm:grid-cols-2">
                      <label
                        className={cn(
                          'flex cursor-pointer items-start gap-3 rounded-md border p-3 text-sm transition-colors',
                          field.value === 'generate'
                            ? 'border-primary bg-primary/5'
                            : 'border-input',
                        )}
                      >
                        <input
                          type="radio"
                          className="mt-1"
                          checked={field.value === 'generate'}
                          onChange={() => field.onChange('generate')}
                          disabled={mutation.isPending}
                        />
                        <div>
                          <div className="font-medium">Generate</div>
                          <div className="text-xs text-muted-foreground">
                            Sistem buat password 16 karakter acak.
                          </div>
                        </div>
                      </label>
                      <label
                        className={cn(
                          'flex cursor-pointer items-start gap-3 rounded-md border p-3 text-sm transition-colors',
                          field.value === 'manual'
                            ? 'border-primary bg-primary/5'
                            : 'border-input',
                        )}
                      >
                        <input
                          type="radio"
                          className="mt-1"
                          checked={field.value === 'manual'}
                          onChange={() => field.onChange('manual')}
                          disabled={mutation.isPending}
                        />
                        <div>
                          <div className="font-medium">Manual</div>
                          <div className="text-xs text-muted-foreground">
                            Anda ketik sendiri password baru.
                          </div>
                        </div>
                      </label>
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {strategy === 'manual' ? (
                <FormField
                  control={form.control}
                  name="password"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Password baru</FormLabel>
                      <FormControl>
                        <Input
                          type="text"
                          autoComplete="new-password"
                          disabled={mutation.isPending}
                          placeholder="minimal 8 karakter"
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              ) : null}

              <DialogFooter>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => onOpenChange(false)}
                  disabled={mutation.isPending}
                >
                  Batal
                </Button>
                <Button type="submit" disabled={mutation.isPending}>
                  {mutation.isPending ? 'Memproses…' : 'Reset password'}
                </Button>
              </DialogFooter>
            </form>
          </Form>
        )}
      </DialogContent>
    </Dialog>
  );
}

// ---------- Suspend / unsuspend / unlock / revoke-sessions ----------

function ConfirmActionDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  cancelLabel = 'Batal',
  destructive = false,
  withReason = false,
  reasonLabel = 'Alasan (opsional)',
  reasonPlaceholder = '',
  onConfirm,
  pending,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  title: string;
  description: React.ReactNode;
  confirmLabel: string;
  cancelLabel?: string;
  destructive?: boolean;
  withReason?: boolean;
  reasonLabel?: string;
  reasonPlaceholder?: string;
  onConfirm: (reason?: string) => void;
  pending: boolean;
}) {
  const [reason, setReason] = React.useState('');
  React.useEffect(() => {
    if (open) setReason('');
  }, [open]);

  return (
    <Dialog open={open} onOpenChange={(v) => !pending && onOpenChange(v)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {withReason ? (
          <div className="space-y-1">
            <Label htmlFor="reason-input" className="text-sm">
              {reasonLabel}
            </Label>
            <Input
              id="reason-input"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={reasonPlaceholder}
              disabled={pending}
              maxLength={200}
            />
          </div>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="ghost"
            onClick={() => onOpenChange(false)}
            disabled={pending}
          >
            {cancelLabel}
          </Button>
          <Button
            type="button"
            variant={destructive ? 'destructive' : 'default'}
            onClick={() => onConfirm(withReason ? reason.trim() || undefined : undefined)}
            disabled={pending}
          >
            {pending ? 'Memproses…' : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---------- Page ----------

export default function AdminPenggunaDetailPage() {
  const params = useParams<{ id: string }>();
  const userId = params?.id ?? '';
  const qc = useQueryClient();
  const { toast } = useToast();

  const [tab, setTab] = React.useState<TabKey>('detail');
  const [editNameOpen, setEditNameOpen] = React.useState(false);
  const [changeRoleOpen, setChangeRoleOpen] = React.useState(false);
  const [resetPasswordOpen, setResetPasswordOpen] = React.useState(false);
  const [suspendOpen, setSuspendOpen] = React.useState(false);
  const [unsuspendOpen, setUnsuspendOpen] = React.useState(false);
  const [unlockOpen, setUnlockOpen] = React.useState(false);
  const [revokeOpen, setRevokeOpen] = React.useState(false);

  const userQuery = useQuery({
    queryKey: ['admin', 'users', userId],
    queryFn: () => api<UserResponse>(`/admin/users/${userId}`),
    enabled: Boolean(userId),
    staleTime: 15_000,
  });

  const onUserMutated = React.useCallback(
    (next: AdminUser) => {
      qc.setQueryData<UserResponse>(['admin', 'users', userId], { user: next });
      qc.invalidateQueries({ queryKey: ['admin', 'users'] });
      qc.invalidateQueries({ queryKey: ['admin', 'audit-log'] });
    },
    [qc, userId],
  );

  const suspendMutation = useMutation({
    mutationFn: (reason?: string) =>
      api<UserMutationResponse>(`/admin/users/${userId}/suspend`, {
        method: 'POST',
        body: { reason },
      }),
    onSuccess: (data) => {
      onUserMutated(data.user);
      toast({ title: 'Akun berhasil di-suspend' });
      setSuspendOpen(false);
    },
    onError: (err: unknown) => {
      const apiErr = err instanceof ApiError ? err : new ApiError({ status: 0, code: 'unknown', message: 'Gagal terhubung.' });
      toast({
        variant: 'destructive',
        title: 'Gagal suspend akun',
        description: withReqId(apiErr, friendlyError(apiErr)),
      });
    },
  });

  const unsuspendMutation = useMutation({
    mutationFn: () =>
      api<UserMutationResponse>(`/admin/users/${userId}/unsuspend`, {
        method: 'POST',
      }),
    onSuccess: (data) => {
      onUserMutated(data.user);
      toast({ title: 'Akun berhasil di-unsuspend' });
      setUnsuspendOpen(false);
    },
    onError: (err: unknown) => {
      const apiErr = err instanceof ApiError ? err : new ApiError({ status: 0, code: 'unknown', message: 'Gagal terhubung.' });
      toast({
        variant: 'destructive',
        title: 'Gagal unsuspend',
        description: withReqId(apiErr, friendlyError(apiErr)),
      });
    },
  });

  const unlockMutation = useMutation({
    mutationFn: () =>
      api<UserMutationResponse>(`/admin/users/${userId}/unlock`, {
        method: 'POST',
      }),
    onSuccess: (data) => {
      onUserMutated(data.user);
      toast({ title: 'Akun berhasil di-unlock' });
      setUnlockOpen(false);
    },
    onError: (err: unknown) => {
      const apiErr = err instanceof ApiError ? err : new ApiError({ status: 0, code: 'unknown', message: 'Gagal terhubung.' });
      toast({
        variant: 'destructive',
        title: 'Gagal unlock akun',
        description: withReqId(apiErr, friendlyError(apiErr)),
      });
    },
  });

  const revokeMutation = useMutation({
    mutationFn: (reason?: string) =>
      api<RevokeSessionsResponse>(`/admin/users/${userId}/revoke-sessions`, {
        method: 'POST',
        body: { reason },
      }),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['admin', 'users', userId, 'sessions'] });
      qc.invalidateQueries({ queryKey: ['admin', 'audit-log'] });
      toast({
        title: 'Logout berhasil',
        description: `${data.revoked_count} sesi dicabut.`,
      });
      setRevokeOpen(false);
    },
    onError: (err: unknown) => {
      const apiErr = err instanceof ApiError ? err : new ApiError({ status: 0, code: 'unknown', message: 'Gagal terhubung.' });
      toast({
        variant: 'destructive',
        title: 'Gagal logout-all',
        description: withReqId(apiErr, friendlyError(apiErr)),
      });
    },
  });

  if (userQuery.isPending) {
    return (
      <div className="space-y-3">
        <div className="h-5 w-40 animate-pulse rounded bg-muted" />
        <div className="h-8 w-72 animate-pulse rounded bg-muted" />
        <div className="h-72 animate-pulse rounded-md border bg-muted/30" />
      </div>
    );
  }

  if (userQuery.isError) {
    const err = userQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    const notFound = apiErr?.status === 404;
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {notFound ? 'Pengguna tidak ditemukan' : 'Gagal memuat pengguna'}
          </CardTitle>
          <CardDescription>
            {notFound
              ? 'Akun ini mungkin sudah dihapus.'
              : apiErr?.requestId
                ? `Coba lagi nanti (req: ${apiErr.requestId}).`
                : 'Coba lagi nanti.'}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline">
            <Link href="/admin/pengguna">
              <ArrowLeft className="size-4" />
              Kembali ke daftar
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  const user = userQuery.data!.user;

  return (
    <div className="space-y-6">
      <header className="space-y-3">
        <Link
          href="/admin/pengguna"
          className="inline-flex items-center gap-1 text-xs text-muted-foreground underline-offset-2 hover:underline"
        >
          <ArrowLeft className="size-3" />
          Kembali ke daftar pengguna
        </Link>

        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-1">
            <h1 className="text-2xl font-semibold tracking-tight">{user.name}</h1>
            <p className="break-all text-sm text-muted-foreground">{user.email}</p>
            <div className="flex flex-wrap items-center gap-2 pt-1">
              <Badge className={roleTone[user.role]}>{roleLabel[user.role]}</Badge>
              <Badge className={statusTone[user.status]}>
                {statusLabel[user.status]}
              </Badge>
              {user.must_change_password ? (
                <Badge className="bg-amber-500/15 text-amber-700 dark:text-amber-400">
                  Wajib ganti password
                </Badge>
              ) : null}
            </div>
          </div>

          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={() => setEditNameOpen(true)}>
              <Pencil className="size-4" />
              Edit nama
            </Button>
            <Button size="sm" variant="outline" onClick={() => setChangeRoleOpen(true)}>
              <UserCog className="size-4" />
              Ubah peran
            </Button>
            <Button size="sm" variant="outline" onClick={() => setResetPasswordOpen(true)}>
              <KeyRound className="size-4" />
              Reset password
            </Button>
            {user.status === 'active' ? (
              <Button size="sm" variant="outline" onClick={() => setSuspendOpen(true)}>
                <ShieldOff className="size-4" />
                Suspend
              </Button>
            ) : null}
            {user.status === 'suspended' ? (
              <Button size="sm" variant="outline" onClick={() => setUnsuspendOpen(true)}>
                <ShieldCheck className="size-4" />
                Unsuspend
              </Button>
            ) : null}
            {user.status === 'locked' ? (
              <Button size="sm" variant="outline" onClick={() => setUnlockOpen(true)}>
                <Unlock className="size-4" />
                Unlock
              </Button>
            ) : null}
            <Button
              size="sm"
              variant="destructive"
              onClick={() => setRevokeOpen(true)}
            >
              <Users className="size-4" />
              Logout dari semua perangkat
            </Button>
          </div>
        </div>
      </header>

      <TabBar active={tab} onChange={setTab} />

      {tab === 'detail' ? <DetailTab user={user} /> : null}
      {tab === 'sessions' ? <SessionsTab userId={user.id} /> : null}
      {tab === 'audit' ? <AuditTab userId={user.id} /> : null}
      {tab === 'attempts' ? <LoginAttemptsTab email={user.email} /> : null}

      <EditNameDialog user={user} open={editNameOpen} onOpenChange={setEditNameOpen} />
      <ChangeRoleDialog
        user={user}
        open={changeRoleOpen}
        onOpenChange={setChangeRoleOpen}
      />
      <ResetPasswordDialog
        user={user}
        open={resetPasswordOpen}
        onOpenChange={setResetPasswordOpen}
      />

      <ConfirmActionDialog
        open={suspendOpen}
        onOpenChange={setSuspendOpen}
        title="Suspend akun"
        description={
          <>
            Akun {user.email} tidak akan bisa login sampai di-unsuspend.
            Semua sesi aktif tetap berlaku — gunakan tombol logout-all untuk
            memutus juga.
          </>
        }
        confirmLabel="Suspend"
        destructive
        withReason
        reasonLabel="Alasan suspend (opsional)"
        reasonPlaceholder="mis. pelanggaran kebijakan"
        pending={suspendMutation.isPending}
        onConfirm={(reason) => suspendMutation.mutate(reason)}
      />

      <ConfirmActionDialog
        open={unsuspendOpen}
        onOpenChange={setUnsuspendOpen}
        title="Unsuspend akun"
        description={<>Akun {user.email} akan kembali bisa login.</>}
        confirmLabel="Unsuspend"
        pending={unsuspendMutation.isPending}
        onConfirm={() => unsuspendMutation.mutate()}
      />

      <ConfirmActionDialog
        open={unlockOpen}
        onOpenChange={setUnlockOpen}
        title="Unlock akun"
        description={
          <>
            Akun {user.email} terkunci karena terlalu banyak gagal login.
            Unlock akan reset hitungan dan mengizinkan login lagi.
          </>
        }
        confirmLabel="Unlock"
        pending={unlockMutation.isPending}
        onConfirm={() => unlockMutation.mutate()}
      />

      <ConfirmActionDialog
        open={revokeOpen}
        onOpenChange={setRevokeOpen}
        title="Logout dari semua perangkat"
        description={
          <>
            Semua refresh token milik {user.email} akan dicabut. Pengguna harus
            login ulang di setiap perangkat.
          </>
        }
        confirmLabel="Cabut semua sesi"
        destructive
        withReason
        reasonLabel="Alasan (opsional)"
        reasonPlaceholder="mis. perangkat hilang"
        pending={revokeMutation.isPending}
        onConfirm={(reason) => revokeMutation.mutate(reason)}
      />
    </div>
  );
}
