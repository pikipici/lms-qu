'use client';

/**
 * Siswa shell — neo-brutalism + pastel pop theme (siswa-only).
 *
 * Wraps everything in `.siswa-theme` so design tokens activate locally.
 * Admin / guru shells stay on the original shadcn neutral theme.
 */

import * as React from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import {
  ClipboardList,
  GraduationCap,
  KeyRound,
  LayoutDashboard,
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
  /** Pastel accent applied to the active state. */
  accent: 'yellow' | 'pink' | 'blue' | 'green' | 'lavender';
}

const NAV: NavItem[] = [
  { href: '/siswa', label: 'Dashboard', Icon: LayoutDashboard, accent: 'yellow' },
  { href: '/siswa/tugas', label: 'Tugas saya', Icon: ClipboardList, accent: 'pink' },
  { href: '/siswa/nilai', label: 'Nilai saya', Icon: TrendingUp, accent: 'lavender' },
  { href: '/siswa/gabung', label: 'Gabung Kelas', Icon: KeyRound, accent: 'green' },
];

const ACCENT_BG: Record<NavItem['accent'], string> = {
  yellow: 'bg-siswa-yellow',
  pink: 'bg-siswa-pink',
  blue: 'bg-siswa-blue',
  green: 'bg-siswa-green',
  lavender: 'bg-siswa-lavender',
};

function isActive(pathname: string, href: string): boolean {
  if (href === '/siswa') return pathname === '/siswa';
  return pathname === href || pathname.startsWith(`${href}/`);
}

const SIDEBAR_MIN = 72;
const SIDEBAR_MAX = 320;
const SIDEBAR_DEFAULT = 248;

function clampSidebarWidth(width: number) {
  return Math.min(SIDEBAR_MAX, Math.max(SIDEBAR_MIN, width));
}

function NavLink({
  item,
  active,
  collapsed,
  onClick,
}: {
  item: NavItem;
  active: boolean;
  collapsed?: boolean;
  onClick?: () => void;
}) {
  const { Icon, accent, label, href } = item;
  return (
    <Link
      href={href}
      onClick={onClick}
      className={cn(
        'group relative flex items-center gap-3 rounded-siswa px-3 py-2 text-sm font-semibold transition-colors',
        'border-2 border-transparent',
        active
          ? cn('siswa-border siswa-shadow-sm', ACCENT_BG[accent])
          : 'text-siswa-text/80 hover:bg-siswa-cream/70 hover:text-siswa-text',
      )}
    >
      <Icon className="size-4 shrink-0" />
      <span className={cn(collapsed && 'sr-only')}>{label}</span>
    </Link>
  );
}

function SiswaShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { toast } = useToast();
  const user = useAuthStore((s) => s.user);
  const refresh = useAuthStore((s) => s.refresh);
  const clear = useAuthStore((s) => s.clear);

  const [sidebarOpen, setSidebarOpen] = React.useState(false);
  const [sidebarWidth, setSidebarWidth] = React.useState(SIDEBAR_DEFAULT);
  const [expandedWidth, setExpandedWidth] = React.useState(SIDEBAR_DEFAULT);
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

  const startResize = React.useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
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
    },
    [sidebarWidth],
  );

  const toggleSidebar = () => {
    setSidebarWidth((width) =>
      width <= SIDEBAR_MIN + 8 ? expandedWidth : SIDEBAR_MIN,
    );
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
    <div className="siswa-theme min-h-screen">
      <div className="flex h-screen w-full overflow-hidden">
        {/* Sidebar (md+) */}
        <aside
          className="sticky top-0 hidden h-screen shrink-0 border-r-2 border-siswa-border bg-siswa-surface transition-[width] md:flex md:flex-col relative"
          style={{ width: sidebarWidth }}
        >
          <div className="flex h-16 items-center border-b-2 border-siswa-border px-4">
            <Link
              href="/siswa"
              className={cn(
                'flex items-center gap-2 font-semibold tracking-tight',
                sidebarCollapsed && 'justify-center',
              )}
            >
              <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-yellow siswa-shadow-sm">
                <GraduationCap className="size-5" strokeWidth={2.5} />
              </span>
              <span
                className={cn(
                  'siswa-display text-base font-bold',
                  sidebarCollapsed && 'sr-only',
                )}
              >
                LMS Siswa
              </span>
            </Link>
          </div>
          <nav className="flex-1 space-y-1.5 overflow-y-auto p-3">
            {NAV.map((item) => (
              <NavLink
                key={item.href}
                item={item}
                active={isActive(pathname, item.href)}
                collapsed={sidebarCollapsed}
              />
            ))}
          </nav>
          <div className="border-t-2 border-siswa-border p-3">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="w-full justify-center text-siswa-text hover:bg-siswa-cream"
              aria-label={sidebarCollapsed ? 'Lebarkan sidebar' : 'Ciutkan sidebar'}
              onClick={toggleSidebar}
            >
              <PanelLeftClose
                className={cn(
                  'size-4 transition-transform',
                  sidebarCollapsed && 'rotate-180',
                )}
              />
              <span className={cn('ml-2', sidebarCollapsed && 'sr-only')}>
                {sidebarCollapsed ? 'Lebarkan' : 'Ciutkan'}
              </span>
            </Button>
          </div>
          <button
            type="button"
            aria-label="Resize sidebar"
            className="absolute -right-2 top-24 hidden h-14 w-4 cursor-col-resize touch-none rounded-full border-2 border-siswa-border bg-siswa-surface siswa-shadow-sm hover:bg-siswa-yellow md:block"
            onPointerDown={startResize}
          />
        </aside>

        {/* Main area */}
        <div className="flex h-screen min-w-0 flex-1 flex-col overflow-y-auto">
          <header className="sticky top-0 z-20 flex h-16 items-center justify-between border-b-2 border-siswa-border bg-siswa-bg/90 px-4 backdrop-blur md:px-6">
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

            {sidebarOpen ? (
              <div className="fixed inset-0 z-50 md:hidden">
                <button
                  type="button"
                  className="absolute inset-0 bg-siswa-text/40 backdrop-blur-sm"
                  aria-label="Tutup menu navigasi"
                  onClick={() => setSidebarOpen(false)}
                />
                <aside className="absolute left-0 top-0 flex h-dvh w-72 max-w-[85vw] flex-col border-r-2 border-siswa-border bg-siswa-surface">
                  <div className="flex h-16 items-center justify-between border-b-2 border-siswa-border px-4">
                    <span className="siswa-display text-base font-bold">
                      Menu
                    </span>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setSidebarOpen(false)}
                    >
                      Tutup
                    </Button>
                  </div>
                  <nav className="flex-1 space-y-1.5 overflow-y-auto p-3">
                    {NAV.map((item) => (
                      <NavLink
                        key={item.href}
                        item={item}
                        active={isActive(pathname, item.href)}
                        onClick={() => setSidebarOpen(false)}
                      />
                    ))}
                  </nav>
                </aside>
              </div>
            ) : null}

            <div className="hidden text-xs font-semibold uppercase tracking-[0.18em] text-siswa-text-muted md:block">
              Panel Siswa
            </div>

            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <button
                  type="button"
                  className="flex items-center gap-2 rounded-full border-2 border-siswa-border bg-siswa-surface px-2 py-1 siswa-shadow-sm hover:bg-siswa-cream"
                >
                  <span className="grid size-8 place-items-center rounded-full bg-siswa-yellow text-xs font-bold text-siswa-text">
                    {initials}
                  </span>
                  <span className="hidden pr-1 text-sm font-semibold sm:inline">
                    {user?.name ?? 'Siswa'}
                  </span>
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-56">
                <DropdownMenuLabel className="font-normal">
                  <div className="flex flex-col space-y-1">
                    <span className="text-sm font-semibold">{user?.name}</span>
                    <span className="break-all text-xs text-muted-foreground">
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
                <DropdownMenuItem
                  onSelect={onLogout}
                  className="text-destructive"
                >
                  <LogOut className="mr-2 size-4" />
                  Logout
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </header>

          <main className="flex-1 p-4 md:p-8">
            <div className="mx-auto w-full max-w-screen-2xl">{children}</div>
          </main>
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
