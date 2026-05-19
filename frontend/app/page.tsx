import Link from 'next/link';

// Landing page (#5.0). Single CTA: Masuk. No public self-register (#11).
export default function HomePage() {
  return (
    <main className="container flex min-h-screen flex-col items-center justify-center gap-10 py-16">
      <div className="flex flex-col items-center gap-4 text-center">
        <span className="rounded-full border border-border px-3 py-1 text-xs font-medium tracking-wide text-muted-foreground">
          Sekolah · Belajar Tertib
        </span>
        <h1 className="text-4xl font-bold leading-tight md:text-5xl">
          LMS Sekolah
        </h1>
        <p className="max-w-prose text-balance text-muted-foreground">
          Platform belajar berbasis bab. Materi, latihan, ulangan, dan tugas
          terorganisir per kelas. Akun dibuat oleh admin sekolah.
        </p>
      </div>

      <Link
        href="/login"
        className="inline-flex h-11 items-center justify-center rounded-md bg-primary px-6 text-sm font-medium text-primary-foreground shadow transition-colors hover:bg-primary/90"
      >
        Masuk
      </Link>

      <footer className="text-xs text-muted-foreground">
        Lupa kredensial?{' '}
        <Link href="/lupa-password" className="underline underline-offset-2">
          Hubungi admin sekolah
        </Link>
      </footer>
    </main>
  );
}
