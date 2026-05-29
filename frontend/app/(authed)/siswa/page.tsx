'use client';

/**
 * /siswa — landing dashboard untuk siswa.
 *
 * Visual baseline: neo-brutalism + pastel pop (siswa-only theme). Stat cards
 * pakai SiswaStat, kelas list pakai SiswaCard berwarna deterministik per
 * kelas_id supaya wayfinding konsisten antar kunjungan.
 */

import * as React from 'react';
import Link from 'next/link';
import { useQuery } from '@tanstack/react-query';
import {
  ArrowRight,
  GraduationCap,
  MessageCircle,
  KeyRound,
  Sparkles,
  TrendingUp,
  Users,
} from 'lucide-react';

import { listMyKelas } from '@/lib/siswa-api';
import { listSiswaChatUnread } from '@/lib/siswa-chat-api';
import { useAuthStore } from '@/lib/auth';
import {
  SiswaBadge,
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardHeader,
  SiswaCardTitle,
  SiswaCardDescription,
  SiswaEmptyState,
  SiswaPageHeader,
  SiswaHighlight,
  SiswaStat,
  kelasToneFromId,
  SECTION_META,
} from '@/components/siswa-ui';

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
  const firstName = userName.split(/\s+/)[0] ?? userName;

  const myKelas = useQuery({
    queryKey: ['siswa', 'kelas', 'list'],
    queryFn: () => listMyKelas({ page: 1, pageSize: 50 }),
    staleTime: 15_000,
  });
  const unreadQ = useQuery({
    queryKey: ['siswa', 'chat', 'unread'],
    queryFn: listSiswaChatUnread,
    refetchInterval: 30_000,
    staleTime: 10_000,
  });

  const items = myKelas.data?.items ?? [];
  const unreadByKelas = React.useMemo(() => {
    const map = new Map<string, number>();
    for (const row of unreadQ.data ?? []) map.set(row.kelas_id, row.unread);
    return map;
  }, [unreadQ.data]);

  return (
    <div className="space-y-8">
      <SiswaPageHeader
        eyebrow="Dashboard"
        title={
          <>
            Halo, <SiswaHighlight>{firstName}</SiswaHighlight>!
          </>
        }
        description="Daftar kelas yang kamu ikuti. Gabung kelas baru pakai kode invite dari guru."
        actions={
          <>
            <SiswaButton asChild tone="surface" size="sm">
              <Link href="/siswa/nilai">
                <TrendingUp className="size-4" />
                Nilai saya
              </Link>
            </SiswaButton>
            <SiswaButton asChild tone="primary" size="sm">
              <Link href="/siswa/gabung">
                <KeyRound className="size-4" />
                Gabung Kelas
              </Link>
            </SiswaButton>
          </>
        }
      />

      <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <SiswaStat
          label="Kelas Diikuti"
          value={items.length}
          hint="Enrollment aktif"
          Icon={GraduationCap}
          tone="materi"
          loading={myKelas.isPending}
        />
        <SiswaStat
          label="Total Tugas"
          value={
            <Link
              href="/siswa/tugas"
              className="underline-offset-4 hover:underline"
            >
              Lihat
            </Link>
          }
          hint="Buka halaman riwayat tugas"
          Icon={Sparkles}
          tone="tugas"
        />
        <SiswaStat
          label="Ujian Aktif"
          value={
            <Link
              href="/siswa/ujian"
              className="underline-offset-4 hover:underline"
            >
              Lihat
            </Link>
          }
          hint="Cek lobby ulangan"
          Icon={Users}
          tone="ulangan"
        />
      </section>

      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>Kelas Saya</SiswaCardTitle>
          <SiswaCardDescription>
            Klik kelas untuk lihat materi, latihan, ulangan, dan tugas.
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          {myKelas.isPending ? (
            <div className="grid gap-3 sm:grid-cols-2">
              {Array.from({ length: 4 }).map((_, i) => (
                <div
                  key={i}
                  className="h-28 animate-pulse rounded-siswa border-2 border-siswa-border-soft bg-siswa-cream/40"
                />
              ))}
            </div>
          ) : items.length === 0 ? (
            <SiswaEmptyState
              icon="📚"
              title="Belum ada kelas"
              description={
                <>
                  Kamu belum gabung kelas apapun. Minta kode invite ke guru atau
                  tunggu admin meng-assign kamu ke kelas.
                </>
              }
              action={
                <SiswaButton asChild>
                  <Link href="/siswa/gabung">
                    <KeyRound className="size-4" />
                    Gabung Kelas
                  </Link>
                </SiswaButton>
              }
            />
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {items.map((it) => {
                const tone = kelasToneFromId(it.kelas.id);
                const sectionMeta = SECTION_META[tone];
                const SectionIcon = sectionMeta.Icon;
                const unread = unreadByKelas.get(it.kelas.id) ?? 0;
                return (
                  <SiswaCard
                    key={it.kelas.id}
                    tone={tone}
                    shadow="md"
                    interactive
                    asButton
                    onClick={() => {
                      window.location.href = `/siswa/kelas/detail?id=${it.kelas.id}`;
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        window.location.href = `/siswa/kelas/detail?id=${it.kelas.id}`;
                      }
                    }}
                    className="overflow-hidden"
                  >
                    <div className="flex items-start justify-between gap-3 border-b-2 border-siswa-border bg-siswa-surface/70 px-5 py-3">
                      <span className="grid size-10 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                        <SectionIcon className="size-5" strokeWidth={2.5} />
                      </span>
                      <div className="flex flex-col items-end gap-2">
                        {unread > 0 ? (
                          <SiswaBadge tone="pink">
                            <MessageCircle className="size-3" />
                            {unread} baru
                          </SiswaBadge>
                        ) : null}
                        <SiswaBadge tone="cream">
                          {it.joined_via === 'kode' ? 'kode invite' : 'admin'}
                        </SiswaBadge>
                      </div>
                    </div>
                    <div className="space-y-2 p-5">
                      <h3 className="siswa-display line-clamp-2 text-lg font-bold leading-tight">
                        {it.kelas.nama}
                      </h3>
                      <p className="text-xs text-siswa-text-muted">
                        Bergabung {formatDate(it.joined_at)}
                      </p>
                      <div className="flex items-center justify-between pt-1 text-sm font-semibold">
                        <span>Buka kelas</span>
                        <ArrowRight className="size-4" strokeWidth={2.5} />
                      </div>
                    </div>
                  </SiswaCard>
                );
              })}
            </div>
          )}
        </SiswaCardBody>
      </SiswaCard>
    </div>
  );
}
