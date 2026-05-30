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
import {
  ArrowRight,
  BookOpenCheck,
  Eye,
  EyeOff,
  GraduationCap,
  ShieldCheck,
} from 'lucide-react';

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
  email: z.string().min(1, { message: 'Email/username wajib diisi.' }),
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
  const [showPassword, setShowPassword] = React.useState(false);

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
    <main className="min-h-screen overflow-hidden bg-[radial-gradient(circle_at_top_left,hsl(var(--primary)/0.16),transparent_34%),linear-gradient(135deg,hsl(var(--background)),hsl(var(--muted)/0.55))]">
      <div className="container flex min-h-screen items-center justify-center px-4 py-8 sm:py-12">
        <div className="grid w-full max-w-5xl items-center gap-6 lg:grid-cols-[1.05fr_0.95fr] lg:gap-10">
          <section className="hidden space-y-6 lg:block">
            <div className="inline-flex items-center gap-2 rounded-full border bg-background/80 px-3 py-1 text-sm font-medium text-muted-foreground shadow-sm backdrop-blur">
              <BookOpenCheck className="size-4 text-primary" />
              LMS sekolah untuk guru dan murid
            </div>
            <div className="space-y-4">
              <h1 className="max-w-xl text-4xl font-bold tracking-tight text-foreground xl:text-5xl">
                Masuk kelas, kerjakan tugas, dan ikuti ujian dari satu tempat.
              </h1>
              <p className="max-w-lg text-base leading-7 text-muted-foreground">
                Gunakan akun yang sudah dibuat atau daftar sebagai siswa kalau sekolahmu membuka pendaftaran mandiri.
              </p>
            </div>
            <div className="grid max-w-xl gap-3 sm:grid-cols-3">
              <div className="rounded-2xl border bg-background/80 p-4 shadow-sm backdrop-blur">
                <GraduationCap className="mb-3 size-5 text-primary" />
                <p className="text-sm font-semibold">Siswa</p>
                <p className="mt-1 text-xs text-muted-foreground">Gabung kelas dan kerjakan ujian.</p>
              </div>
              <div className="rounded-2xl border bg-background/80 p-4 shadow-sm backdrop-blur">
                <BookOpenCheck className="mb-3 size-5 text-primary" />
                <p className="text-sm font-semibold">Guru</p>
                <p className="mt-1 text-xs text-muted-foreground">Kelola kelas, tugas, dan nilai.</p>
              </div>
              <div className="rounded-2xl border bg-background/80 p-4 shadow-sm backdrop-blur">
                <ShieldCheck className="mb-3 size-5 text-primary" />
                <p className="text-sm font-semibold">Admin</p>
                <p className="mt-1 text-xs text-muted-foreground">Pantau data dan akses sekolah.</p>
              </div>
            </div>
          </section>

          <div className="mx-auto w-full max-w-md space-y-4">
            <div className="text-center lg:hidden">
              <div className="mx-auto mb-3 flex size-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground shadow-sm">
                <BookOpenCheck className="size-6" />
              </div>
              <p className="text-sm font-medium text-muted-foreground">LMS Sekolah</p>
            </div>

            <Card className="border-2 shadow-xl shadow-primary/10">
              <CardHeader className="space-y-2 text-center">
                <CardTitle className="text-2xl sm:text-3xl">Masuk ke akun</CardTitle>
                <CardDescription>
                  Pakai email atau username yang terdaftar di LMS.
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
                      name="email"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>Email / Username</FormLabel>
                          <FormControl>
                            <Input
                              type="text"
                              autoComplete="username"
                              placeholder="nama@sekolah.id atau budi01"
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
                            <div className="relative">
                              <Input
                                type={showPassword ? 'text' : 'password'}
                                autoComplete="current-password"
                                disabled={mutation.isPending}
                                className="pr-11"
                                {...field}
                              />
                              <button
                                type="button"
                                className="absolute inset-y-0 right-0 flex w-11 items-center justify-center rounded-r-md text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                                onClick={() => setShowPassword((value) => !value)}
                                disabled={mutation.isPending}
                                aria-label={
                                  showPassword
                                    ? 'Sembunyikan password'
                                    : 'Lihat password'
                                }
                                aria-pressed={showPassword}
                              >
                                {showPassword ? (
                                  <EyeOff className="size-4" />
                                ) : (
                                  <Eye className="size-4" />
                                )}
                              </button>
                            </div>
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <div className="flex justify-end">
                      <Link
                        href="/lupa-password"
                        className="text-xs font-medium text-primary underline-offset-2 hover:underline"
                      >
                        Lupa password?
                      </Link>
                    </div>

                    <Button
                      type="submit"
                      className="h-11 w-full gap-2 font-semibold"
                      disabled={mutation.isPending}
                    >
                      {mutation.isPending ? 'Memproses…' : 'Masuk'}
                      {!mutation.isPending ? <ArrowRight className="size-4" /> : null}
                    </Button>
                  </form>
                </Form>
              </CardContent>
              <CardFooter className="flex-col gap-3 border-t bg-muted/35 px-6 py-5 text-center">
                <p className="text-xs text-muted-foreground">
                  Belum punya akun siswa?
                </p>
                <Link
                  href="/register"
                  className="inline-flex items-center justify-center rounded-md border bg-background px-4 py-2 text-sm font-semibold text-foreground shadow-sm transition-colors hover:bg-muted"
                >
                  Daftar sebagai siswa
                </Link>
              </CardFooter>
            </Card>
            <p className="px-2 text-center text-xs text-muted-foreground">
              Akun guru dan admin dibuat oleh pihak sekolah.
            </p>
          </div>
        </div>
      </div>
    </main>
  );
}
