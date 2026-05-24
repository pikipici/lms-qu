'use client';

import * as React from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Edit, Plus, Search, Trash2 } from 'lucide-react';

import {
  createSekolah,
  deleteSekolah,
  listSekolah,
  type Sekolah,
  updateSekolah,
} from '@/lib/sekolah-api';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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

const emptyForm = { nama: '', npsn: '', alamat: '' };

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
    setForm({ nama: row.nama, npsn: row.npsn ?? '', alamat: row.alamat ?? '' });
  };

  const data = sekolah.data;

  return (
    <main className="space-y-6 p-4 md:p-6">
      <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Master Sekolah</h1>
          <p className="text-sm text-muted-foreground">Kelola daftar sekolah untuk metadata kelas guru.</p>
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
            className="grid gap-3 md:grid-cols-[1.2fr_0.7fr_1.5fr_auto] md:items-end"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate();
            }}
          >
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
            <Button type="submit" disabled={save.isPending}>{save.isPending ? 'Menyimpan...' : 'Simpan'}</Button>
          </form>
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
