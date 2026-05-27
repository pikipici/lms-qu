import { JoinRequestsPanel } from '@/components/JoinRequestsPanel';

export default function GuruSiswaJoinRequestsPage() {
  return (
    <main className="space-y-6 p-4 md:p-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Permintaan Gabung Siswa</h1>
        <p className="text-sm text-muted-foreground">Permintaan hanya untuk kelas yang kamu ampu.</p>
      </div>
      <JoinRequestsPanel scope="guru" />
    </main>
  );
}
