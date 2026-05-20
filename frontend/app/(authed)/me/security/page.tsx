'use client';

/**
 * /me/security — change password (Fase 1.G.3).
 *
 * Locked decisions referenced:
 *   - #32 Force-change-password: when must_change_password=true the AuthGuard
 *         only lets the user reach this page; everything else redirects here.
 *   - #42 ChangePassword revokes ALL refresh tokens for the user (server
 *         decision, conservative default). Client must therefore re-login
 *         after a successful change — we clear the store and bounce to /login.
 *   - #36 Password rules: min 8 chars (frontend mirrors backend validation).
 */

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useMutation } from '@tanstack/react-query';

import { api, ApiError } from '@/lib/api';
import { useAuthStore } from '@/lib/auth';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';

const schema = z
  .object({
    currentPassword: z.string().min(1, { message: 'Password lama wajib diisi.' }),
    newPassword: z
      .string()
      .min(8, { message: 'Password baru minimal 8 karakter.' }),
    confirmPassword: z.string().min(1, { message: 'Konfirmasi password wajib diisi.' }),
  })
  .refine((v) => v.newPassword === v.confirmPassword, {
    path: ['confirmPassword'],
    message: 'Konfirmasi tidak sama dengan password baru.',
  })
  .refine((v) => v.currentPassword !== v.newPassword, {
    path: ['newPassword'],
    message: 'Password baru harus berbeda dari password lama.',
  });

type FormInput = z.infer<typeof schema>;

function friendlyError(err: ApiError): string {
  switch (err.code) {
    case 'invalid_current_password':
      return 'Password lama salah.';
    case 'weak_password':
      return 'Password baru terlalu lemah (minimal 8 karakter).';
    case 'same_password':
      return 'Password baru harus berbeda dari password lama.';
    default:
      return err.message || 'Gagal mengganti password.';
  }
}

export default function MeSecurityPage() {
  const router = useRouter();
  const { toast } = useToast();
  const clear = useAuthStore((s) => s.clear);
  const mustChange = useAuthStore((s) => s.user?.mustChangePassword ?? false);

  const form = useForm<FormInput>({
    resolver: zodResolver(schema),
    defaultValues: {
      currentPassword: '',
      newPassword: '',
      confirmPassword: '',
    },
  });

  const mutation = useMutation({
    mutationFn: (input: FormInput) =>
      api('/auth/change-password', {
        method: 'POST',
        body: {
          current_password: input.currentPassword,
          new_password: input.newPassword,
        },
      }),
    onSuccess: () => {
      toast({
        title: 'Password berhasil diganti',
        description:
          'Untuk keamanan, semua sesi Anda telah keluar. Silakan login ulang.',
      });
      clear();
      router.replace('/login');
    },
    onError: (err: unknown) => {
      const apiErr =
        err instanceof ApiError
          ? err
          : new ApiError({
              status: 0,
              code: 'unknown',
              message: 'Tidak dapat terhubung ke server.',
            });
      toast({
        variant: 'destructive',
        title: 'Gagal ganti password',
        description: apiErr.requestId
          ? `${friendlyError(apiErr)} (req: ${apiErr.requestId})`
          : friendlyError(apiErr),
      });
    },
  });

  const onSubmit = (values: FormInput) => mutation.mutate(values);

  return (
    <main className="container max-w-xl py-12 space-y-6">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Keamanan</h1>
        <p className="text-sm text-muted-foreground">
          Ganti password akun. Setelah berhasil, semua sesi akan keluar dan
          Anda perlu login ulang.
        </p>
      </header>

      {mustChange ? (
        <Card className="border-amber-500/50 bg-amber-500/10">
          <CardHeader>
            <CardTitle className="text-base">Wajib ganti password</CardTitle>
            <CardDescription>
              Akun Anda menggunakan password sementara dari admin. Ganti
              sekarang untuk dapat menggunakan fitur lainnya.
            </CardDescription>
          </CardHeader>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>Form ganti password</CardTitle>
          <CardDescription>
            Minimal 8 karakter. Hindari memakai password lama Anda di sistem
            lain.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(onSubmit)}
              className="space-y-4"
              noValidate
            >
              <FormField
                control={form.control}
                name="currentPassword"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Password lama</FormLabel>
                    <FormControl>
                      <Input
                        type="password"
                        autoComplete="current-password"
                        disabled={mutation.isPending}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="newPassword"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Password baru</FormLabel>
                    <FormControl>
                      <Input
                        type="password"
                        autoComplete="new-password"
                        disabled={mutation.isPending}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="confirmPassword"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Konfirmasi password baru</FormLabel>
                    <FormControl>
                      <Input
                        type="password"
                        autoComplete="new-password"
                        disabled={mutation.isPending}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <Button
                type="submit"
                className="w-full"
                disabled={mutation.isPending}
              >
                {mutation.isPending ? 'Memproses…' : 'Ganti password'}
              </Button>
            </form>
          </Form>
        </CardContent>
        <CardFooter className="justify-between text-xs text-muted-foreground">
          <span>Kembali ke profil</span>
          <Link
            href="/me"
            className={`underline-offset-2 hover:underline ${
              mustChange ? 'pointer-events-none opacity-50' : ''
            }`}
            aria-disabled={mustChange}
            tabIndex={mustChange ? -1 : 0}
          >
            /me
          </Link>
        </CardFooter>
      </Card>
    </main>
  );
}
