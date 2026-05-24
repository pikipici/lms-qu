'use client';

/**
 * /siswa/gabung — siswa gabung kelas via kode invite (Phase 2.C.3 FE).
 *
 * Backend: POST /api/v1/siswa/kelas/join body {kode_invite}.
 * 201 inserted=true → kelas baru gabung. 200 inserted=false → idempotent.
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
import { ArrowLeft, KeyRound, Loader2, Sparkles } from 'lucide-react';

import { ApiError } from '@/lib/api';
import { joinKelasByKode } from '@/lib/siswa-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
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
import {
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardDescription,
  SiswaCardHeader,
  SiswaCardTitle,
  SiswaPageHeader,
} from '@/components/siswa-ui';

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
    <div className="mx-auto max-w-2xl space-y-6">
      <Button asChild variant="ghost" size="sm" className="-ml-2 text-siswa-text">
        <Link href="/siswa">
          <ArrowLeft className="size-4" />
          Kembali ke dashboard
        </Link>
      </Button>

      <SiswaPageHeader
        eyebrow="Gabung Kelas"
        title="Punya kode invite?"
        description="Masukin kode 6 karakter dari guru. Huruf besar/kecil bebas — sistem auto-normalisasi."
      />

      <SiswaCard tone="latihan" shadow="md">
        <SiswaCardHeader>
          <div className="flex items-start justify-between gap-3">
            <div className="space-y-1">
              <SiswaCardTitle className="flex items-center gap-2">
                <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                  <KeyRound className="size-4" strokeWidth={2.5} />
                </span>
                Kode Invite
              </SiswaCardTitle>
              <SiswaCardDescription>
                Kalau belum punya, tanya wali kelas atau guru mata pelajaran lu.
              </SiswaCardDescription>
            </div>
            <Sparkles className="size-5 text-siswa-text/60" strokeWidth={2.5} />
          </div>
        </SiswaCardHeader>

        <SiswaCardBody>
          <Form {...form}>
            <form onSubmit={onSubmit} className="space-y-5">
              <FormField
                control={form.control}
                name="kode_invite"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-sm font-semibold uppercase tracking-wider text-siswa-text-muted">
                      Kode 6 karakter
                    </FormLabel>
                    <FormControl>
                      <Input
                        placeholder="6NY57C"
                        autoFocus
                        autoComplete="off"
                        spellCheck={false}
                        maxLength={32}
                        className="h-14 rounded-siswa border-2 border-siswa-border bg-siswa-surface text-center font-mono text-2xl font-bold uppercase tracking-[0.5em] text-siswa-text shadow-none focus-visible:ring-0 focus-visible:border-siswa-border focus-visible:outline-none"
                        {...field}
                        onChange={(e) =>
                          field.onChange(e.target.value.toUpperCase())
                        }
                      />
                    </FormControl>
                    <FormDescription className="text-siswa-text-muted">
                      Tip: tempel kode dari pesan guru, sistem otomatis bersihin spasi.
                    </FormDescription>
                    <FormMessage className="font-semibold" />
                  </FormItem>
                )}
              />

              <div className="flex flex-wrap justify-end gap-3">
                <SiswaButton
                  type="button"
                  tone="ghost"
                  onClick={() => router.replace('/siswa')}
                  disabled={join.isPending}
                >
                  Batal
                </SiswaButton>
                <SiswaButton
                  type="submit"
                  tone="primary"
                  disabled={join.isPending}
                >
                  {join.isPending ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <KeyRound className="size-4" strokeWidth={2.5} />
                  )}
                  Gabung
                </SiswaButton>
              </div>
            </form>
          </Form>
        </SiswaCardBody>
      </SiswaCard>

      <div className="rounded-siswa border-2 border-dashed border-siswa-border-soft bg-siswa-surface/50 p-4 text-sm text-siswa-text-muted">
        💡 <span className="font-semibold text-siswa-text">Tidak punya kode?</span>{' '}
        Admin sekolah mungkin sudah meng-assign lu ke kelas tanpa kode. Cek
        dashboard dulu sebelum nanya.
      </div>
    </div>
  );
}
