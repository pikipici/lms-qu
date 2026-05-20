'use client';

/**
 * /me — profile page (Fase 1.G.3).
 *
 * Read-only view of the authenticated user. The bearer is refreshed via
 * the api() interceptor; this page only needs to call /auth/me and render.
 *
 * Locked decisions referenced:
 *   - #42 Logout revokes the current refresh token; access token expires on
 *         its own (15m, stateless).
 */

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useQuery, useMutation } from '@tanstack/react-query';

import { api, ApiError } from '@/lib/api';
import { useAuthStore, type Role } from '@/lib/auth';
import { useToast } from '@/hooks/use-toast';
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
        // Best-effort revoke; 204 on success, 204 even on bad token.
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

  return (
    <main className="container max-w-xl py-12 space-y-6">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Profil</h1>
        <p className="text-sm text-muted-foreground">
          Informasi akun Anda. Akun dikelola oleh admin sekolah.
        </p>
      </header>

      {mustChange ? (
        <Card className="border-amber-500/50 bg-amber-500/10">
          <CardHeader>
            <CardTitle className="text-base">Wajib ganti password</CardTitle>
            <CardDescription>
              Anda harus mengganti password sementara sebelum dapat menggunakan
              fitur lainnya.
            </CardDescription>
          </CardHeader>
          <CardFooter>
            <Button asChild size="sm">
              <Link href="/me/security">Ganti password sekarang</Link>
            </Button>
          </CardFooter>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>Detail akun</CardTitle>
          <CardDescription>Read-only — hubungi admin untuk perubahan.</CardDescription>
        </CardHeader>
        <CardContent>
          {meQuery.isPending ? (
            <p className="text-sm text-muted-foreground">Memuat…</p>
          ) : meQuery.isError ? (
            <p className="text-sm text-destructive">
              Gagal memuat profil
              {meQuery.error instanceof ApiError && meQuery.error.requestId
                ? ` (req: ${meQuery.error.requestId})`
                : null}
              .
            </p>
          ) : user ? (
            <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-[140px_1fr]">
              <dt className="text-muted-foreground">Nama</dt>
              <dd>{user.name}</dd>
              <dt className="text-muted-foreground">Email</dt>
              <dd className="break-all">{user.email}</dd>
              <dt className="text-muted-foreground">Role</dt>
              <dd>{roleLabel[user.role]}</dd>
              <dt className="text-muted-foreground">Status</dt>
              <dd>{statusLabel[user.status]}</dd>
              <dt className="text-muted-foreground">Login terakhir</dt>
              <dd>{formatDate(user.last_login_at)}</dd>
              <dt className="text-muted-foreground">Akun dibuat</dt>
              <dd>{formatDate(user.created_at)}</dd>
            </dl>
          ) : null}
        </CardContent>
        <CardFooter className="flex flex-wrap gap-2">
          <Button asChild variant="outline" size="sm">
            <Link href="/me/security">Ganti password</Link>
          </Button>
          <Button asChild variant="outline" size="sm">
            <Link href="/me/perangkat">Perangkat aktif</Link>
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={mustChange || logoutMutation.isPending}
            onClick={() => {
              if (mustChange) {
                toast({
                  title: 'Tidak bisa logout',
                  description:
                    'Selesaikan ganti password dulu sebelum keluar.',
                });
                return;
              }
              logoutMutation.mutate();
            }}
          >
            {logoutMutation.isPending ? 'Memproses…' : 'Logout'}
          </Button>
        </CardFooter>
      </Card>
    </main>
  );
}
