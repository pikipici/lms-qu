'use client';

/**
 * Siswa shell — sidebar + header for /siswa/* pages.
 *
 * Mirror of /guru/layout.tsx: parent (authed) layout enforces login + force-
 * change-password gate; this file adds:
 *   - Role guard (siswa only).
 *   - Persistent sidebar nav.
 *   - Header strip with user dropdown (Profil + Logout).
 *
 * Phase 2.C scope — only Dashboard + Gabung Kelas. Tugas/Materi/Pengumuman
 * come in later fases.
 */

import * as React from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import {
  LayoutDashboard,
  KeyRound,
  ClipboardList,
  GraduationCap,
  LogOut,
  Menu,
  PanelLeftClose,
  ShieldAlert,
  TrendingUp,
  UserCog,
} from 'lucide-react';

import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/auth';
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
}

const NAV: NavItem[] = [
  { href: '/siswa', label: 'Dashboard', Icon: LayoutDashboard },
  { href: '/siswa/tugas', label: 'Tugas saya', Icon: ClipboardList },
  { href: '/siswa/ujian', label: 'Ujian saya', Icon: GraduationCap },
  { href: '/siswa/nilai', label: 'Nilai saya', Icon: TrendingUp },
  { href: '/siswa/gabung', label: 'Gabung Kelas', Icon: KeyRound },
];

function isActive(pathname: string, href: string): boolean {
  if (href === '/siswa') return pathname === '/siswa';
  return pathname === href || pathname.startsWith(`${href}/`);
}

const SIDEBAR_MIN = 64;
const SIDEBAR_MAX = 320;
const SIDEBAR_DEFAULT = 240;

function clampSidebarWidth(width: number) {
  return Math.min(SIDEBAR_MAX, Math.max(SIDEBAR_MIN, width));
}

function SiswaShell({ children }: { children: React.ReactNode }) {
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
    const saved = window.localStorage.getItem('lms:siswa-sidebar-width');
    if (!saved) return;
    const parsed = Number(saved);
    if (Number.isFinite(parsed)) {
      const width = clampSidebarWidth(parsed);
      setSidebarWidth(width);
      if (width > SIDEBAR_MIN + 8) setExpandedWidth(width);
    }
  }, []);

  React.useEffect(() => {
    window.localStorage.setItem('lms:siswa-sidebar-width', String(sidebarWidth));
  }, [sidebarWidth]);

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
    <div className="h-screen overflow-hidden bg-muted/30">
      <div className="mx-auto flex h-screen max-w-[1400px] overflow-hidden">
        <aside
          className="sticky top-0 hidden h-screen shrink-0 border-r bg-background transition-[width] md:flex md:flex-col relative"
          style={{ width: sidebarWidth }}
        >
          <div className="flex h-14 items-center border-b px-4">
            <Link href="/siswa" className={cn('text-sm font-semibold tracking-tight', sidebarCollapsed && 'sr-only')}>
              LMS Siswa
            </Link>
          </div>
          <nav className="flex-1 space-y-1 overflow-y-auto p-2">
            {NAV.map(({ href, label, Icon }) => {
              const active = isActive(pathname, href);
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
                  <span className={cn(sidebarCollapsed && 'sr-only')}>{label}</span>
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

        <div className="flex h-screen min-w-0 flex-1 flex-col overflow-y-auto">
          <header className="sticky top-0 z-20 flex h-14 items-center justify-between border-b bg-background/80 px-4 backdrop-blur md:px-6">
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
                <aside className="absolute left-0 top-0 flex h-dvh w-72 max-w-[85vw] flex-col border-r bg-background shadow-xl">
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
              Panel Siswa
            </div>

            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="sm" className="gap-2">
                  <span className="grid size-7 place-items-center rounded-full bg-primary/10 text-xs font-semibold text-primary">
                    {initials}
                  </span>
                  <span className="hidden text-sm sm:inline">
                    {user?.name ?? 'Siswa'}
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

export default function SiswaLayout({ children }: { children: React.ReactNode }) {
  return (
    <RoleGuard allow="siswa">
      <SiswaShell>{children}</SiswaShell>
    </RoleGuard>
  );
}
