'use client';

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useQuery, useMutation } from '@tanstack/react-query';
import {
  ArrowRight,
  KeyRound,
  Laptop,
  LogOut,
  ShieldCheck,
  Sparkles,
  UserRound,
} from 'lucide-react';

import { api, ApiError } from '@/lib/api';
import { useAuthStore, type Role } from '@/lib/auth';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

interface MeResponse {
  user: {
    id: string;
    name: string;
    email: string;
    role: Role;
    status: 'active' | 'suspended' | 'locked';
    must_change_password: boolean;
    last_login_at?: string | null;
    created_at: string;
    updated_at: string;
  };
}

const roleLabel: Record<Role, string> = {
  admin: 'Administrator',
  guru: 'Guru',
  siswa: 'Siswa',
};

const statusLabel: Record<'active' | 'suspended' | 'locked', string> = {
  active: 'Aktif',
  suspended: 'Suspended',
  locked: 'Terkunci',
};

const statusTone: Record<'active' | 'suspended' | 'locked', string> = {
  active: 'border-emerald-300 bg-emerald-50 text-emerald-700',
  suspended: 'border-amber-300 bg-amber-50 text-amber-700',
  locked: 'border-red-300 bg-red-50 text-red-700',
};

const roleHome: Record<Role, string> = {
  admin: '/admin',
  guru: '/guru',
  siswa: '/siswa',
};

function formatDate(input?: string | null): string {
  if (!input) return '-';
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

function initials(name?: string | null): string {
  return (name || 'Akun')
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? '')
    .join('') || 'AK';
}

function AccountBadge({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <span className={cn('inline-flex items-center rounded-full border px-2.5 py-1 text-xs font-semibold', className)}>
      {children}
    </span>
  );
}

export default function MePage() {
  const router = useRouter();
  const { toast } = useToast();
  const refresh = useAuthStore((s) => s.refresh);
  const clear = useAuthStore((s) => s.clear);
  const storeUser = useAuthStore((s) => s.user);

  const meQuery = useQuery({
    queryKey: ['auth', 'me'],
    queryFn: () => api<MeResponse>('/auth/me'),
    staleTime: 60_000,
  });

  const logoutMutation = useMutation({
    mutationFn: async () => {
      if (refresh) {
        await api('/auth/logout', {
          method: 'POST',
          body: { refresh_token: refresh },
          anon: true,
        }).catch(() => undefined);
      }
    },
    onSettled: () => {
      clear();
      router.replace('/login');
    },
  });

  const user = meQuery.data?.user ?? null;
  const mustChange = user?.must_change_password ?? storeUser?.mustChangePassword ?? false;
  const displayName = user?.name ?? storeUser?.name ?? 'Akun saya';
  const displayEmail = user?.email ?? storeUser?.email ?? 'Memuat email...';
  const displayRole = user?.role ?? storeUser?.role ?? 'siswa';

  return (
    <main className="min-h-screen bg-[radial-gradient(circle_at_top_left,_#dbeafe,_transparent_28rem),linear-gradient(135deg,_#fff7ed_0%,_#f8fafc_45%,_#ecfeff_100%)] px-4 py-8 sm:px-6 lg:px-8">
      <div className="mx-auto max-w-5xl space-y-6">
        <section className="overflow-hidden rounded-3xl border bg-white/85 shadow-xl shadow-slate-200/70 backdrop-blur">
          <div className="relative p-6 sm:p-8">
            <div className="absolute right-6 top-6 hidden rounded-full border bg-white/80 px-3 py-1 text-xs font-semibold text-slate-500 sm:flex">
              <Sparkles className="mr-1.5 size-3.5 text-amber-500" />
              Account Center
            </div>
            <div className="flex flex-col gap-5 sm:flex-row sm:items-center">
              <div className="grid size-20 shrink-0 place-items-center rounded-3xl border-2 border-slate-900 bg-amber-300 text-2xl font-black text-slate-950 shadow-[6px_6px_0_#0f172a]">
                {initials(displayName)}
              </div>
              <div className="min-w-0 flex-1 space-y-3">
                <div>
                  <p className="text-xs font-bold uppercase tracking-[0.22em] text-slate-500">Profil akun</p>
                  <h1 className="mt-1 truncate text-3xl font-black tracking-tight text-slate-950 sm:text-4xl">
                    {displayName}
                  </h1>
                  <p className="break-all text-sm font-medium text-slate-600">{displayEmail}</p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <AccountBadge className="border-sky-300 bg-sky-50 text-sky-700">
                    <UserRound className="mr-1.5 size-3.5" />
                    {roleLabel[displayRole]}
                  </AccountBadge>
                  {user ? (
                    <AccountBadge className={statusTone[user.status]}>
                      <ShieldCheck className="mr-1.5 size-3.5" />
                      {statusLabel[user.status]}
                    </AccountBadge>
                  ) : null}
                </div>
              </div>
            </div>
          </div>
        </section>

        {mustChange ? (
          <Card className="border-amber-300 bg-amber-50 shadow-sm">
            <CardHeader className="sm:flex-row sm:items-center sm:justify-between">
              <div>
                <CardTitle className="text-base">Wajib ganti password</CardTitle>
                <CardDescription>
                  Akun masih memakai password sementara. Ganti password dulu agar semua fitur bisa dipakai.
                </CardDescription>
              </div>
              <Button asChild size="sm" className="mt-3 sm:mt-0">
                <Link href="/me/security">Ganti sekarang</Link>
              </Button>
            </CardHeader>
          </Card>
        ) : null}

        <div className="grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
          <Card className="border-slate-200 bg-white/90 shadow-sm">
            <CardHeader>
              <CardTitle>Ringkasan akun</CardTitle>
              <CardDescription>Informasi dasar akun yang dikelola oleh admin sekolah.</CardDescription>
            </CardHeader>
            <CardContent>
              {meQuery.isPending ? (
                <div className="space-y-3">
                  {[0, 1, 2].map((i) => <div key={i} className="h-10 animate-pulse rounded-xl bg-slate-100" />)}
                </div>
              ) : meQuery.isError ? (
                <p className="text-sm text-destructive">
                  Gagal memuat profil{meQuery.error instanceof ApiError && meQuery.error.requestId ? ` (req: ${meQuery.error.requestId})` : ''}.
                </p>
              ) : user ? (
                <div className="grid gap-3 sm:grid-cols-2">
                  <InfoTile label="Nama" value={user.name} />
                  <InfoTile label="Email" value={user.email} breakAll />
                  <InfoTile label="Login terakhir" value={formatDate(user.last_login_at)} />
                  <InfoTile label="Akun dibuat" value={formatDate(user.created_at)} />
                </div>
              ) : null}
            </CardContent>
          </Card>

          <Card className="border-slate-200 bg-white/90 shadow-sm">
            <CardHeader>
              <CardTitle>Aksi cepat</CardTitle>
              <CardDescription>Kelola keamanan akun dan sesi login.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-2">
              <QuickLink href="/me/security" icon={KeyRound} label="Keamanan akun" description="Ganti password dan amankan akses." />
              <QuickLink href="/me/perangkat" icon={Laptop} label="Perangkat aktif" description="Cek sesi login yang masih aktif." />
              <QuickLink href={roleHome[displayRole]} icon={ArrowRight} label="Kembali ke dashboard" description={`Buka panel ${roleLabel[displayRole]}.`} />
            </CardContent>
            <CardFooter>
              <Button
                variant="outline"
                className="w-full justify-center border-red-200 text-red-700 hover:bg-red-50 hover:text-red-800"
                disabled={mustChange || logoutMutation.isPending}
                onClick={() => {
                  if (mustChange) {
                    toast({ title: 'Tidak bisa logout', description: 'Selesaikan ganti password dulu sebelum keluar.' });
                    return;
                  }
                  logoutMutation.mutate();
                }}
              >
                <LogOut className="mr-2 size-4" />
                {logoutMutation.isPending ? 'Memproses...' : 'Logout dari akun ini'}
              </Button>
            </CardFooter>
          </Card>
        </div>
      </div>
    </main>
  );
}

function InfoTile({ label, value, breakAll = false }: { label: string; value: string; breakAll?: boolean }) {
  return (
    <div className="rounded-2xl border bg-slate-50/80 p-4">
      <p className="text-xs font-bold uppercase tracking-wide text-slate-500">{label}</p>
      <p className={cn('mt-1 text-sm font-semibold text-slate-950', breakAll && 'break-all')}>{value}</p>
    </div>
  );
}

function QuickLink({ href, icon: Icon, label, description }: { href: string; icon: React.ElementType; label: string; description: string }) {
  return (
    <Link href={href} className="group flex items-center gap-3 rounded-2xl border bg-slate-50/80 p-3 transition hover:-translate-y-0.5 hover:bg-white hover:shadow-md">
      <span className="grid size-10 place-items-center rounded-xl bg-slate-950 text-white">
        <Icon className="size-4" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-bold text-slate-950">{label}</span>
        <span className="block text-xs text-slate-500">{description}</span>
      </span>
      <ArrowRight className="size-4 text-slate-400 transition group-hover:translate-x-0.5" />
    </Link>
  );
}
