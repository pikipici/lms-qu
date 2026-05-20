'use client';

/**
 * /admin/pengguna/baru — create user (Fase 1.H.3).
 *
 * Backend contract: POST /api/v1/admin/users
 *   body: { name, email, role, password_strategy: 'manual'|'generate', password? }
 *   resp: 201 { user, generated_password? }
 *
 * Locked decision #31 — show the password (typed or generated) ONCE in the
 * success view; admin must hand it to the user out-of-band. After this
 * screen the plaintext is unrecoverable; only "Reset password" can issue
 * a new one.
 *
 * Locked decision #52 — re-auth (current_password) is only required for
 * promote/demote (POST /admin/users/:id/role), not for create. Backend
 * does not accept current_password on this endpoint.
 */

import * as React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useMutation } from '@tanstack/react-query';
import { ArrowLeft, ClipboardCopy, ClipboardCheck, ShieldAlert } from 'lucide-react';

import { api, ApiError } from '@/lib/api';
import { type Role } from '@/lib/auth';
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
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

const PasswordStrategy = z.enum(['manual', 'generate']);

const baseSchema = z.object({
  name: z.string().trim().min(1, { message: 'Nama wajib diisi.' }).max(120),
  email: z
    .string()
    .trim()
    .toLowerCase()
    .min(1, { message: 'Email wajib diisi.' })
    .email({ message: 'Format email tidak valid.' }),
  role: z.enum(['admin', 'guru', 'siswa']),
  password_strategy: PasswordStrategy,
  password: z.string().default(''),
});

const schema = baseSchema.superRefine((value, ctx) => {
  if (value.password_strategy === 'manual') {
    if (value.password.length < 8) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['password'],
        message: 'Password minimal 8 karakter.',
      });
    }
  }
});

type FormInput = z.infer<typeof schema>;

interface CreatedUser {
  id: string;
  name: string;
  email: string;
  role: Role;
}

interface CreateResponse {
  user: CreatedUser;
  generated_password?: string | null;
}

interface SuccessState {
  user: CreatedUser;
  password: string;
  strategy: 'manual' | 'generate';
}

const roleLabel: Record<Role, string> = {
  admin: 'Administrator',
  guru: 'Guru',
  siswa: 'Siswa',
};

const selectClass =
  'h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50';

function friendlyError(err: ApiError): string {
  switch (err.code) {
    case 'email_already_exists':
      return 'Email sudah terdaftar. Pilih email lain.';
    case 'weak_password':
      return 'Password terlalu lemah (minimal 8 karakter).';
    case 'invalid_role':
      return 'Role tidak dikenal.';
    case 'invalid_strategy':
      return 'Strategi password tidak dikenal.';
    case 'conflicting_password':
      return 'Strategi "Generate" tidak boleh diisi password manual.';
    default:
      return err.message || 'Gagal membuat akun.';
  }
}

function CopyButton({ value, label }: { value: string; label?: string }) {
  const [copied, setCopied] = React.useState(false);

  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // Some browsers reject writeText without secure context. Fallback:
      // create a hidden textarea and execCommand.
      const ta = document.createElement('textarea');
      ta.value = value;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      try {
        document.execCommand('copy');
        setCopied(true);
        window.setTimeout(() => setCopied(false), 2000);
      } finally {
        document.body.removeChild(ta);
      }
    }
  };

  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      onClick={onCopy}
      className="gap-2"
    >
      {copied ? (
        <>
          <ClipboardCheck className="size-4" />
          Tersalin
        </>
      ) : (
        <>
          <ClipboardCopy className="size-4" />
          {label ?? 'Salin'}
        </>
      )}
    </Button>
  );
}

export default function AdminCreateUserPage() {
  const router = useRouter();
  const { toast } = useToast();
  const [success, setSuccess] = React.useState<SuccessState | null>(null);

  const form = useForm<FormInput>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      email: '',
      role: 'guru',
      password_strategy: 'generate',
      password: '',
    },
  });

  const strategy = form.watch('password_strategy');

  const mutation = useMutation({
    mutationFn: (input: FormInput) =>
      api<CreateResponse>('/admin/users', {
        method: 'POST',
        body: {
          name: input.name,
          email: input.email,
          role: input.role,
          password_strategy: input.password_strategy,
          password:
            input.password_strategy === 'manual' ? input.password : undefined,
        },
      }),
    onSuccess: (data, variables) => {
      const password =
        variables.password_strategy === 'generate'
          ? data.generated_password ?? ''
          : variables.password;
      setSuccess({
        user: data.user,
        password,
        strategy: variables.password_strategy,
      });
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
        title: 'Gagal membuat akun',
        description: apiErr.requestId
          ? `${friendlyError(apiErr)} (req: ${apiErr.requestId})`
          : friendlyError(apiErr),
      });
    },
  });

  const onSubmit = (values: FormInput) => mutation.mutate(values);

  const onCreateAnother = () => {
    setSuccess(null);
    form.reset({
      name: '',
      email: '',
      role: 'guru',
      password_strategy: 'generate',
      password: '',
    });
  };

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <Link
            href="/admin/pengguna"
            className="inline-flex items-center gap-1 text-xs text-muted-foreground underline-offset-2 hover:underline"
          >
            <ArrowLeft className="size-3" />
            Kembali ke daftar pengguna
          </Link>
          <h1 className="text-2xl font-semibold tracking-tight">
            Tambah pengguna
          </h1>
          <p className="text-sm text-muted-foreground">
            Buat akun untuk admin, guru, atau siswa baru. Akun aktif langsung
            setelah dibuat dan wajib ganti password saat login pertama.
          </p>
        </div>
      </header>

      {success ? (
        <Card className="border-emerald-500/50 bg-emerald-500/5">
          <CardHeader>
            <CardTitle>Akun berhasil dibuat</CardTitle>
            <CardDescription>
              Berikan password berikut kepada pengguna. Password ini hanya
              ditampilkan sekali. Kalau modal ini tertutup tanpa disimpan,
              jalankan reset password dari halaman detail pengguna.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <dl className="grid gap-2 text-sm sm:grid-cols-[120px_1fr]">
              <dt className="text-muted-foreground">Nama</dt>
              <dd className="font-medium">{success.user.name}</dd>
              <dt className="text-muted-foreground">Email</dt>
              <dd className="break-all font-medium">{success.user.email}</dd>
              <dt className="text-muted-foreground">Role</dt>
              <dd>{roleLabel[success.user.role]}</dd>
            </dl>

            <div className="rounded-md border bg-background p-4">
              <Label className="text-xs uppercase tracking-wide text-muted-foreground">
                Password awal{' '}
                {success.strategy === 'generate'
                  ? '(di-generate oleh sistem)'
                  : '(diketik manual)'}
              </Label>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <code className="flex-1 break-all rounded bg-muted px-3 py-2 font-mono text-base">
                  {success.password || '—'}
                </code>
                {success.password ? (
                  <CopyButton value={success.password} label="Salin password" />
                ) : null}
                <CopyButton
                  value={`${success.user.email} / ${success.password}`}
                  label="Salin email + password"
                />
              </div>
              <p className="mt-3 flex items-start gap-2 text-xs text-amber-700 dark:text-amber-400">
                <ShieldAlert className="mt-0.5 size-4 shrink-0" />
                Pengguna wajib mengganti password ini saat login pertama.
              </p>
            </div>
          </CardContent>
          <CardFooter className="flex flex-wrap gap-2">
            <Button
              onClick={() => router.push(`/admin/pengguna/detail?id=${success.user.id}`)}
            >
              Buka detail
            </Button>
            <Button variant="outline" onClick={onCreateAnother}>
              Tambah lagi
            </Button>
            <Button variant="ghost" asChild>
              <Link href="/admin/pengguna">Selesai</Link>
            </Button>
          </CardFooter>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Form pengguna</CardTitle>
            <CardDescription>
              Email dipakai sebagai identitas login. Password awal akan diminta
              diganti saat login pertama.
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
                  name="name"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Nama lengkap</FormLabel>
                      <FormControl>
                        <Input
                          autoComplete="off"
                          disabled={mutation.isPending}
                          placeholder="mis. Budi Santoso"
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="email"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Email</FormLabel>
                      <FormControl>
                        <Input
                          type="email"
                          autoComplete="off"
                          disabled={mutation.isPending}
                          placeholder="nama@sekolah.id"
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="role"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Role</FormLabel>
                      <FormControl>
                        <select
                          className={selectClass}
                          disabled={mutation.isPending}
                          {...field}
                        >
                          <option value="guru">Guru</option>
                          <option value="siswa">Siswa</option>
                          <option value="admin">Administrator</option>
                        </select>
                      </FormControl>
                      <FormDescription>
                        Admin punya akses penuh termasuk mengelola pengguna lain.
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="password_strategy"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Password awal</FormLabel>
                      <div className="grid gap-2 sm:grid-cols-2">
                        <label
                          className={`flex cursor-pointer items-start gap-3 rounded-md border p-3 text-sm transition-colors ${
                            field.value === 'generate'
                              ? 'border-primary bg-primary/5'
                              : 'border-input'
                          }`}
                        >
                          <input
                            type="radio"
                            className="mt-1"
                            checked={field.value === 'generate'}
                            onChange={() => field.onChange('generate')}
                            disabled={mutation.isPending}
                          />
                          <div>
                            <div className="font-medium">Generate otomatis</div>
                            <div className="text-xs text-muted-foreground">
                              Sistem buat password 16 karakter acak.
                            </div>
                          </div>
                        </label>
                        <label
                          className={`flex cursor-pointer items-start gap-3 rounded-md border p-3 text-sm transition-colors ${
                            field.value === 'manual'
                              ? 'border-primary bg-primary/5'
                              : 'border-input'
                          }`}
                        >
                          <input
                            type="radio"
                            className="mt-1"
                            checked={field.value === 'manual'}
                            onChange={() => field.onChange('manual')}
                            disabled={mutation.isPending}
                          />
                          <div>
                            <div className="font-medium">Ketik manual</div>
                            <div className="text-xs text-muted-foreground">
                              Anda menentukan password awal sendiri.
                            </div>
                          </div>
                        </label>
                      </div>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {strategy === 'manual' ? (
                  <FormField
                    control={form.control}
                    name="password"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Password manual</FormLabel>
                        <FormControl>
                          <Input
                            type="text"
                            autoComplete="new-password"
                            disabled={mutation.isPending}
                            placeholder="minimal 8 karakter"
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          Disarankan kombinasi huruf besar, angka, dan simbol.
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                ) : null}

                <div className="flex flex-wrap items-center justify-end gap-2 pt-2">
                  <Button
                    type="button"
                    variant="ghost"
                    asChild
                    disabled={mutation.isPending}
                  >
                    <Link href="/admin/pengguna">Batal</Link>
                  </Button>
                  <Button type="submit" disabled={mutation.isPending}>
                    {mutation.isPending ? 'Menyimpan…' : 'Buat akun'}
                  </Button>
                </div>
              </form>
            </Form>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
