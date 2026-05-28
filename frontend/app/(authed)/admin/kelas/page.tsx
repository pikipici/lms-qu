'use client';

import * as React from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Archive, ArchiveRestore, Plus, RotateCcw } from 'lucide-react';

import { api, ApiError } from '@/lib/api';
import { type Kelas, archiveKelas, createKelas, listKelas, updateKelas } from '@/lib/kelas-api';
import { listSekolahOptions } from '@/lib/sekolah-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

const PAGE_SIZE = 12;
const selectClass = 'h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50';

interface GuruOption {
  id: string;
  name: string;
  email: string;
}

interface GuruListResponse {
  users: GuruOption[];
}

function formatDate(input?: string | null): string {
  if (!input) return '-';
  try {
    return new Date(input).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Jakarta' });
  } catch {
    return input;
  }
}

function KelasCard({ kelas, onEdit, onArchive }: { kelas: Kelas; onEdit: (kelas: Kelas) => void; onArchive: (kelas: Kelas) => void }) {
  const archived = Boolean(kelas.archived_at);
  return (
    <Card className={archived ? 'opacity-70' : undefined}>
      <CardHeader className="space-y-1.5 pb-3">
        <div className="flex items-start justify-between gap-2">
          <CardTitle className="text-base leading-tight">{kelas.nama}</CardTitle>
          <span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${archived ? 'bg-muted text-muted-foreground' : 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'}`}>
            {archived ? <Archive className="size-3" /> : <ArchiveRestore className="size-3" />}
            {archived ? 'Diarsipkan' : 'Aktif'}
          </span>
        </div>
        <CardDescription className={kelas.deskripsi ? 'line-clamp-2' : 'italic text-muted-foreground/70'}>
          {kelas.deskripsi || 'Tidak ada deskripsi.'}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <div className="rounded-md border bg-muted/30 p-3">
          <div className="text-xs uppercase tracking-wide text-muted-foreground">Kode invite</div>
          <div className="mt-1 font-mono text-lg font-semibold tracking-wider">{kelas.kode_invite}</div>
        </div>
        <dl className="grid grid-cols-2 gap-x-3 gap-y-1.5 text-xs">
          <dt className="text-muted-foreground">Sekolah</dt>
          <dd className="text-right font-medium">{kelas.sekolah_nama || 'Tanpa sekolah'}</dd>
          <dt className="text-muted-foreground">Murid</dt>
          <dd className="text-right font-medium">{kelas.jumlah_murid ?? 0}</dd>
          <dt className="text-muted-foreground">Dibuat</dt>
          <dd className="text-right text-muted-foreground">{formatDate(kelas.created_at)}</dd>
        </dl>
        <div className="flex gap-2 pt-1">
          <Button type="button" variant="outline" size="sm" className="flex-1" onClick={() => onEdit(kelas)}>
            Edit
          </Button>
          {!archived ? (
            <Button type="button" variant="outline" size="sm" className="flex-1" onClick={() => onArchive(kelas)}>
              Arsipkan
            </Button>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}

function CreateKelasDialog({ open, onOpenChange, onCreated }: { open: boolean; onOpenChange: (open: boolean) => void; onCreated: () => void }) {
  const { toast } = useToast();
  const [nama, setNama] = React.useState('');
  const [deskripsi, setDeskripsi] = React.useState('');
  const [sekolahID, setSekolahID] = React.useState('');
  const [guruID, setGuruID] = React.useState('');

  React.useEffect(() => {
    if (!open) {
      setNama('');
      setDeskripsi('');
      setSekolahID('');
      setGuruID('');
    }
  }, [open]);

  const sekolahQuery = useQuery({ queryKey: ['sekolah-options'], queryFn: () => listSekolahOptions({ pageSize: 100 }), enabled: open, staleTime: 60_000 });
  const guruQuery = useQuery({
    queryKey: ['admin', 'guru-options'],
    queryFn: () => api<GuruListResponse>('/admin/users?role=guru&status=active&page_size=100'),
    enabled: open,
    staleTime: 60_000,
  });

  const mutation = useMutation({
    mutationFn: () => createKelas({ nama: nama.trim(), deskripsi: deskripsi.trim() || undefined, sekolah_id: sekolahID || undefined, guru_id: guruID }),
    onSuccess: ({ kelas }) => {
      toast({ title: 'Kelas dibuat', description: `${kelas.nama} siap dipakai. Kode: ${kelas.kode_invite}` });
      onCreated();
      onOpenChange(false);
    },
    onError: (err) => {
      const message = err instanceof ApiError ? err.message : 'Gagal membuat kelas.';
      const requestId = err instanceof ApiError ? err.requestId : undefined;
      toast({ title: 'Tidak bisa membuat kelas', description: requestId ? `${message} (req: ${requestId})` : message, variant: 'destructive' });
    },
  });

  const canSubmit = nama.trim() !== '' && guruID !== '' && !mutation.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Tambah kelas</DialogTitle>
          <DialogDescription>Admin menentukan sekolah dan guru pengampu. Siswa nanti memilih kelas ini saat daftar.</DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (canSubmit) mutation.mutate(); }}>
          <div className="space-y-1.5">
            <Label htmlFor="nama">Nama kelas</Label>
            <Input id="nama" value={nama} onChange={(e) => setNama(e.target.value)} placeholder="Misal: 7A Matematika" required />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="guru">Guru pengampu</Label>
            <select id="guru" className={selectClass} value={guruID} onChange={(e) => setGuruID(e.target.value)} disabled={guruQuery.isLoading} required>
              <option value="">Pilih guru</option>
              {(guruQuery.data?.users ?? []).map((g) => <option key={g.id} value={g.id}>{g.name} - {g.email}</option>)}
            </select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="sekolah">Sekolah</Label>
            <select id="sekolah" className={selectClass} value={sekolahID} onChange={(e) => setSekolahID(e.target.value)} disabled={sekolahQuery.isLoading}>
              <option value="">Tanpa sekolah</option>
              {(sekolahQuery.data?.items ?? []).map((s) => <option key={s.id} value={s.id}>{s.nama}</option>)}
            </select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="deskripsi">Deskripsi (opsional)</Label>
            <textarea id="deskripsi" className="flex min-h-[72px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" value={deskripsi} onChange={(e) => setDeskripsi(e.target.value)} placeholder="Catatan singkat tentang kelas ini." />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={mutation.isPending}>Batal</Button>
            <Button type="submit" disabled={!canSubmit}>{mutation.isPending ? 'Menyimpan...' : 'Tambah kelas'}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function EditKelasDialog({ kelas, onOpenChange, onSaved }: { kelas: Kelas | null; onOpenChange: (open: boolean) => void; onSaved: () => void }) {
  const { toast } = useToast();
  const [nama, setNama] = React.useState('');
  const [deskripsi, setDeskripsi] = React.useState('');

  React.useEffect(() => {
    setNama(kelas?.nama ?? '');
    setDeskripsi(kelas?.deskripsi ?? '');
  }, [kelas]);

  const mutation = useMutation({
    mutationFn: () => {
      if (!kelas) throw new Error('kelas kosong');
      return updateKelas(kelas.id, { version: kelas.version, nama: nama.trim(), deskripsi: deskripsi.trim() });
    },
    onSuccess: ({ kelas: saved }) => {
      toast({ title: 'Kelas diperbarui', description: saved.nama });
      onSaved();
      onOpenChange(false);
    },
    onError: (err) => {
      const message = err instanceof ApiError ? err.message : 'Gagal memperbarui kelas.';
      toast({ title: 'Tidak bisa update kelas', description: message, variant: 'destructive' });
    },
  });

  return (
    <Dialog open={Boolean(kelas)} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Edit kelas</DialogTitle>
          <DialogDescription>Ubah nama dan deskripsi kelas. Sekolah/guru pengampu belum dipindah dari dialog ini.</DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (nama.trim()) mutation.mutate(); }}>
          <div className="space-y-1.5">
            <Label htmlFor="edit-nama">Nama kelas</Label>
            <Input id="edit-nama" value={nama} onChange={(e) => setNama(e.target.value)} required />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="edit-deskripsi">Deskripsi</Label>
            <textarea id="edit-deskripsi" className="flex min-h-[72px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" value={deskripsi} onChange={(e) => setDeskripsi(e.target.value)} />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={mutation.isPending}>Batal</Button>
            <Button type="submit" disabled={!nama.trim() || mutation.isPending}>{mutation.isPending ? 'Menyimpan...' : 'Simpan'}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function ArchiveKelasDialog({ kelas, onOpenChange, onArchived }: { kelas: Kelas | null; onOpenChange: (open: boolean) => void; onArchived: () => void }) {
  const { toast } = useToast();
  const mutation = useMutation({
    mutationFn: () => {
      if (!kelas) throw new Error('kelas kosong');
      return archiveKelas(kelas.id);
    },
    onSuccess: ({ kelas: archived }) => {
      toast({ title: 'Kelas diarsipkan', description: archived.nama });
      onArchived();
      onOpenChange(false);
    },
    onError: (err) => {
      const message = err instanceof ApiError ? err.message : 'Gagal mengarsipkan kelas.';
      toast({ title: 'Tidak bisa arsipkan kelas', description: message, variant: 'destructive' });
    },
  });

  return (
    <Dialog open={Boolean(kelas)} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Arsipkan kelas?</DialogTitle>
          <DialogDescription>Kelas {kelas?.nama ? `"${kelas.nama}"` : 'ini'} akan disembunyikan dari daftar aktif.</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={mutation.isPending}>Batal</Button>
          <Button type="button" variant="destructive" onClick={() => mutation.mutate()} disabled={mutation.isPending}>{mutation.isPending ? 'Mengarsipkan...' : 'Arsipkan'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default function AdminKelasPage() {
  const [page, setPage] = React.useState(1);
  const [includeArchived, setIncludeArchived] = React.useState(false);
  const [selectedSekolahId, setSelectedSekolahId] = React.useState('');
  const [createOpen, setCreateOpen] = React.useState(false);
  const [editingKelas, setEditingKelas] = React.useState<Kelas | null>(null);
  const [archivingKelas, setArchivingKelas] = React.useState<Kelas | null>(null);
  const queryClient = useQueryClient();

  React.useEffect(() => setPage(1), [includeArchived, selectedSekolahId]);

  const sekolahQuery = useQuery({ queryKey: ['sekolah-options'], queryFn: () => listSekolahOptions({ pageSize: 100 }), staleTime: 60_000 });
  const kelasQuery = useQuery({
    queryKey: ['admin', 'kelas', { page, includeArchived, selectedSekolahId }],
    queryFn: () => listKelas({ page, pageSize: PAGE_SIZE, includeArchived, sekolahId: selectedSekolahId || undefined }),
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });

  const items = kelasQuery.data?.items ?? [];
  const total = kelasQuery.data?.total ?? 0;
  const totalPages = kelasQuery.data?.total_pages ?? 0;

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">Kelas</h1>
          <p className="text-sm text-muted-foreground">Admin membuat kelas, memilih sekolah, dan assign guru pengampu.</p>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)}><Plus className="size-4" />Tambah kelas</Button>
      </header>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-3 space-y-0">
          <div className="space-y-1">
            <CardTitle className="text-base">Daftar kelas</CardTitle>
            <CardDescription>{kelasQuery.isPending ? 'Memuat...' : `Total ${total} kelas${totalPages > 1 ? ` - Halaman ${page} / ${totalPages}` : ''}`}</CardDescription>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <select className="h-9 min-w-48 rounded-md border border-input bg-background px-3 text-sm shadow-sm" value={selectedSekolahId} onChange={(e) => setSelectedSekolahId(e.target.value)} disabled={sekolahQuery.isLoading}>
              <option value="">Semua sekolah</option>
              {(sekolahQuery.data?.items ?? []).map((s) => <option key={s.id} value={s.id}>{s.nama}</option>)}
            </select>
            <Label className="flex cursor-pointer items-center gap-2 text-xs text-muted-foreground">
              <input type="checkbox" className="size-4 rounded border-input" checked={includeArchived} onChange={(e) => setIncludeArchived(e.target.checked)} />
              Tampilkan diarsipkan
            </Label>
            <Button variant="outline" size="sm" onClick={() => kelasQuery.refetch()} disabled={kelasQuery.isFetching}><RotateCcw className="size-4" />Refresh</Button>
          </div>
        </CardHeader>
        <CardContent>
          {kelasQuery.isPending ? (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">{Array.from({ length: 3 }).map((_, i) => <div key={i} className="h-56 animate-pulse rounded-md border bg-muted/40" />)}</div>
          ) : kelasQuery.isError ? (
            <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">Gagal memuat daftar kelas.</div>
          ) : items.length === 0 ? (
            <div className="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">Belum ada kelas. Klik Tambah kelas untuk mulai.</div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {items.map((k) => (
                <KelasCard key={k.id} kelas={k} onEdit={setEditingKelas} onArchive={setArchivingKelas} />
              ))}
            </div>
          )}
          <div className="mt-4 flex flex-wrap items-center justify-end gap-2 text-sm text-muted-foreground">
            <Button variant="outline" size="sm" disabled={page <= 1 || kelasQuery.isFetching} onClick={() => setPage((p) => Math.max(1, p - 1))}>Prev</Button>
            <Button variant="outline" size="sm" disabled={totalPages > 0 ? page >= totalPages : items.length < PAGE_SIZE} onClick={() => setPage((p) => p + 1)}>Next</Button>
          </div>
        </CardContent>
      </Card>

      <CreateKelasDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={() => {
          setPage(1);
          queryClient.invalidateQueries({ queryKey: ['admin', 'kelas'] });
        }}
      />
      <EditKelasDialog
        kelas={editingKelas}
        onOpenChange={(open) => {
          if (!open) setEditingKelas(null);
        }}
        onSaved={() => queryClient.invalidateQueries({ queryKey: ['admin', 'kelas'] })}
      />
      <ArchiveKelasDialog
        kelas={archivingKelas}
        onOpenChange={(open) => {
          if (!open) setArchivingKelas(null);
        }}
        onArchived={() => queryClient.invalidateQueries({ queryKey: ['admin', 'kelas'] })}
      />
    </div>
  );
}
