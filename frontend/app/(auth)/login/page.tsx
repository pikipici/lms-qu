'use client';

/**
 * Login page — Fase 1.G.1.
 *
 * Wired to POST /api/v1/auth/login. On success: store tokens in Zustand,
 * route by must_change_password + role.
 *
 * NOTE: /admin /guru /siswa routes don't exist yet (Fase 1.H+). Redirects
 * will 404 on the FE for now — that's intentional placeholder behavior.
 */

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useMutation } from '@tanstack/react-query';

import { api, ApiError } from '@/lib/api';
import { useAuthStore, type AuthUser, type Role } from '@/lib/auth';
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

const loginSchema = z.object({
  email: z
    .string()
    .min(1, { message: 'Email wajib diisi.' })
    .email({ message: 'Format email tidak valid.' }),
  password: z.string().min(1, { message: 'Password wajib diisi.' }),
});
type LoginInput = z.infer<typeof loginSchema>;

interface LoginResponse {
  user: {
    id: string;
    name: string;
    email: string;
    role: Role;
    status: 'active' | 'suspended' | 'locked';
    must_change_password: boolean;
    last_login_at?: string;
    created_at: string;
    updated_at: string;
  };
  tokens: {
    access_token: string;
    refresh_token: string;
  };
}

function landingFor(role: Role): string {
  if (role === 'admin') return '/admin';
  if (role === 'guru') return '/guru';
  return '/siswa';
}

function friendlyError(err: ApiError): string {
  if (err.code === 'invalid_credentials') return 'Email atau password salah.';
  if (err.code === 'user_suspended')
    return 'Akun di-suspend. Hubungi admin sekolah.';
  if (err.code === 'user_locked')
    return 'Akun dikunci karena terlalu banyak gagal login. Hubungi admin.';
  if (err.code === 'too_many_requests' || err.status === 429)
    return 'Terlalu banyak percobaan. Coba lagi dalam 15 menit.';
  return err.message;
}

export default function LoginPage() {
  const router = useRouter();
  const setSession = useAuthStore((s) => s.setSession);
  const { toast } = useToast();

  const form = useForm<LoginInput>({
    resolver: zodResolver(loginSchema),
    defaultValues: { email: '', password: '' },
  });

  const mutation = useMutation({
    mutationFn: (input: LoginInput) =>
      api<LoginResponse>('/auth/login', {
        method: 'POST',
        body: input,
        anon: true,
      }),
    onSuccess: (data) => {
      const user: AuthUser = {
        id: data.user.id,
        name: data.user.name,
        email: data.user.email,
        role: data.user.role,
        status: data.user.status,
        mustChangePassword: data.user.must_change_password,
      };
      setSession({
        access: data.tokens.access_token,
        refresh: data.tokens.refresh_token,
        user,
      });
      const dest = user.mustChangePassword
        ? '/me/security'
        : landingFor(user.role);
      router.replace(dest);
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
        title: 'Login gagal',
        description: apiErr.requestId
          ? `${friendlyError(apiErr)} (req: ${apiErr.requestId})`
          : friendlyError(apiErr),
      });
    },
  });

  const onSubmit = (values: LoginInput) => mutation.mutate(values);

  return (
    <main className="container flex min-h-screen flex-col items-center justify-center py-16">
      <div className="w-full max-w-sm space-y-6">
        <Card>
          <CardHeader className="space-y-2 text-center">
            <CardTitle className="text-2xl">Masuk</CardTitle>
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
                  name="email"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Email</FormLabel>
                      <FormControl>
                        <Input
                          type="email"
                          autoComplete="email"
                          placeholder="nama@sekolah.id"
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
                  name="password"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Password</FormLabel>
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

                <Button
                  type="submit"
                  className="w-full"
                  disabled={mutation.isPending}
                >
                  {mutation.isPending ? 'Memproses…' : 'Masuk'}
                </Button>
              </form>
            </Form>
          </CardContent>
          <CardFooter className="justify-center">
            <Link
              href="/lupa-password"
              className="text-xs text-muted-foreground underline-offset-2 hover:underline"
            >
              Lupa password?
            </Link>
          </CardFooter>
        </Card>
      </div>
    </main>
  );
}
