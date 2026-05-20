// Force change password page (#32). When user.must_change_password=true,
// frontend redirects here. Form aktif di Fase 1.
export default function MeSecurityPage() {
  return (
    <main className="container py-16">
      <h1 className="text-2xl font-semibold">Keamanan</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        Ganti password aktif di Fase 1.
      </p>
    </main>
  );
}
