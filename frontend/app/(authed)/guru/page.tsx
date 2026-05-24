'use client';

/**
 * /guru — landing dashboard.
 *
 * For Phase 2.B.3 the only meaningful flow is "kelas". Show a compact
 * dashboard with quick links and a recent-kelas snapshot (top 3 by
 * created_at desc, hide archived). Real stat tiles wire up later.
 */

import * as React from 'react';
import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import {
  ArrowRight,
  ClipboardCheck,
  GraduationCap,
  ListChecks,
  ScrollText,
} from 'lucide-react';

import { listKelas } from '@/lib/kelas-api';
import { getPendingCounts, getPendingItems } from '@/lib/guru-api';
import { useAuthStore } from '@/lib/auth';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { GuruFeedList } from '@/components/guru/GuruFeedList';

export default function GuruDashboardPage() {
  const userName = useAuthStore((s) => s.user?.name ?? 'Guru');

  const recentKelas = useQuery({
    queryKey: ['guru', 'kelas', 'recent'],
    queryFn: () => listKelas({ page: 1, pageSize: 3, includeArchived: false }),
    staleTime: 15_000,
  });

  const pendingQ = useQuery({
    queryKey: ['guru', 'pending-counts'],
    queryFn: getPendingCounts,
    staleTime: 15_000,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  });

  const pendingItemsQ = useQuery({
    queryKey: ['guru', 'pending-items'],
    queryFn: () => getPendingItems(3),
    staleTime: 15_000,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  });

  const items = recentKelas.data?.items ?? [];
  const total = recentKelas.data?.total ?? 0;
  const ungraded = pendingQ.data?.ungraded_submissions ?? 0;
  const reviewUlangan = pendingQ.data?.pending_review_ulangan ?? 0;
  const reviewUjian = pendingQ.data?.pending_review_ujian ?? 0;
  const firstKelasID = items[0]?.id;
  const fallbackTugasHref = firstKelasID
    ? `/guru/kelas/detail?id=${firstKelasID}&tab=tugas`
    : '/guru/kelas';
  const fallbackUlanganHref = firstKelasID
    ? `/guru/kelas/detail?id=${firstKelasID}&tab=bab`
    : '/guru/kelas';
  const fallbackUjianHref = firstKelasID
    ? `/guru/kelas/detail?id=${firstKelasID}&tab=ujian`
    : '/guru/kelas';
  const tugasHref = pendingItemsQ.data?.ungraded_submissions?.[0]?.target_url ?? fallbackTugasHref;
  const ulanganHref = pendingItemsQ.data?.pending_review_ulangan?.[0]?.target_url ?? fallbackUlanganHref;
  const ujianHref = pendingItemsQ.data?.pending_review_ujian?.[0]?.target_url ?? fallbackUjianHref;

  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">
          Halo, {userName}!
        </h1>
        <p className="text-sm text-muted-foreground">
          Kelola kelas, materi, dan tugas dari sini. Statistik real akan
          tersambung di task berikutnya.
        </p>
      </header>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card className="flex flex-col">
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-base">Total Kelas Aktif</CardTitle>
              <CardDescription>Kelas yang sedang lu kelola.</CardDescription>
            </div>
            <GraduationCap className="size-5 text-muted-foreground" />
          </CardHeader>
          <CardContent className="mt-auto flex items-end justify-between gap-2">
            {recentKelas.isPending ? (
              <div className="h-7 w-12 animate-pulse rounded bg-muted" />
            ) : (
              <span className="text-2xl font-semibold">{total}</span>
            )}
            <Button asChild variant="ghost" size="sm">
              <Link href="/guru/kelas">
                Buka
                <ArrowRight className="size-4" />
              </Link>
            </Button>
          </CardContent>
        </Card>

        <Card className="flex flex-col">
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-base">Tugas perlu dinilai</CardTitle>
              <CardDescription>
                Submission tugas yang masih nunggu nilai.
              </CardDescription>
            </div>
            <ClipboardCheck className="size-5 text-muted-foreground" />
          </CardHeader>
          <CardContent className="mt-auto flex items-end justify-between gap-2">
            {pendingQ.isPending ? (
              <div className="h-7 w-12 animate-pulse rounded bg-muted" />
            ) : (
              <span
                className={
                  ungraded > 0
                    ? 'text-2xl font-semibold text-rose-600'
                    : 'text-2xl font-semibold'
                }
              >
                {ungraded}
              </span>
            )}
            <Button asChild variant="ghost" size="sm">
              <Link href={tugasHref}>
                Cek
                <ArrowRight className="size-4" />
              </Link>
            </Button>
          </CardContent>
        </Card>

        <Card className="flex flex-col">
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-base">Review ulangan bab</CardTitle>
              <CardDescription>
                Hasil ulangan bab siap dibuka untuk siswa.
              </CardDescription>
            </div>
            <ListChecks className="size-5 text-muted-foreground" />
          </CardHeader>
          <CardContent className="mt-auto flex items-end justify-between gap-2">
            {pendingQ.isPending ? (
              <div className="h-7 w-12 animate-pulse rounded bg-muted" />
            ) : (
              <span
                className={
                  reviewUlangan > 0
                    ? 'text-2xl font-semibold text-amber-600'
                    : 'text-2xl font-semibold'
                }
              >
                {reviewUlangan}
              </span>
            )}
            <Button asChild variant="ghost" size="sm">
              <Link href={ulanganHref}>
                Cek
                <ArrowRight className="size-4" />
              </Link>
            </Button>
          </CardContent>
        </Card>

        <Card className="flex flex-col">
          <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
            <div className="space-y-1">
              <CardTitle className="text-base">Review ujian</CardTitle>
              <CardDescription>
                Hasil ulangan harian / ujian yang siap dibuka.
              </CardDescription>
            </div>
            <ScrollText className="size-5 text-muted-foreground" />
          </CardHeader>
          <CardContent className="mt-auto flex items-end justify-between gap-2">
            {pendingQ.isPending ? (
              <div className="h-7 w-12 animate-pulse rounded bg-muted" />
            ) : (
              <span
                className={
                  reviewUjian > 0
                    ? 'text-2xl font-semibold text-amber-600'
                    : 'text-2xl font-semibold'
                }
              >
                {reviewUjian}
              </span>
            )}
            <Button asChild variant="ghost" size="sm">
              <Link href={ujianHref}>
                Cek
                <ArrowRight className="size-4" />
              </Link>
            </Button>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Kelas terbaru</CardTitle>
          <CardDescription>
            Tiga kelas yang baru lu buat. Klik untuk masuk halaman daftar
            lengkap.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {recentKelas.isPending ? (
            <div className="space-y-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <div
                  key={i}
                  className="h-12 animate-pulse rounded-md border bg-muted/40"
                />
              ))}
            </div>
          ) : items.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Belum ada kelas. Buat kelas pertama lu sekarang.
            </p>
          ) : (
            <ul className="divide-y">
              {items.map((k) => (
                <li
                  key={k.id}
                  className="flex items-center justify-between gap-3 py-3"
                >
                  <div className="min-w-0 space-y-0.5">
                    <p className="truncate text-sm font-medium">{k.nama}</p>
                    <p className="truncate text-xs text-muted-foreground">
                      Kode: <span className="font-mono">{k.kode_invite}</span>
                      {' · '}{k.jumlah_murid ?? 0} murid
                    </p>
                  </div>
                  <Button asChild variant="ghost" size="sm">
                    <Link href={`/guru/kelas/detail?id=${k.id}`}>Detail</Link>
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Aktivitas terbaru</CardTitle>
          <CardDescription>
            Submission tugas, ulangan harian selesai, dan siswa baru join.
            Auto-refresh tiap 30 detik.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <GuruFeedList />
        </CardContent>
      </Card>
    </div>
  );
}
