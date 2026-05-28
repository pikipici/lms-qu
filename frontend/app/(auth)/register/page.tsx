'use client';

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useMutation, useQuery } from '@tanstack/react-query';

import { ApiError } from '@/lib/api';
import { listPublicKelas, listPublicSekolah, registerSiswa } from '@/lib/registration-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

const emptyForm = {
  nama: '',
  username: '',
  password: '',
  password_confirm: '',
  sekolah_id: '',
  kelas_id: '',
};

function friendlyError(err: unknown): string {
  if (!(err instanceof ApiError)) return 'Tidak dapat terhubung ke server.';
  if (err.code === 'username_taken') return 'Username sudah dipakai. Pilih username lain.';
  if (err.code === 'registration_disabled') return 'Pendaftaran mandiri belum aktif untuk sekolah ini.';
  if (err.code === 'kelas_not_in_sekolah') return 'Kelas tidak cocok dengan sekolah yang dipilih.';
  if (err.code === 'password_mismatch') return 'Konfirmasi password tidak sama.';
  return err.message;
}

export default function RegisterSiswaPage() {
  const router = useRouter();
  const { toast } = useToast();
  const [form, setForm] = React.useState(emptyForm);

  const sekolahQ = useQuery({ queryKey: ['public-sekolah'], queryFn: listPublicSekolah });
  const sekolahItems = sekolahQ.data?.data ?? [];
  const selectedSekolah = sekolahItems.find((s) => s.id === form.sekolah_id);
  const kelasQ = useQuery({
    queryKey: ['public-kelas', form.sekolah_id],
    queryFn: () => listPublicKelas(form.sekolah_id),
    enabled: Boolean(form.sekolah_id),
  });

  const mutation = useMutation({
    mutationFn: registerSiswa,
    onSuccess: (res) => {
      toast({ title: 'Pendaftaran berhasil', description: res.message });
      router.replace('/login');
    },
    onError: (err) => toast({ title: 'Pendaftaran gagal', description: friendlyError(err), variant: 'destructive' }),
  });

  return (
    <main className="container flex min-h-screen flex-col items-center justify-center py-16">
      <div className="w-full max-w-lg space-y-6">
        <Card>
          <CardHeader className="space-y-2 text-center">
            <CardTitle className="text-2xl">Daftar sebagai Siswa</CardTitle>
            <CardDescription>Pilih sekolah dan kelas yang sudah disiapkan admin.</CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault();
                mutation.mutate(form);
              }}
            >
              <div className="space-y-1.5">
                <Label htmlFor="nama">Nama Lengkap</Label>
                <Input id="nama" value={form.nama} onChange={(e) => setForm((v) => ({ ...v, nama: e.target.value }))} required />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="username">Username</Label>
                <Input id="username" value={form.username} onChange={(e) => setForm((v) => ({ ...v, username: e.target.value }))} required placeholder="contoh: budi01" />
                <p className="text-xs text-muted-foreground">Gunakan huruf kecil, angka, titik, dash, atau underscore.</p>
              </div>
              <div className="grid gap-3 md:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="password">Password</Label>
                  <Input id="password" type="password" value={form.password} onChange={(e) => setForm((v) => ({ ...v, password: e.target.value }))} required />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="password_confirm">Konfirmasi Password</Label>
                  <Input id="password_confirm" type="password" value={form.password_confirm} onChange={(e) => setForm((v) => ({ ...v, password_confirm: e.target.value }))} required />
                </div>
              </div>
              <div className="grid gap-3 md:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="sekolah">Sekolah</Label>
                  <select
                    id="sekolah"
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm"
                    value={form.sekolah_id}
                    onChange={(e) => setForm((v) => ({ ...v, sekolah_id: e.target.value, kelas_id: '' }))}
                    required
                  >
                    <option value="">Pilih sekolah</option>
                    {sekolahItems.map((s) => <option key={s.id} value={s.id}>{s.nama}</option>)}
                  </select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="kelas">Kelas</Label>
                  <select
                    id="kelas"
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm"
                    value={form.kelas_id}
                    onChange={(e) => setForm((v) => ({ ...v, kelas_id: e.target.value }))}
                    disabled={!form.sekolah_id || kelasQ.isLoading}
                    required
                  >
                    <option value="">{form.sekolah_id ? 'Pilih kelas' : 'Pilih sekolah dulu'}</option>
                    {(kelasQ.data?.data ?? []).map((k) => <option key={k.id} value={k.id}>{k.nama}</option>)}
                  </select>
                </div>
              </div>
              {selectedSekolah ? (
                <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">
                  Mode sekolah ini: {selectedSekolah.siswa_registration_mode === 'auto_approve' ? 'langsung masuk kelas setelah daftar.' : 'menunggu persetujuan admin/guru setelah daftar.'}
                </p>
              ) : null}
              <Button className="w-full" type="submit" disabled={mutation.isPending || sekolahQ.isLoading}>
                {mutation.isPending ? 'Mendaftar...' : 'Daftar'}
              </Button>
            </form>
          </CardContent>
          <CardFooter className="justify-center">
            <Link href="/login" className="text-xs text-muted-foreground underline-offset-2 hover:underline">
              Sudah punya akun? Masuk
            </Link>
          </CardFooter>
        </Card>
      </div>
    </main>
  );
}
