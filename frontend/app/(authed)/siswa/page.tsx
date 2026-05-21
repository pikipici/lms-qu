'use client';

/**
 * /siswa — landing dashboard untuk siswa.
 *
 * List kelas yang sudah di-join (active enrollment only — kelas archived /
 * removed enrollment hidden by backend ListMyKelas). Header CTA langsung
 * ke /siswa/gabung untuk gabung kelas baru via kode invite.
 */

import * as React from 'react';
import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import { ArrowRight, GraduationCap, KeyRound, Users } from 'lucide-react';

import { listMyKelas } from '@/lib/siswa-api';
import { useAuthStore } from '@/lib/auth';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString('id-ID', {
      day: 'numeric',
      month: 'short',
      year: 'numeric',
    });
  } catch {
    return iso;
  }
}

export default function SiswaDashboardPage() {
  const userName = useAuthStore((s) => s.user?.name ?? 'Siswa');

  const myKelas = useQuery({
    queryKey: ['siswa', 'kelas', 'list'],
    queryFn: () => listMyKelas({ page: 1, pageSize: 50 }),
    staleTime: 15_000,
  });

  const items = myKelas.data?.items ?? [];

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">
            Halo, {userName}!
          </h1>
          <p className="text-sm text-muted-foreground">
            Daftar kelas yang lu ikuti. Gabung kelas baru pakai kode invite
            dari guru.
          </p>
        </div>
        <Button asChild size="sm">
          <Link href="/siswa/gabung">
            <KeyRound className="size-4" />
            Gabung Kelas
          </Link>
        </Button>
      </header>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <Card className="flex flex-col">
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-base">Kelas Diikuti</CardTitle>
              <CardDescription>Total enrollment aktif lu.</CardDescription>
            </div>
            <GraduationCap className="size-5 text-muted-foreground" />
          </CardHeader>
          <CardContent className="mt-auto flex items-end justify-between gap-2">
            {myKelas.isPending ? (
              <div className="h-7 w-12 animate-pulse rounded bg-muted" />
            ) : (
              <span className="text-2xl font-semibold">{items.length}</span>
            )}
          </CardContent>
        </Card>

        <Card className="flex flex-col sm:col-span-2">
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-base">Punya kode invite?</CardTitle>
              <CardDescription>
                Masukin kode 6 karakter yang dikasih guru untuk langsung gabung
                kelas. Format huruf besar/kecil bebas — sistem auto-normalisasi.
              </CardDescription>
            </div>
            <Users className="size-5 text-muted-foreground" />
          </CardHeader>
          <CardContent className="mt-auto">
            <Button asChild size="sm" variant="outline">
              <Link href="/siswa/gabung">
                <KeyRound className="size-4" />
                Gabung pakai kode
              </Link>
            </Button>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Kelas saya</CardTitle>
          <CardDescription>
            Klik salah satu kelas untuk lihat materi, tugas, dan ulangan
            (segera tersedia di fase berikutnya).
          </CardDescription>
        </CardHeader>
        <CardContent>
          {myKelas.isPending ? (
            <div className="space-y-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <div
                  key={i}
                  className="h-14 animate-pulse rounded-md border bg-muted/40"
                />
              ))}
            </div>
          ) : items.length === 0 ? (
            <div className="rounded-md border border-dashed p-6 text-center">
              <p className="text-sm text-muted-foreground">
                Lu belum gabung kelas apapun. Minta kode invite ke guru atau
                tunggu admin meng-assign lu.
              </p>
              <Button asChild size="sm" className="mt-3">
                <Link href="/siswa/gabung">
                  <KeyRound className="size-4" />
                  Gabung Kelas
                </Link>
              </Button>
            </div>
          ) : (
            <ul className="divide-y">
              {items.map((it) => (
                <li
                  key={it.kelas.id}
                  className="flex items-center justify-between gap-3 py-3"
                >
                  <div className="min-w-0 space-y-0.5">
                    <p className="truncate text-sm font-medium">
                      {it.kelas.nama}
                    </p>
                    <p className="truncate text-xs text-muted-foreground">
                      Gabung {formatDate(it.joined_at)} via{' '}
                      <span className="font-medium">
                        {it.joined_via === 'kode' ? 'kode invite' : 'admin'}
                      </span>
                      {' · '}Bobot {it.kelas.bobot_soal_ulangan}/
                      {it.kelas.bobot_tugas}
                    </p>
                  </div>
                  <Button asChild variant="ghost" size="sm">
                    <Link href={`/siswa/kelas/detail?id=${it.kelas.id}`}>
                      Buka
                      <ArrowRight className="size-4" />
                    </Link>
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
