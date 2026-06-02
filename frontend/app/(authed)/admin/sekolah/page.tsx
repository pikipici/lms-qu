'use client';

import * as React from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Archive, Edit, MoveRight, Plus, Search, Trash2 } from 'lucide-react';

import { ApiError } from '@/lib/api';
import { archiveRombel, createRombel, deleteRombel, listRombelMembers, listRombels, moveRombelMember, updateRombel, type Rombel, type RombelMember } from '@/lib/rombel-api';
import {
  createSekolah,
  deleteSekolah,
  listSekolah,
  type Sekolah,
  updateSekolah,
} from '@/lib/sekolah-api';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useToast } from '@/hooks/use-toast';

const PAGE_SIZE = 20;

function useDebounced<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = React.useState(value);
  React.useEffect(() => {
    const id = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(id);
  }, [value, delay]);
  return debounced;
}

type SekolahForm = {
  nama: string;
  npsn: string;
  alamat: string;
  siswa_registration_enabled: boolean;
  siswa_registration_mode: 'auto_approve' | 'approval_required';
};

const emptyForm: SekolahForm = {
  nama: '',
  npsn: '',
  alamat: '',
  siswa_registration_enabled: false,
  siswa_registration_mode: 'approval_required',
};

function RombelSection({ sekolah }: { sekolah: Sekolah }) {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [nama, setNama] = React.useState('');
  const [editing, setEditing] = React.useState<Rombel | null>(null);
  const [editNama, setEditNama] = React.useState('');
  const [movingFrom, setMovingFrom] = React.useState<Rombel | null>(null);

  const rombel = useQuery({
    queryKey: ['admin-rombel', sekolah.id],
    queryFn: () => listRombels(sekolah.id, { pageSize: 100 }),
    enabled: Boolean(sekolah.id),
  });

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['admin-rombel', sekolah.id] });
    queryClient.invalidateQueries({ queryKey: ['admin-sekolah'] });
  };

  const create = useMutation({
    mutationFn: () => createRombel(sekolah.id, { nama: nama.trim() }),
    onSuccess: () => {
      setNama('');
      invalidate();
      toast({ title: 'Rombel ditambahkan' });
    },
    onError: (err) => {
      const message = err instanceof ApiError ? err.message : 'Gagal menambah rombel';
      toast({ title: 'Gagal menambah rombel', description: message, variant: 'destructive' });
    },
  });

  const update = useMutation({
    mutationFn: () => {
      if (!editing) throw new Error('rombel kosong');
      return updateRombel(editing.id, { version: editing.version, nama: editNama.trim(), deskripsi: editing.deskripsi ?? '' });
    },
    onSuccess: () => {
      setEditing(null);
      setEditNama('');
      invalidate();
      toast({ title: 'Rombel diperbarui' });
    },
    onError: (err) => {
      const message = err instanceof ApiError ? err.message : 'Gagal update rombel';
      toast({ title: 'Gagal update rombel', description: message, variant: 'destructive' });
    },
  });

  const archive = useMutation({
    mutationFn: archiveRombel,
    onSuccess: () => {
      invalidate();
      toast({ title: 'Rombel diarsipkan' });
    },
    onError: () => toast({ title: 'Gagal arsipkan rombel', variant: 'destructive' }),
  });

  const removeRombel = useMutation({
    mutationFn: deleteRombel,
    onSuccess: () => {
      invalidate();
      toast({ title: 'Rombel dihapus' });
    },
    onError: (err) => {
      const message = err instanceof ApiError ? err.message : 'Hanya rombel kosong yang bisa dihapus permanen';
      toast({ title: 'Gagal hapus rombel', description: message, variant: 'destructive' });
    },
  });

  const canCreate = nama.trim() !== '' && !create.isPending;

  return (
    <div className="space-y-3 rounded-lg border bg-muted/10 p-3">
      <div>
        <h3 className="text-sm font-semibold">Rombel {sekolah.nama}</h3>
        <p className="text-xs text-muted-foreground">Rombel adalah kelas resmi sekolah, contoh VII-A atau VIII-B. Guru pengampu tidak dipilih di sini.</p>
      </div>
      <form
        className="flex flex-col gap-2 md:flex-row"
        onSubmit={(e) => {
          e.preventDefault();
          if (canCreate) create.mutate();
        }}
      >
        <Input value={nama} onChange={(e) => setNama(e.target.value)} placeholder="Nama rombel, misal: VII-A" />
        <Button type="submit" disabled={!canCreate}>{create.isPending ? 'Menambah...' : 'Tambah Rombel'}</Button>
      </form>
      <div className="space-y-2">
        {(rombel.data?.items ?? []).map((item) => (
          <div key={item.id} className="flex flex-col gap-2 rounded-md border bg-background p-2 md:flex-row md:items-center md:justify-between">
            {editing?.id === item.id ? (
              <Input value={editNama} onChange={(e) => setEditNama(e.target.value)} className="md:max-w-xs" />
            ) : (
              <div>
                <div className="font-medium">{item.nama}</div>
                <div className="text-xs text-muted-foreground">{item.jumlah_siswa ?? 0} siswa</div>
              </div>
            )}
            <div className="flex gap-2">
              {editing?.id === item.id ? (
                <>
                  <Button size="sm" onClick={() => update.mutate()} disabled={!editNama.trim() || update.isPending}>Simpan</Button>
                  <Button size="sm" variant="outline" onClick={() => setEditing(null)}>Batal</Button>
                </>
              ) : (
                <>
                  <Button size="sm" variant="outline" onClick={() => { setEditing(item); setEditNama(item.nama); }}><Edit className="mr-2 size-4" />Edit</Button>
                  <Button size="sm" variant="outline" onClick={() => setMovingFrom(item)}><MoveRight className="mr-2 size-4" />Pindah siswa</Button>
                  <Button size="sm" variant="outline" onClick={() => archive.mutate(item.id)} disabled={archive.isPending}><Archive className="mr-2 size-4" />Arsipkan</Button>
                  <Button size="sm" variant="destructive" onClick={() => removeRombel.mutate(item.id)} disabled={removeRombel.isPending}><Trash2 className="mr-2 size-4" />Hapus</Button>
                </>
              )}
            </div>
          </div>
        ))}
        {rombel.isLoading ? <p className="text-sm text-muted-foreground">Memuat rombel...</p> : null}
        {!rombel.isLoading && rombel.data?.items.length === 0 ? <p className="text-sm text-muted-foreground">Belum ada rombel untuk sekolah ini.</p> : null}
      </div>
      <MoveRombelMemberDialog
        open={Boolean(movingFrom)}
        from={movingFrom}
        rombels={rombel.data?.items ?? []}
        onOpenChange={(open) => {
          if (!open) setMovingFrom(null);
        }}
        onMoved={() => {
          setMovingFrom(null);
          invalidate();
        }}
      />
    </div>
  );
}

function MoveRombelMemberDialog({
  open,
  from,
  rombels,
  onOpenChange,
  onMoved,
}: {
  open: boolean;
  from: Rombel | null;
  rombels: Rombel[];
  onOpenChange: (open: boolean) => void;
  onMoved: () => void;
}) {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [selectedSiswa, setSelectedSiswa] = React.useState('');
  const [targetRombel, setTargetRombel] = React.useState('');

  React.useEffect(() => {
    setSelectedSiswa('');
    setTargetRombel('');
  }, [from?.id]);

  const members = useQuery({
    queryKey: ['admin-rombel-members', from?.id],
    queryFn: () => listRombelMembers(from!.id),
    enabled: open && Boolean(from?.id),
  });

  const move = useMutation({
    mutationFn: () => moveRombelMember({ siswa_id: selectedSiswa, to_rombel_id: targetRombel }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-rombel-members'] });
      toast({ title: 'Siswa dipindahkan' });
      onMoved();
    },
    onError: (err) => {
      const message = err instanceof ApiError ? err.message : 'Gagal memindahkan siswa';
      toast({ title: 'Gagal memindahkan siswa', description: message, variant: 'destructive' });
    },
  });

  const memberItems = members.data?.items ?? [];
  const targetOptions = rombels.filter((item) => item.id !== from?.id && item.active && !item.archived_at);
  const canSubmit = Boolean(selectedSiswa && targetRombel) && !move.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-1rem)] max-w-lg">
        <DialogHeader>
          <DialogTitle>Pindah siswa dari {from?.nama ?? 'rombel'}</DialogTitle>
          <DialogDescription>
            Memindahkan siswa akan menonaktifkan rombel lama di sekolah yang sama dan mengaktifkan rombel tujuan.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="move-siswa">Siswa</Label>
            <select
              id="move-siswa"
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background"
              value={selectedSiswa}
              onChange={(e) => setSelectedSiswa(e.target.value)}
              disabled={members.isLoading || memberItems.length === 0}
            >
              <option value="">{members.isLoading ? 'Memuat siswa...' : 'Pilih siswa'}</option>
              {memberItems.map((member: RombelMember) => (
                <option key={member.siswa_id} value={member.siswa_id}>
                  {member.nama || member.email} ({member.email})
                </option>
              ))}
            </select>
            {!members.isLoading && memberItems.length === 0 ? <p className="text-xs text-muted-foreground">Rombel ini belum punya siswa aktif.</p> : null}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="move-target">Rombel tujuan</Label>
            <select
              id="move-target"
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background"
              value={targetRombel}
              onChange={(e) => setTargetRombel(e.target.value)}
              disabled={targetOptions.length === 0}
            >
              <option value="">Pilih rombel tujuan</option>
              {targetOptions.map((item) => (
                <option key={item.id} value={item.id}>{item.nama}</option>
              ))}
            </select>
            {targetOptions.length === 0 ? <p className="text-xs text-muted-foreground">Tambahkan rombel lain dulu untuk memindahkan siswa.</p> : null}
          </div>
        </div>
        <DialogFooter className="gap-2">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>Batal</Button>
          <Button type="button" disabled={!canSubmit} onClick={() => move.mutate()}>
            {move.isPending ? 'Memindahkan...' : 'Pindahkan'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default function AdminSekolahPage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [page, setPage] = React.useState(1);
  const [q, setQ] = React.useState('');
  const debouncedQ = useDebounced(q, 300);
  const [editing, setEditing] = React.useState<Sekolah | null>(null);
  const [form, setForm] = React.useState(emptyForm);

  React.useEffect(() => setPage(1), [debouncedQ]);

  const sekolah = useQuery({
    queryKey: ['admin-sekolah', debouncedQ, page],
    queryFn: () => listSekolah({ q: debouncedQ, page, pageSize: PAGE_SIZE }),
    placeholderData: keepPreviousData,
  });

  const save = useMutation({
    mutationFn: () => (editing ? updateSekolah(editing.id, form) : createSekolah(form)),
    onSuccess: () => {
      setEditing(null);
      setForm(emptyForm);
      queryClient.invalidateQueries({ queryKey: ['admin-sekolah'] });
      toast({ title: 'Sekolah tersimpan' });
    },
    onError: () => toast({ title: 'Gagal menyimpan sekolah', variant: 'destructive' }),
  });

  const remove = useMutation({
    mutationFn: deleteSekolah,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-sekolah'] });
      toast({ title: 'Sekolah dihapus' });
    },
    onError: () => toast({ title: 'Gagal menghapus sekolah', variant: 'destructive' }),
  });

  const startEdit = (row: Sekolah) => {
    setEditing(row);
    setForm({
      nama: row.nama,
      npsn: row.npsn ?? '',
      alamat: row.alamat ?? '',
      siswa_registration_enabled: row.siswa_registration_enabled,
      siswa_registration_mode: row.siswa_registration_mode ?? 'approval_required',
    });
  };

  const data = sekolah.data;

  return (
    <main className="space-y-6 p-4 md:p-6">
      <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Sekolah & Rombel</h1>
          <p className="text-sm text-muted-foreground">Kelola sekolah dan rombel resmi untuk pendaftaran siswa.</p>
        </div>
        <Button onClick={() => { setEditing(null); setForm(emptyForm); }}>
          <Plus className="mr-2 size-4" /> Tambah Sekolah
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{editing ? 'Edit Sekolah' : 'Tambah Sekolah'}</CardTitle>
          <CardDescription>Nama wajib diisi. NPSN opsional tapi harus unik kalau dipakai.</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate();
            }}
          >
            <div className="grid gap-3 md:grid-cols-[1.2fr_0.7fr_1.5fr]">
              <div className="space-y-1.5">
                <Label htmlFor="nama">Nama</Label>
                <Input id="nama" value={form.nama} onChange={(e) => setForm((v) => ({ ...v, nama: e.target.value }))} required />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="npsn">NPSN</Label>
                <Input id="npsn" value={form.npsn} onChange={(e) => setForm((v) => ({ ...v, npsn: e.target.value }))} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="alamat">Alamat</Label>
                <Input id="alamat" value={form.alamat} onChange={(e) => setForm((v) => ({ ...v, alamat: e.target.value }))} />
              </div>
            </div>
            <div className="grid gap-3 rounded-lg border bg-muted/20 p-3 md:grid-cols-[1fr_1fr_auto] md:items-end">
              <label className="flex items-center gap-3 text-sm font-medium">
                <input
                  type="checkbox"
                  className="size-4"
                  checked={form.siswa_registration_enabled}
                  onChange={(e) => setForm((v) => ({ ...v, siswa_registration_enabled: e.target.checked }))}
                />
                Izinkan siswa daftar sendiri
              </label>
              <div className="space-y-1.5">
                <Label htmlFor="registration-mode">Mode pendaftaran</Label>
                <select
                  id="registration-mode"
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm"
                  value={form.siswa_registration_mode}
                  onChange={(e) => setForm((v) => ({ ...v, siswa_registration_mode: e.target.value as 'auto_approve' | 'approval_required' }))}
                >
                  <option value="approval_required">Perlu persetujuan admin/guru</option>
                  <option value="auto_approve">Langsung masuk kelas</option>
                </select>
              </div>
              <Button type="submit" disabled={save.isPending}>{save.isPending ? 'Menyimpan...' : 'Simpan'}</Button>
            </div>
          </form>
          {editing ? <div className="mt-4"><RombelSection sekolah={editing} /></div> : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="gap-3 md:flex-row md:items-center md:justify-between">
          <div>
            <CardTitle>Daftar Sekolah</CardTitle>
            <CardDescription>{data ? `${data.total} sekolah` : 'Memuat data...'}</CardDescription>
          </div>
          <div className="relative w-full md:w-80">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input className="pl-9" placeholder="Cari nama/NPSN..." value={q} onChange={(e) => setQ(e.target.value)} />
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {(data?.items ?? []).map((row) => (
            <div key={row.id} className="flex flex-col gap-3 rounded-lg border p-3 md:flex-row md:items-center md:justify-between">
              <div>
                <div className="font-medium">{row.nama}</div>
                <div className="text-sm text-muted-foreground">{row.npsn || 'Tanpa NPSN'} · {row.alamat || 'Tanpa alamat'}</div>
                <div className="mt-1 text-xs font-medium text-muted-foreground">
                  {row.jumlah_kelas ?? 0} rombel aktif · Daftar siswa: {row.siswa_registration_enabled ? (row.siswa_registration_mode === 'auto_approve' ? 'langsung masuk' : 'perlu approval') : 'nonaktif'}
                </div>
              </div>
              <div className="flex gap-2">
                <Button variant="outline" size="sm" onClick={() => startEdit(row)}><Edit className="mr-2 size-4" />Edit</Button>
                <Button variant="destructive" size="sm" onClick={() => remove.mutate(row.id)} disabled={remove.isPending}><Trash2 className="mr-2 size-4" />Hapus</Button>
              </div>
            </div>
          ))}
          {data?.items.length === 0 ? <p className="py-8 text-center text-sm text-muted-foreground">Belum ada sekolah.</p> : null}
          <div className="flex items-center justify-between pt-2 text-sm text-muted-foreground">
            <span>Halaman {data?.page ?? page} / {data?.total_pages || 1}</span>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((v) => v - 1)}>Prev</Button>
              <Button variant="outline" size="sm" disabled={!data || page >= data.total_pages} onClick={() => setPage((v) => v + 1)}>Next</Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </main>
  );
}
