'use client';

/**
 * /me/perangkat — active sessions + logout-all (Fase 1.G.4).
 *
 * Locked decisions referenced:
 *   - #42 Refresh tokens are tracked server-side; logout-all revokes every
 *         active jti for the user. Per-jti revoke endpoint is admin-only at
 *         the moment, so single-row revoke is deferred to v0.8.
 *
 * The current jti is recovered by decoding the (unverified) JWT payload of
 * the refresh token kept in Zustand. Decoding here is purely a UX hint
 * (badge "Sesi ini") and never used as a trust boundary.
 */

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useQuery, useMutation } from '@tanstack/react-query';

import { api, ApiError } from '@/lib/api';
import { useAuthStore } from '@/lib/auth';
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

function decodeRefreshJti(refresh: string | null): string | null {
  if (!refresh) return null;
  const parts = refresh.split('.');
  const payloadPart = parts[1];
  if (!payloadPart) return null;
  try {
    // base64url -> base64
    const b64 = payloadPart.replace(/-/g, '+').replace(/_/g, '/');
    const padded = b64 + '='.repeat((4 - (b64.length % 4)) % 4);
    const json = atob(padded);
    const payload = JSON.parse(json) as { jti?: unknown };
    return typeof payload.jti === 'string' ? payload.jti : null;
  } catch {
    return null;
  }
}

function maskJti(jti: string): string {
  if (jti.length <= 8) return jti;
  return `${jti.slice(0, 4)}…${jti.slice(-4)}`;
}

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
  const trimmed = ua.trim();
  if (!trimmed) return 'Perangkat tidak dikenal';

  const lower = trimmed.toLowerCase();
  // Crude heuristics — cukup untuk UX hint, bukan deteksi presisi.
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

export default function MePerangkatPage() {
  const router = useRouter();
  const { toast } = useToast();
  const refresh = useAuthStore((s) => s.refresh);
  const clear = useAuthStore((s) => s.clear);

  const currentJti = React.useMemo(() => decodeRefreshJti(refresh), [refresh]);

  const sessionsQuery = useQuery({
    queryKey: ['auth', 'sessions'],
    queryFn: () => api<SessionsResponse>('/auth/sessions'),
    staleTime: 30_000,
  });

  const logoutAllMutation = useMutation({
    mutationFn: () =>
      api('/auth/logout-all', {
        method: 'POST',
      }),
    onSuccess: () => {
      toast({
        title: 'Berhasil logout dari semua perangkat',
        description: 'Silakan login ulang untuk melanjutkan.',
      });
      clear();
      router.replace('/login');
    },
    onError: (err: unknown) => {
      const apiErr =
        err instanceof ApiError
          ? err
          : new ApiError({
              status: 0,
              code: 'unknown',
              message: 'Tidak dapat terhubung ke server.',
            });
      toast({
        variant: 'destructive',
        title: 'Gagal logout-all',
        description: apiErr.requestId
          ? `${apiErr.message} (req: ${apiErr.requestId})`
          : apiErr.message,
      });
    },
  });

  const sessions = sessionsQuery.data?.sessions ?? [];
  const totalActive = sessions.length;

  return (
    <main className="container max-w-2xl py-12 space-y-6">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Perangkat aktif</h1>
        <p className="text-sm text-muted-foreground">
          Daftar sesi refresh token yang masih hidup. Logout dari semua
          perangkat akan mengakhiri seluruh sesi termasuk perangkat ini.
        </p>
      </header>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4">
          <div className="space-y-1">
            <CardTitle>Sesi aktif</CardTitle>
            <CardDescription>
              {sessionsQuery.isPending
                ? 'Memuat…'
                : sessionsQuery.isError
                  ? 'Gagal memuat sesi.'
                  : `${totalActive} sesi aktif.`}
            </CardDescription>
          </div>
          <Button
            variant="destructive"
            size="sm"
            disabled={
              logoutAllMutation.isPending ||
              sessionsQuery.isPending ||
              totalActive === 0
            }
            onClick={() => logoutAllMutation.mutate()}
          >
            {logoutAllMutation.isPending ? 'Memproses…' : 'Logout dari semua perangkat'}
          </Button>
        </CardHeader>
        <CardContent className="space-y-3">
          {sessionsQuery.isError ? (
            <p className="text-sm text-destructive">
              {sessionsQuery.error instanceof ApiError && sessionsQuery.error.requestId
                ? `Tidak bisa memuat daftar sesi (req: ${sessionsQuery.error.requestId}).`
                : 'Tidak bisa memuat daftar sesi.'}
            </p>
          ) : sessionsQuery.isPending ? (
            <p className="text-sm text-muted-foreground">Memuat…</p>
          ) : sessions.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Tidak ada sesi aktif tercatat.
            </p>
          ) : (
            <ul className="divide-y divide-border rounded-md border">
              {sessions.map((s) => {
                const isCurrent = currentJti != null && s.jti === currentJti;
                return (
                  <li
                    key={s.id}
                    className="flex flex-col gap-2 p-4 sm:flex-row sm:items-center sm:justify-between"
                  >
                    <div className="space-y-1 text-sm">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium">
                          {summarizeUserAgent(s.user_agent)}
                        </span>
                        {isCurrent ? (
                          <span className="rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
                            Sesi ini
                          </span>
                        ) : null}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        IP {s.ip ?? '—'} • JTI {maskJti(s.jti)}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        Mulai {formatDate(s.issued_at)} • Berakhir{' '}
                        {formatDate(s.expires_at)}
                      </div>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </CardContent>
        <CardFooter className="text-xs text-muted-foreground">
          Per-perangkat revoke akan ditambahkan di v0.8. Untuk sementara, gunakan
          tombol logout-all di atas.
        </CardFooter>
      </Card>

      <div className="text-sm">
        <Link
          href="/me"
          className="text-muted-foreground underline-offset-2 hover:underline"
        >
          ← Kembali ke profil
        </Link>
      </div>
    </main>
  );
}
