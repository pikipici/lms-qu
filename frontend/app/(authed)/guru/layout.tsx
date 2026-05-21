'use client';

/**
 * Guru shell — sidebar + header for /guru/* pages.
 *
 * Mirror of /admin/layout.tsx. Parent (authed) layout enforces login +
 * force-change-password gate; this file adds:
 *   - Role guard (guru only).
 *   - Persistent sidebar nav.
 *   - Pending submissions badge (Task 4.E.2 — polled 30s).
 *   - Header strip with user dropdown (Profil + Logout).
 */

import * as React from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import {
  LayoutDashboard,
  GraduationCap,
  LogOut,
  ShieldAlert,
  UserCog,
} from 'lucide-react';

import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/auth';
import { getPendingCounts } from '@/lib/guru-api';
import { RoleGuard } from '@/lib/role-guard';
import { cn } from '@/lib/utils';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';

interface NavItem {
  href: string;
  label: string;
  Icon: React.ComponentType<{ className?: string }>;
  /** When set, shows a badge next to the label with the resolved value. */
  badgeKey?: 'ungraded';
}

const NAV: NavItem[] = [
  { href: '/guru', label: 'Dashboard', Icon: LayoutDashboard, badgeKey: 'ungraded' },
  { href: '/guru/kelas', label: 'Kelas', Icon: GraduationCap },
];

function isActive(pathname: string, href: string): boolean {
  if (href === '/guru') return pathname === '/guru';
  return pathname === href || pathname.startsWith(`${href}/`);
}

function GuruShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { toast } = useToast();
  const user = useAuthStore((s) => s.user);
  const refresh = useAuthStore((s) => s.refresh);
  const clear = useAuthStore((s) => s.clear);

  // Pending counters — polled every 30s while guru navigates (Task 4.E.2).
  const pendingQ = useQuery({
    queryKey: ['guru', 'pending-counts'],
    queryFn: getPendingCounts,
    staleTime: 15_000,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  });
  const ungraded = pendingQ.data?.ungraded_submissions ?? 0;

  const badgeFor = (key?: NavItem['badgeKey']): number | undefined => {
    if (!key) return undefined;
    if (key === 'ungraded') return ungraded > 0 ? ungraded : undefined;
    return undefined;
  };

  const onLogout = async () => {
    try {
      if (refresh) {
        await api('/auth/logout', {
          method: 'POST',
          body: { refresh_token: refresh },
          anon: true,
        }).catch(() => undefined);
      }
    } finally {
      clear();
      toast({ title: 'Berhasil logout' });
      router.replace('/login');
    }
  };

  const initials =
    (user?.name ?? '??')
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((s) => s[0]?.toUpperCase() ?? '')
      .join('') || '??';

  return (
    <div className="min-h-screen bg-muted/30">
      <div className="mx-auto flex min-h-screen max-w-[1400px]">
        {/* Sidebar */}
        <aside className="hidden w-60 shrink-0 border-r bg-background md:flex md:flex-col">
          <div className="flex h-14 items-center border-b px-4">
            <Link href="/guru" className="text-sm font-semibold tracking-tight">
              LMS Guru
            </Link>
          </div>
          <nav className="flex-1 space-y-1 p-2">
            {NAV.map(({ href, label, Icon, badgeKey }) => {
              const active = isActive(pathname, href);
              const badge = badgeFor(badgeKey);
              return (
                <Link
                  key={href}
                  href={href}
                  className={cn(
                    'flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors',
                    active
                      ? 'bg-accent text-accent-foreground font-medium'
                      : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
                  )}
                >
                  <Icon className="size-4" />
                  <span className="flex-1">{label}</span>
                  {badge !== undefined && (
                    <span className="ml-auto inline-flex min-w-5 items-center justify-center rounded-full bg-rose-500/15 px-1.5 py-0.5 text-[10px] font-semibold text-rose-700 dark:text-rose-300">
                      {badge}
                    </span>
                  )}
                </Link>
              );
            })}
          </nav>
          <div className="border-t p-2 text-xs text-muted-foreground">
            <div className="px-2 py-1">v0.7.2 · Fase 2</div>
          </div>
        </aside>

        {/* Main column */}
        <div className="flex min-h-screen flex-1 flex-col">
          <header className="sticky top-0 z-20 flex h-14 items-center justify-between border-b bg-background/80 px-4 backdrop-blur md:px-6">
            {/* Mobile nav (compact) */}
            <nav className="flex gap-1 overflow-x-auto md:hidden">
              {NAV.map(({ href, label, Icon, badgeKey }) => {
                const active = isActive(pathname, href);
                const badge = badgeFor(badgeKey);
                return (
                  <Link
                    key={href}
                    href={href}
                    className={cn(
                      'flex items-center gap-1 rounded-md px-2 py-1 text-xs',
                      active
                        ? 'bg-accent text-accent-foreground'
                        : 'text-muted-foreground',
                    )}
                  >
                    <Icon className="size-3.5" />
                    {label}
                    {badge !== undefined && (
                      <span className="inline-flex min-w-4 items-center justify-center rounded-full bg-rose-500/15 px-1 text-[10px] font-semibold text-rose-700 dark:text-rose-300">
                        {badge}
                      </span>
                    )}
                  </Link>
                );
              })}
            </nav>
            <div className="hidden text-sm text-muted-foreground md:block">
              Panel Guru
            </div>

            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="sm" className="gap-2">
                  <span className="grid size-7 place-items-center rounded-full bg-primary/10 text-xs font-semibold text-primary">
                    {initials}
                  </span>
                  <span className="hidden text-sm sm:inline">
                    {user?.name ?? 'Guru'}
                  </span>
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-56">
                <DropdownMenuLabel className="font-normal">
                  <div className="flex flex-col space-y-1">
                    <span className="text-sm font-medium">{user?.name}</span>
                    <span className="text-xs text-muted-foreground break-all">
                      {user?.email}
                    </span>
                  </div>
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild>
                  <Link href="/me">
                    <UserCog className="mr-2 size-4" />
                    Profil
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link href="/me/perangkat">
                    <ShieldAlert className="mr-2 size-4" />
                    Perangkat aktif
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem onSelect={onLogout} className="text-destructive">
                  <LogOut className="mr-2 size-4" />
                  Logout
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </header>

          <main className="flex-1 p-4 md:p-8">{children}</main>
        </div>
      </div>
    </div>
  );
}

export default function GuruLayout({ children }: { children: React.ReactNode }) {
  return (
    <RoleGuard allow="guru">
      <GuruShell>{children}</GuruShell>
    </RoleGuard>
  );
}
