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
  Library,
  LogOut,
  Menu,
  PanelLeftClose,
  ShieldAlert,
  UserCog,
  UserPlus,
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
  badgeKey?: 'ungraded' | 'review_ulangan' | 'review_ujian' | 'pending_total';
}

const NAV: NavItem[] = [
  { href: '/guru', label: 'Dashboard', Icon: LayoutDashboard, badgeKey: 'pending_total' },
  { href: '/guru/kelas', label: 'Kelas', Icon: GraduationCap },
  { href: '/guru/siswa-join-requests', label: 'Request Siswa', Icon: UserPlus },
  { href: '/guru/bank-soal', label: 'Bank Soal', Icon: Library },
];

function isActive(pathname: string, href: string): boolean {
  if (href === '/guru') return pathname === '/guru';
  return pathname === href || pathname.startsWith(`${href}/`);
}

const SIDEBAR_MIN = 64;
const SIDEBAR_MAX = 320;
const SIDEBAR_DEFAULT = 240;

function clampSidebarWidth(width: number) {
  return Math.min(SIDEBAR_MAX, Math.max(SIDEBAR_MIN, width));
}

function GuruShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { toast } = useToast();
  const user = useAuthStore((s) => s.user);
  const [sidebarOpen, setSidebarOpen] = React.useState(false);
  const [sidebarWidth, setSidebarWidth] = React.useState(SIDEBAR_DEFAULT);
  const [expandedWidth, setExpandedWidth] = React.useState(SIDEBAR_DEFAULT);
  const refresh = useAuthStore((s) => s.refresh);
  const clear = useAuthStore((s) => s.clear);
  const sidebarCollapsed = sidebarWidth <= SIDEBAR_MIN + 8;

  React.useEffect(() => {
    const saved = window.localStorage.getItem('lms:guru-sidebar-width');
    if (!saved) return;
    const parsed = Number(saved);
    if (Number.isFinite(parsed)) {
      const width = clampSidebarWidth(parsed);
      setSidebarWidth(width);
      if (width > SIDEBAR_MIN + 8) setExpandedWidth(width);
    }
  }, []);

  React.useEffect(() => {
    window.localStorage.setItem('lms:guru-sidebar-width', String(sidebarWidth));
  }, [sidebarWidth]);

  React.useEffect(() => {
    if (!sidebarOpen) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = previous;
    };
  }, [sidebarOpen]);

  const startResize = React.useCallback((event: React.PointerEvent<HTMLButtonElement>) => {
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = sidebarWidth;

    const onMove = (moveEvent: PointerEvent) => {
      const next = clampSidebarWidth(startWidth + moveEvent.clientX - startX);
      setSidebarWidth(next);
      if (next > SIDEBAR_MIN + 8) setExpandedWidth(next);
    };
    const onUp = () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
  }, [sidebarWidth]);

  const toggleSidebar = () => {
    setSidebarWidth((width) => (width <= SIDEBAR_MIN + 8 ? expandedWidth : SIDEBAR_MIN));
  };

  // Pending counters — polled every 30s while guru navigates (Task 4.E.2 +
  // Task 7.D consolidated locked #93 — 3 counters cross-kelas).
  const pendingQ = useQuery({
    queryKey: ['guru', 'pending-counts'],
    queryFn: getPendingCounts,
    staleTime: 15_000,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  });
  const ungraded = pendingQ.data?.ungraded_submissions ?? 0;
  const reviewUlangan = pendingQ.data?.pending_review_ulangan ?? 0;
  const reviewUjian = pendingQ.data?.pending_review_ujian ?? 0;
  const pendingTotal = ungraded + reviewUlangan + reviewUjian;

  const badgeFor = (key?: NavItem['badgeKey']): number | undefined => {
    if (!key) return undefined;
    if (key === 'ungraded') return ungraded > 0 ? ungraded : undefined;
    if (key === 'review_ulangan') return reviewUlangan > 0 ? reviewUlangan : undefined;
    if (key === 'review_ujian') return reviewUjian > 0 ? reviewUjian : undefined;
    if (key === 'pending_total') return pendingTotal > 0 ? pendingTotal : undefined;
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
    <div className="h-svh overflow-hidden bg-muted/30 md:h-screen">
      <div className="flex h-svh w-full overflow-hidden md:h-screen">
        {/* Sidebar */}
        <aside
          className="sticky top-0 hidden h-screen shrink-0 border-r bg-background transition-[width] md:flex md:flex-col relative"
          style={{ width: sidebarWidth }}
        >
          <div className="flex h-14 items-center border-b px-4">
            <Link href="/guru" className={cn('text-sm font-semibold tracking-tight', sidebarCollapsed && 'sr-only')}>
              LMS Guru
            </Link>
          </div>
          <nav className="flex-1 space-y-1 overflow-y-auto p-2">
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
                  <span className={cn('flex-1', sidebarCollapsed && 'sr-only')}>{label}</span>
                  {badge !== undefined && (
                    <span className="ml-auto inline-flex min-w-5 items-center justify-center rounded-full bg-rose-500/15 px-1.5 py-0.5 text-[10px] font-semibold text-rose-700 dark:text-rose-300">
                      {badge}
                    </span>
                  )}
                </Link>
              );
            })}
          </nav>
          <div className="border-t p-2">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="w-full justify-center"
              aria-label={sidebarCollapsed ? 'Lebarkan sidebar' : 'Ciutkan sidebar'}
              onClick={toggleSidebar}
            >
              <PanelLeftClose className={cn('size-4 transition-transform', sidebarCollapsed && 'rotate-180')} />
              <span className={cn('ml-2', sidebarCollapsed && 'sr-only')}>
                {sidebarCollapsed ? 'Lebarkan' : 'Ciutkan'}
              </span>
            </Button>
          </div>
          <div className={cn("border-t p-2 text-xs text-muted-foreground", sidebarCollapsed && "hidden")}>
            <div className="px-2 py-1">v0.7.2 · Fase 2</div>
          </div>
          <button
            type="button"
            aria-label="Resize sidebar"
            className="absolute -right-2 top-20 hidden h-14 w-4 cursor-col-resize touch-none rounded-full border bg-background shadow-sm transition-colors hover:border-primary/50 hover:bg-primary/10 md:block"
            onPointerDown={startResize}
          />
        </aside>

        <div className="flex h-svh min-w-0 flex-1 flex-col overflow-x-hidden overflow-y-auto md:h-screen">
          <header className="sticky top-0 z-20 flex h-14 shrink-0 items-center justify-between gap-2 border-b bg-background/80 px-3 backdrop-blur sm:px-4 md:px-6">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="md:hidden"
              aria-label="Buka menu navigasi"
              aria-expanded={sidebarOpen}
              onClick={() => setSidebarOpen(true)}
            >
              <Menu className="size-5" />
            </Button>
            {sidebarOpen && (
              <div className="fixed inset-0 z-50 md:hidden">
                <button
                  type="button"
                  className="absolute inset-0 bg-background/80 backdrop-blur-sm"
                  aria-label="Tutup menu navigasi"
                  onClick={() => setSidebarOpen(false)}
                />
                <aside className="absolute left-0 top-0 flex h-dvh w-72 max-w-[88vw] flex-col border-r bg-background shadow-xl">
                  <div className="flex h-14 items-center justify-between border-b px-4">
                    <span className="text-sm font-semibold tracking-tight">Menu</span>
                    <Button type="button" variant="ghost" size="sm" onClick={() => setSidebarOpen(false)}>
                      Tutup
                    </Button>
                  </div>
                  <nav className="flex-1 space-y-1 overflow-y-auto p-2">
                    {NAV.map(({ href, label, Icon }) => {
                      const active = isActive(pathname, href);
                      return (
                        <Link
                          key={href}
                          href={href}
                          onClick={() => setSidebarOpen(false)}
                          className={cn(
                            'flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
                            active
                              ? 'bg-accent text-accent-foreground font-medium'
                              : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
                          )}
                        >
                          <Icon className="size-4" />
                          <span>{label}</span>
                        </Link>
                      );
                    })}
                  </nav>
                </aside>
              </div>
            )}
            <div className="hidden text-sm text-muted-foreground md:block">
              Panel Guru
            </div>

            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="sm" className="min-w-0 gap-2">
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

          <main className="min-w-0 flex-1 p-3 sm:p-4 md:p-8">
            <div className="mx-auto w-full min-w-0 max-w-screen-2xl">{children}</div>
          </main>
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
