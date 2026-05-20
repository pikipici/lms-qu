'use client';

/**
 * AuthGuard — client-side route guard for authenticated layouts.
 *
 * Locked decisions referenced:
 *   - #32 Force change password gate: redirect to /me/security when
 *         must_change_password=true; pass through only on that path.
 *   - #57 Auth boundary: any non-anon route requires a valid access token.
 *
 * Behavior:
 *   1. Wait for Zustand persist hydration (avoids flash on hard reload).
 *   2. No access token → router.replace('/login').
 *   3. mustChangePassword=true and not on /me/security → router.replace('/me/security').
 *   4. Otherwise render children.
 *
 * Used at the (authed) route group layout, so any page inside the group
 * inherits the guard with no per-page boilerplate.
 */

import * as React from 'react';
import { useRouter, usePathname } from 'next/navigation';

import { useAuthStore } from '@/lib/auth';

const FORCE_CHANGE_PATH = '/me/security';

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const access = useAuthStore((s) => s.access);
  const user = useAuthStore((s) => s.user);

  // Track when the persist middleware has finished hydrating from
  // localStorage. Before that, store reads return the default null values
  // and we'd false-positive "logged out" on every refresh.
  const [hydrated, setHydrated] = React.useState(false);

  React.useEffect(() => {
    // zustand/persist exposes hasHydrated(); fall back to assuming hydrated
    // on next tick if the API isn't available (older versions).
    const persistApi = (
      useAuthStore as unknown as {
        persist?: {
          hasHydrated?: () => boolean;
          onFinishHydration?: (cb: () => void) => () => void;
        };
      }
    ).persist;

    if (persistApi?.hasHydrated?.()) {
      setHydrated(true);
      return;
    }
    if (persistApi?.onFinishHydration) {
      const unsub = persistApi.onFinishHydration(() => setHydrated(true));
      return unsub;
    }
    // Fallback: trust the next tick.
    const id = window.setTimeout(() => setHydrated(true), 0);
    return () => window.clearTimeout(id);
  }, []);

  React.useEffect(() => {
    if (!hydrated) return;

    if (!access) {
      router.replace('/login');
      return;
    }
    if (user?.mustChangePassword && pathname !== FORCE_CHANGE_PATH) {
      router.replace(FORCE_CHANGE_PATH);
    }
  }, [hydrated, access, user?.mustChangePassword, pathname, router]);

  // Pre-hydration: render nothing to avoid flicker between SSG-blank and
  // the protected page. Static export means the HTML payload is the SSG
  // output; the client-side guard kicks in once hydration completes.
  if (!hydrated) return null;

  // Still anonymous after hydration → redirect effect above will fire.
  // Render nothing while it does.
  if (!access) return null;

  // Force-change gate: hide content on every authed path except the
  // change-password page itself.
  if (user?.mustChangePassword && pathname !== FORCE_CHANGE_PATH) {
    return null;
  }

  return <>{children}</>;
}
