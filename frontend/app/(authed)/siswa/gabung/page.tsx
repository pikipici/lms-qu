'use client';

/**
 * /siswa/gabung — siswa gabung kelas via kode invite (Phase 2.C.3 FE).
 *
 * Backend: POST /api/v1/siswa/kelas/join body {kode_invite}.
 * 201 inserted=true → kelas baru gabung. 200 inserted=false → idempotent (sudah join).
 * 404 kode_invite_not_found / 400 kode_invite_required / 409 kelas_archived /
 * 409 enrollment_removed.
 */

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, KeyRound, Loader2 } from 'lucide-react';

import { ApiError } from '@/lib/api';
import { joinKelasByKode } from '@/lib/siswa-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';

const schema = z.object({
  kode_invite: z
    .string()
    .trim()
    .min(1, 'Kode invite wajib diisi')
    .max(32, 'Kode terlalu panjang'),
});

type FormValues = z.infer<typeof schema>;

function mapJoinError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case 'kode_invite_required':
        return 'Kode invite wajib diisi.';
      case 'kode_invite_not_found':
        return 'Kode invite tidak ditemukan. Cek lagi ke guru lu.';
      case 'kelas_archived':
        return 'Kelas ini sudah diarsipkan oleh guru. Tanya guru lu kalau ini error.';
      case 'enrollment_removed':
        return 'Lu pernah dikeluarkan dari kelas ini. Minta guru/admin untuk daftarin ulang.';
      case 'forbidden':
        return 'Akun lu bukan siswa. Login pakai akun siswa.';
      case 'too_many_requests':
        return 'Terlalu banyak percobaan. Coba lagi sebentar.';
      default:
        return err.message || 'Gagal gabung kelas.';
    }
  }
  return 'Terjadi kesalahan jaringan. Coba lagi.';
}

export default function SiswaGabungKelasPage() {
  const router = useRouter();
  const { toast } = useToast();
  const qc = useQueryClient();

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { kode_invite: '' },
  });

  const join = useMutation({
    mutationFn: (values: FormValues) =>
      joinKelasByKode(values.kode_invite.trim().toUpperCase()),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['siswa', 'kelas'] });
      if (data.inserted) {
        toast({
          title: 'Berhasil gabung',
          description: `Kelas "${data.kelas.nama}" sekarang ada di dashboard lu.`,
        });
      } else {
        toast({
          title: 'Sudah pernah gabung',
          description: `Lu udah terdaftar di kelas "${data.kelas.nama}". Buka dashboard.`,
        });
      }
      router.replace('/siswa');
    },
    onError: (err: unknown) => {
      form.setError('kode_invite', { message: mapJoinError(err) });
    },
  });

  const onSubmit = form.handleSubmit((values) => {
    join.mutate(values);
  });

  return (
    <div className="mx-auto max-w-xl space-y-4">
      <div>
        <Button asChild variant="ghost" size="sm" className="-ml-2">
          <Link href="/siswa">
            <ArrowLeft className="size-4" />
            Kembali ke dashboard
          </Link>
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <KeyRound className="size-4" />
            Gabung Kelas via Kode Invite
          </CardTitle>
          <CardDescription>
            Masukin kode 6 karakter yang dikasih guru. Huruf besar/kecil bebas;
            sistem otomatis menormalisasi.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={onSubmit} className="space-y-4">
              <FormField
                control={form.control}
                name="kode_invite"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Kode Invite</FormLabel>
                    <FormControl>
                      <Input
                        placeholder="Contoh: 6NY57C"
                        autoFocus
                        autoComplete="off"
                        spellCheck={false}
                        className="font-mono uppercase tracking-widest"
                        {...field}
                        onChange={(e) =>
                          field.onChange(e.target.value.toUpperCase())
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      Tanya guru lu kalau belum punya kode.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className="flex justify-end gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => router.replace('/siswa')}
                  disabled={join.isPending}
                >
                  Batal
                </Button>
                <Button type="submit" disabled={join.isPending}>
                  {join.isPending && <Loader2 className="size-4 animate-spin" />}
                  Gabung
                </Button>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  );
}
