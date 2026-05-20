import { AuthGuard } from '@/lib/auth-guard';

/**
 * Layout for the (authed) route group — all pages inside it require a
 * valid access token. The group has no URL segment of its own, so paths
 * stay clean (e.g. /me/security, /admin, /guru/kelas).
 *
 * Force-change-password gate (#32) lives in AuthGuard.
 */
export default function AuthedLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <AuthGuard>{children}</AuthGuard>;
}
