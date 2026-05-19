import Link from 'next/link';

// Locked decision #41: forgot password = "hubungi admin". No self-service in
// MVP. This page is just informational so the login page can link here.
export default function LupaPasswordPage() {
  return (
    <main className="container flex min-h-screen flex-col items-center justify-center py-16">
      <div className="w-full max-w-md space-y-6 rounded-lg border border-border bg-card p-8 shadow-sm">
        <div className="space-y-2">
          <h1 className="text-2xl font-semibold">Lupa Password</h1>
          <p className="text-sm text-muted-foreground">
            Saat ini reset password belum bisa dilakukan mandiri.
          </p>
        </div>

        <div className="rounded-md border border-border bg-muted/40 p-4 text-sm">
          <p className="font-medium">Cara reset:</p>
          <ol className="ml-4 mt-2 list-decimal space-y-1 text-muted-foreground">
            <li>Hubungi admin sekolah atau guru wali kelas.</li>
            <li>Admin akan memberikan password sementara.</li>
            <li>Login dengan password sementara, lalu wajib ganti password baru.</li>
          </ol>
        </div>

        <Link
          href="/login"
          className="inline-flex h-10 w-full items-center justify-center rounded-md border border-border bg-background text-sm font-medium shadow-sm transition-colors hover:bg-accent"
        >
          Kembali ke Masuk
        </Link>
      </div>
    </main>
  );
}
