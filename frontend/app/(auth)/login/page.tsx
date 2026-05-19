'use client';

// Login stub (Fase 0). The real form (Zod + react-hook-form + TanStack Query
// mutation hitting POST /api/v1/auth/login) lands in Fase 1. This page exists
// so the static export has a valid /login route and so navigation works.
import Link from 'next/link';

export default function LoginPage() {
  return (
    <main className="container flex min-h-screen flex-col items-center justify-center py-16">
      <div className="w-full max-w-sm space-y-6">
        <div className="space-y-2 text-center">
          <h1 className="text-2xl font-semibold">Masuk</h1>
          <p className="text-sm text-muted-foreground">
            Akun dibuat oleh admin sekolah. Tidak ada pendaftaran mandiri.
          </p>
        </div>

        <form
          className="space-y-4 rounded-lg border border-border bg-card p-6 shadow-sm"
          onSubmit={(e) => {
            e.preventDefault();
            // Fase 1 wires the real submit.
          }}
        >
          <div className="space-y-2">
            <label htmlFor="email" className="text-sm font-medium">
              Email
            </label>
            <input
              id="email"
              type="email"
              autoComplete="email"
              disabled
              className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm disabled:cursor-not-allowed disabled:opacity-60"
              placeholder="nama@sekolah.id"
            />
          </div>

          <div className="space-y-2">
            <label htmlFor="password" className="text-sm font-medium">
              Password
            </label>
            <input
              id="password"
              type="password"
              autoComplete="current-password"
              disabled
              className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm disabled:cursor-not-allowed disabled:opacity-60"
            />
          </div>

          <button
            type="submit"
            disabled
            className="inline-flex h-10 w-full items-center justify-center rounded-md bg-primary text-sm font-medium text-primary-foreground shadow transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-60"
          >
            Masuk
          </button>

          <p className="text-center text-xs text-muted-foreground">
            Form aktif di Fase 1. Saat ini hanya skeleton.
          </p>
        </form>

        <div className="text-center text-xs text-muted-foreground">
          <Link href="/lupa-password" className="underline underline-offset-2">
            Lupa password?
          </Link>
        </div>
      </div>
    </main>
  );
}
