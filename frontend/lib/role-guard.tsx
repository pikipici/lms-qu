'use client';

/**
 * RoleGuard — narrows the (authed) layer to a single allowed role.
 *
 * Place inside a route group layout (e.g. /admin) so every child page
 * inherits the check. The parent (authed) layout already enforced login
 * + force-change-password gate; this guard only handles role mismatch.
 *
 * Mismatch behavior: redirect to the user's own role landing.
 */

import * as React from 'react';
import { useRouter } from 'next/navigation';

import { useAuthStore, type Role } from '@/lib/auth';

const landing: Record<Role, string> = {
  admin: '/admin',
  guru: '/guru',
  siswa: '/siswa',
};

export function RoleGuard({
  allow,
  children,
}: {
  allow: Role | Role[];
  children: React.ReactNode;
}) {
  const router = useRouter();
  const role = useAuthStore((s) => s.user?.role ?? null);
  const allowed = React.useMemo(
    () => (Array.isArray(allow) ? allow : [allow]),
    [allow],
  );

  React.useEffect(() => {
    if (!role) return; // parent AuthGuard handles anonymous
    if (!allowed.includes(role)) {
      router.replace(landing[role]);
    }
  }, [role, allowed, router]);

  if (!role || !allowed.includes(role)) return null;
  return <>{children}</>;
}
