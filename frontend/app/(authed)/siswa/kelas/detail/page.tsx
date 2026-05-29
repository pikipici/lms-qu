'use client';

/**
 * /siswa/kelas/detail?id=:id — siswa kelas detail page (Task 3.E.2).
 *
 * Static export: pakai query string. Mirror /guru/kelas/detail pattern.
 *
 * Layout:
 *   - Header card berwarna deterministik per kelas_id (kelasToneFromId).
 *   - Section "Bab kelas" (materi accent, biru).
 *   - Section "Tugas kelas" (tugas accent, pink).
 *   - Section "Pengumuman" (umum accent, krem).
 */

import * as React from 'react';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  ArrowLeft,
  ArrowRight,
  BookOpen,
  AlertCircle,
  Calendar,
  ClipboardList,
  GraduationCap,
  Megaphone,
  MessageCircle,
  RotateCcw,
  Send,
  TrendingUp,
  type LucideIcon,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import { listMyKelas, type MyKelasItem } from '@/lib/siswa-api';
import {
  listSiswaBab,
  type SiswaBabItem,
  type SiswaBabListResponse,
} from '@/lib/siswa-bab-api';
import {
  getSiswaChat,
  markSiswaChatRead,
  sendSiswaChatMessage,
  type SiswaChatPayload,
} from '@/lib/siswa-chat-api';
import {
  listSiswaUjianByKelas,
  listSiswaUjianHasil,
  type SiswaUjianHasilListResult,
  type Ujian,
} from '@/lib/siswa-ujian-api';
import { Button } from '@/components/ui/button';
import { SiswaBabProgressBar } from '@/components/siswa/SiswaBabProgressBar';
import { PengumumanReadList } from '@/components/pengumuman/PengumumanReadList';
import { SiswaTugasList } from '@/components/submission/SiswaTugasList';
import { UjianLobbyCard } from '@/components/siswa-ujian/UjianLobbyCard';
import {
  SECTION_META,
  SiswaBadge,
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardDescription,
  SiswaCardHeader,
  SiswaCardTitle,
  kelasToneFromId,
} from '@/components/siswa-ui';

function formatDate(input?: string | null): string {
  if (!input) return '—';
  try {
    return new Date(input).toLocaleString('id-ID', {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: 'Asia/Jakarta',
    });
  } catch {
    return input;
  }
}

function joinedViaLabel(via: 'kode' | 'admin'): string {
  return via === 'kode' ? 'kode invite' : 'admin';
}

type DetailTab = 'materi' | 'tugas' | 'ujian' | 'pengumuman' | 'chat' | 'nilai';

const DETAIL_TABS: {
  key: DetailTab;
  label: string;
  description: string;
  Icon: LucideIcon;
}[] = [
  { key: 'materi', label: 'Materi', description: 'Bab, materi, latihan, dan ulangan.', Icon: BookOpen },
  { key: 'tugas', label: 'Tugas', description: 'Tugas kelas-wide dari guru.', Icon: ClipboardList },
  { key: 'ujian', label: 'Ujian', description: 'Ujian khusus kelas ini.', Icon: GraduationCap },
  { key: 'pengumuman', label: 'Pengumuman', description: 'Info terbaru dari guru.', Icon: Megaphone },
  { key: 'chat', label: 'Chat', description: 'Tanya guru kelas langsung.', Icon: MessageCircle },
  { key: 'nilai', label: 'Nilai', description: 'Rekap nilai kelas ini.', Icon: TrendingUp },
];

function isDetailTab(value: string | null): value is DetailTab {
  return DETAIL_TABS.some((tab) => tab.key === value);
}

function BabCard({ kelasID, bab }: { kelasID: string; bab: SiswaBabItem }) {
  const href = `/siswa/kelas/detail/bab?id=${kelasID}&bid=${bab.id}`;
  return (
    <Link
      href={href}
      className="block rounded-siswa siswa-border bg-siswa-surface siswa-press p-4 focus-visible:outline-none"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex items-center gap-2">
            <SiswaBadge tone="blue">Bab {bab.nomor}</SiswaBadge>
            <h3 className="siswa-display truncate text-base font-bold leading-tight">
              {bab.judul}
            </h3>
          </div>
          {bab.deskripsi ? (
            <p className="line-clamp-2 text-sm text-siswa-text-muted">
              {bab.deskripsi}
            </p>
          ) : null}
          <SiswaBabProgressBar
            persen={bab.progress.persen}
            materiRead={bab.progress.materi_read}
            materiTotal={bab.progress.materi_total}
            babKosong={bab.progress.bab_kosong}
            size="sm"
            variant="siswa"
          />
        </div>
        <ArrowRight
          className="mt-1 size-4 shrink-0 text-siswa-text-muted"
          strokeWidth={2.5}
        />
      </div>
    </Link>
  );
}

function SiswaChatBox({ kelasID }: { kelasID: string }) {
  const queryClient = useQueryClient();
  const [body, setBody] = React.useState('');
  const chatQuery = useQuery({
    queryKey: ['siswa', 'kelas', 'chat', kelasID],
    queryFn: () => getSiswaChat(kelasID),
    refetchInterval: 12_000,
    staleTime: 5_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && [400, 403, 404].includes(err.status)) {
        return false;
      }
      return failureCount < 2;
    },
  });

  React.useEffect(() => {
    if (chatQuery.data?.conversation.siswa_unread_count) {
      void markSiswaChatRead(kelasID).then(() => {
        queryClient.setQueryData<SiswaChatPayload>(
          ['siswa', 'kelas', 'chat', kelasID],
          (old) => old
            ? {
                ...old,
                conversation: { ...old.conversation, siswa_unread_count: 0 },
              }
            : old,
        );
      }).catch(() => undefined);
    }
  }, [chatQuery.data?.conversation.siswa_unread_count, kelasID, queryClient]);

  const sendMutation = useMutation({
    mutationFn: (text: string) => sendSiswaChatMessage(kelasID, text),
    onSuccess: () => {
      setBody('');
      void queryClient.invalidateQueries({
        queryKey: ['siswa', 'kelas', 'chat', kelasID],
      });
    },
  });

  const messages = chatQuery.data?.messages ?? [];
  const disabled = sendMutation.isPending || body.trim().length === 0;

  return (
    <SiswaCard tone="umum" shadow="md">
      <SiswaCardHeader>
        <div className="flex flex-row items-start justify-between gap-3">
          <div className="space-y-1">
            <SiswaCardTitle className="flex items-center gap-2">
              <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                <MessageCircle className="size-4" strokeWidth={2.5} />
              </span>
              Chat guru
            </SiswaCardTitle>
            <SiswaCardDescription>
              Tanya materi, tugas, atau ulangan di kelas ini. Chat ini kebaca guru
              dan admin sekolah.
            </SiswaCardDescription>
          </div>
          <SiswaButton
            type="button"
            tone="surface"
            size="sm"
            onClick={() => chatQuery.refetch()}
            disabled={chatQuery.isFetching}
          >
            <RotateCcw className="size-4" />
            Refresh
          </SiswaButton>
        </div>
      </SiswaCardHeader>
      <SiswaCardBody className="space-y-4">
        {chatQuery.isError ? (
          <div className="rounded-siswa border-2 border-siswa-danger bg-siswa-surface p-4 text-sm">
            <p className="font-bold">Gagal memuat chat</p>
            <p className="text-siswa-text-muted">
              {chatQuery.error instanceof ApiError
                ? chatQuery.error.message
                : 'Terjadi kesalahan tidak terduga.'}
            </p>
          </div>
        ) : (
          <div className="max-h-[420px] space-y-3 overflow-y-auto rounded-siswa siswa-border bg-siswa-surface/70 p-3">
            {chatQuery.isPending ? (
              <div className="space-y-2">
                {[0, 1, 2].map((i) => (
                  <div key={i} className="h-14 animate-pulse rounded-siswa bg-siswa-text/10" />
                ))}
              </div>
            ) : messages.length === 0 ? (
              <div className="rounded-siswa border-2 border-dashed border-siswa-border-soft bg-white/50 p-6 text-center text-sm text-siswa-text-muted">
                Belum ada pesan. Mulai tanya guru kamu di sini.
              </div>
            ) : (
              messages.map((msg) => {
                const mine = msg.sender_role === 'siswa';
                return (
                  <div key={msg.id} className={`flex ${mine ? 'justify-end' : 'justify-start'}`}>
                    <div
                      className={`max-w-[82%] rounded-siswa siswa-border px-3 py-2 text-sm siswa-shadow-sm ${
                        mine ? 'bg-siswa-primary text-siswa-text' : 'bg-white text-siswa-text'
                      }`}
                    >
                      <div className="mb-1 flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.14em] text-siswa-text-muted">
                        <span>{mine ? 'Kamu' : msg.sender_name || msg.sender_role}</span>
                        <span>{formatDate(msg.created_at)}</span>
                      </div>
                      <p className="whitespace-pre-wrap leading-relaxed">{msg.body}</p>
                    </div>
                  </div>
                );
              })
            )}
          </div>
        )}

        <form
          className="space-y-2"
          onSubmit={(e) => {
            e.preventDefault();
            const text = body.trim();
            if (text) sendMutation.mutate(text);
          }}
        >
          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            maxLength={4000}
            rows={3}
            placeholder="Tulis pesan ke guru..."
            className="min-h-24 w-full rounded-siswa siswa-border bg-siswa-surface px-4 py-3 text-sm outline-none focus:ring-2 focus:ring-siswa-border-soft"
          />
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-xs text-siswa-text-muted">{body.trim().length}/4000 karakter</p>
            <SiswaButton type="submit" tone="primary" size="sm" disabled={disabled}>
              <Send className="size-4" />
              {sendMutation.isPending ? 'Mengirim...' : 'Kirim pesan'}
            </SiswaButton>
          </div>
          {sendMutation.isError ? (
            <p className="text-sm font-semibold text-siswa-danger">
              {sendMutation.error instanceof ApiError
                ? sendMutation.error.message
                : 'Pesan gagal dikirim.'}
            </p>
          ) : null}
        </form>
      </SiswaCardBody>
    </SiswaCard>
  );
}

function SiswaUjianKelasTab({ kelasID, kelasName }: { kelasID: string; kelasName: string }) {
  const ujianQuery = useQuery({
    queryKey: ['siswa', 'ujian', 'list', kelasID],
    queryFn: () => listSiswaUjianByKelas(kelasID, { limit: 100 }),
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && [400, 403, 404].includes(err.status)) {
        return false;
      }
      return failureCount < 2;
    },
  });

  const hasilQuery = useQuery({
    queryKey: ['siswa', 'ujian', 'hasil', kelasID],
    queryFn: () => listSiswaUjianHasil(kelasID),
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && [400, 403, 404].includes(err.status)) {
        return false;
      }
      return failureCount < 2;
    },
  });

  const isLoading = ujianQuery.isPending || hasilQuery.isPending;
  const isError = ujianQuery.isError || hasilQuery.isError;
  const ujianItems = React.useMemo<Ujian[]>(() => {
    const now = Date.now();
    return [...(ujianQuery.data?.items ?? [])].sort((a, b) => ujianSortKey(a, now) - ujianSortKey(b, now));
  }, [ujianQuery.data?.items]);
  const hasilAggregate = hasilQuery.data?.hasil;

  return (
    <SiswaCard tone="ulangan" shadow="md">
      <SiswaCardHeader>
        <div className="flex flex-row items-start justify-between gap-3">
          <div className="space-y-1">
            <SiswaCardTitle className="flex items-center gap-2">
              <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                <GraduationCap className="size-4" strokeWidth={2.5} />
              </span>
              Ujian kelas
            </SiswaCardTitle>
            <SiswaCardDescription>
              Ujian yang dipublish guru untuk kelas ini. Mulai, lanjutkan, atau cek pembahasan dari sini.
            </SiswaCardDescription>
          </div>
          <SiswaButton
            type="button"
            tone="surface"
            size="sm"
            onClick={() => {
              void ujianQuery.refetch();
              void hasilQuery.refetch();
            }}
            disabled={ujianQuery.isFetching || hasilQuery.isFetching}
          >
            <RotateCcw className="size-4" />
            Refresh
          </SiswaButton>
        </div>
      </SiswaCardHeader>
      <SiswaCardBody>
        {isLoading ? (
          <div className="space-y-3">
            {[0, 1].map((i) => (
              <div
                key={i}
                className="h-48 animate-pulse rounded-siswa siswa-border bg-siswa-surface/60"
              />
            ))}
          </div>
        ) : isError ? (
          <SiswaUjianErrorState
            error={ujianQuery.error ?? hasilQuery.error}
            onRetry={() => {
              void ujianQuery.refetch();
              void hasilQuery.refetch();
            }}
            fetching={ujianQuery.isFetching || hasilQuery.isFetching}
          />
        ) : ujianItems.length === 0 ? (
          <div className="rounded-siswa border-2 border-dashed border-siswa-border-soft bg-siswa-surface/60 p-8 text-center">
            <GraduationCap className="mx-auto mb-2 size-8 text-siswa-text-muted" strokeWidth={2.5} />
            <p className="text-sm text-siswa-text-muted">
              Belum ada ujian yang dipublish di kelas ini. Nanti kalau guru sudah publish, muncul di tab ini.
            </p>
          </div>
        ) : (
          <div className="space-y-4">
            {ujianItems.map((ujian) => (
              <UjianLobbyCard
                key={ujian.id}
                ujian={ujian}
                kelasName={kelasName}
                hasilAggregate={hasilAggregate as SiswaUjianHasilListResult | undefined}
              />
            ))}
          </div>
        )}

        {isError && ujianItems.length > 0 ? (
          <p className="mt-3 flex items-center gap-2 text-xs text-siswa-text-muted">
            <AlertCircle className="size-3.5 text-siswa-warning" />
            Sebagian data ujian gagal di-fetch. Coba refresh kalau status attempt belum update.
          </p>
        ) : null}
      </SiswaCardBody>
    </SiswaCard>
  );
}

function SiswaUjianErrorState({
  error,
  onRetry,
  fetching,
}: {
  error: Error | null;
  onRetry: () => void;
  fetching: boolean;
}) {
  const apiErr = error instanceof ApiError ? error : null;
  return (
    <div className="space-y-3 rounded-siswa border-2 border-siswa-danger bg-siswa-surface p-4 text-sm">
      <p className="font-bold">Gagal memuat ujian</p>
      <p className="text-siswa-text-muted">
        {apiErr?.message ?? error?.message ?? 'Terjadi kesalahan tidak terduga.'}
        {apiErr?.requestId ? ` (req: ${apiErr.requestId})` : ''}
      </p>
      <SiswaButton type="button" tone="surface" size="sm" onClick={onRetry} disabled={fetching}>
        <RotateCcw className="size-4" />
        Coba lagi
      </SiswaButton>
    </div>
  );
}

function ujianSortKey(ujian: Ujian, now: number): number {
  const startMs = ujian.waktu_mulai ? new Date(ujian.waktu_mulai).getTime() : null;
  const endMs = ujian.waktu_selesai ? new Date(ujian.waktu_selesai).getTime() : null;
  if (startMs && now < startMs) return 100_000_000 + (startMs - now);
  if (endMs && now > endMs) return 1_000_000_000 + (now - endMs);
  if (endMs) return endMs - now;
  return 50_000_000;
}

function DetailTabBar({
  active,
  onChange,
}: {
  active: DetailTab;
  onChange: (tab: DetailTab) => void;
}) {
  return (
    <div>
      <div className="grid grid-cols-2 gap-2 rounded-siswa siswa-border bg-siswa-surface/80 p-2 siswa-shadow-sm sm:flex">
        {DETAIL_TABS.map((tab) => {
          const activeTab = tab.key === active;
          const Icon = tab.Icon;
          return (
            <button
              key={tab.key}
              type="button"
              onClick={() => onChange(tab.key)}
              className={`flex min-w-0 items-center justify-center gap-2 rounded-siswa border-2 px-2 py-2 text-xs font-bold transition-transform focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-siswa-border sm:px-4 sm:text-sm ${
                activeTab
                  ? 'border-siswa-border bg-siswa-primary text-siswa-text siswa-shadow-sm -translate-y-0.5'
                  : 'border-siswa-border bg-siswa-surface text-siswa-text hover:-translate-y-0.5 hover:bg-siswa-cream/80'
              }`}
            >
              <Icon className="size-4" strokeWidth={2.5} />
              {tab.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}

function SiswaKelasDetailContent({ kelasID, initialTab }: { kelasID: string; initialTab: DetailTab }) {
  const router = useRouter();
  const [activeTab, setActiveTab] = React.useState<DetailTab>(initialTab);
  const setTab = React.useCallback((next: DetailTab) => {
    setActiveTab(next);
    router.replace(`/siswa/kelas/detail?id=${kelasID}&tab=${next}`, { scroll: false });
  }, [kelasID, router]);

  React.useEffect(() => {
    setActiveTab(initialTab);
  }, [initialTab]);

  const enrollmentQuery = useQuery({
    queryKey: ['siswa', 'kelas', 'list'],
    queryFn: () => listMyKelas({ page: 1, pageSize: 50 }),
    staleTime: 30_000,
  });

  const babQuery = useQuery({
    queryKey: ['siswa', 'kelas', 'bab', kelasID],
    queryFn: () => listSiswaBab(kelasID),
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError) {
        if (err.status === 403 || err.status === 404 || err.status === 400) {
          return false;
        }
      }
      return failureCount < 2;
    },
  });

  const enrollment: MyKelasItem | undefined = React.useMemo(() => {
    return enrollmentQuery.data?.items.find((it) => it.kelas.id === kelasID);
  }, [enrollmentQuery.data, kelasID]);

  if (enrollmentQuery.isPending) {
    return (
      <div className="space-y-4">
        <div className="h-6 w-32 animate-pulse rounded bg-siswa-text/10" />
        <div className="h-32 animate-pulse rounded-siswa siswa-border bg-siswa-surface" />
        <div className="h-64 animate-pulse rounded-siswa siswa-border bg-siswa-surface" />
      </div>
    );
  }

  if (enrollmentQuery.isError) {
    const err = enrollmentQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>Gagal memuat kelas</SiswaCardTitle>
          <SiswaCardDescription>
            {apiErr?.message ?? 'Terjadi kesalahan tidak terduga.'}
            {apiErr?.requestId ? ` (req: ${apiErr.requestId})` : ''}
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody className="flex flex-wrap gap-3">
          <SiswaButton
            type="button"
            tone="primary"
            size="sm"
            onClick={() => enrollmentQuery.refetch()}
            disabled={enrollmentQuery.isFetching}
          >
            <RotateCcw className="size-4" />
            Coba lagi
          </SiswaButton>
          <SiswaButton asChild tone="surface" size="sm">
            <Link href="/siswa">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </SiswaButton>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  if (!enrollment) {
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>Kelas tidak ditemukan</SiswaCardTitle>
          <SiswaCardDescription>
            Kamu belum gabung kelas ini, atau ID kelas tidak valid. Gabung
            kelas baru pakai kode invite di /siswa/gabung.
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          <SiswaButton asChild tone="surface" size="sm">
            <Link href="/siswa">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </SiswaButton>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  const kelas = enrollment.kelas;
  const archived = Boolean(kelas.archived_at);
  const tone = kelasToneFromId(kelas.id);
  const meta = SECTION_META[tone];
  const KelasIcon = meta.Icon;

  return (
    <div className="space-y-6">
      <Button asChild variant="ghost" size="sm" className="-ml-2 text-siswa-text">
        <Link href="/siswa">
          <ArrowLeft className="size-4" />
          Daftar kelas
        </Link>
      </Button>

      {/* Header card berwarna deterministik */}
      <SiswaCard tone={tone} shadow="lg" className="overflow-hidden">
        <div className="flex items-start gap-4 border-b-2 border-siswa-border bg-siswa-surface/70 px-6 py-5">
          <span className="grid size-12 shrink-0 place-items-center rounded-siswa siswa-border bg-siswa-surface siswa-shadow-sm">
            <KelasIcon className="size-6" strokeWidth={2.5} />
          </span>
          <div className="min-w-0 flex-1 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs font-semibold uppercase tracking-[0.18em] text-siswa-text-muted">
                Kelas
              </span>
              {archived ? (
                <SiswaBadge tone="cream">Diarsipkan</SiswaBadge>
              ) : null}
              <SiswaBadge tone="neutral">
                via {joinedViaLabel(enrollment.joined_via)}
              </SiswaBadge>
            </div>
            <h1 className="siswa-display text-2xl font-bold leading-tight sm:text-3xl">
              {kelas.nama}
            </h1>
            <p className="flex items-center gap-1.5 text-xs text-siswa-text-muted">
              <Calendar className="size-3" />
              Bergabung {formatDate(enrollment.joined_at)}
            </p>
          </div>
        </div>
        <div className="space-y-3 px-6 py-5">
          {kelas.deskripsi ? (
            <p className="text-sm text-siswa-text">{kelas.deskripsi}</p>
          ) : (
            <p className="text-sm italic text-siswa-text-muted">
              Belum ada deskripsi dari guru.
            </p>
          )}
          <div className="flex flex-wrap gap-2 pt-1">
            <SiswaButton asChild tone="surface" size="sm">
              <Link href={`/siswa/kelas/detail/nilai?id=${kelasID}`}>
                <TrendingUp className="size-4" strokeWidth={2.5} />
                Lihat nilai kelas ini
              </Link>
            </SiswaButton>
          </div>
        </div>
      </SiswaCard>

      <DetailTabBar active={activeTab} onChange={setTab} />

      {activeTab === 'materi' ? (
        <SiswaCard tone="materi" shadow="md">
          <SiswaCardHeader>
            <div className="flex flex-row items-start justify-between gap-3">
              <div className="space-y-1">
                <SiswaCardTitle className="flex items-center gap-2">
                  <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                    <BookOpen className="size-4" strokeWidth={2.5} />
                  </span>
                  Materi kelas
                </SiswaCardTitle>
                <SiswaCardDescription>
                  Klik bab buat lihat materi, latihan, ulangan, dan tugas. Progress bar nge-track materi yang udah kamu baca.
                </SiswaCardDescription>
              </div>
              <SiswaButton type="button" tone="surface" size="sm" onClick={() => babQuery.refetch()} disabled={babQuery.isFetching}>
                <RotateCcw className="size-4" />
                Refresh
              </SiswaButton>
            </div>
          </SiswaCardHeader>
          <SiswaCardBody>
            <BabListBody kelasID={kelasID} query={babQuery} />
          </SiswaCardBody>
        </SiswaCard>
      ) : null}

      {activeTab === 'tugas' ? (
        <SiswaCard tone="tugas" shadow="md">
          <SiswaCardHeader>
            <SiswaCardTitle className="flex items-center gap-2">
              <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                <ClipboardList className="size-4" strokeWidth={2.5} />
              </span>
              Tugas kelas
            </SiswaCardTitle>
            <SiswaCardDescription>
              Tugas kelas-wide dari guru. Tugas spesifik bab tersedia di halaman bab masing-masing.
            </SiswaCardDescription>
          </SiswaCardHeader>
          <SiswaCardBody>
            <SiswaTugasList kelasID={kelasID} babID={null} emptyState="Belum ada tugas kelas-wide." />
          </SiswaCardBody>
        </SiswaCard>
      ) : null}

      {activeTab === 'ujian' ? (
        <SiswaUjianKelasTab kelasID={kelasID} kelasName={kelas.nama} />
      ) : null}

      {activeTab === 'pengumuman' ? (
        <SiswaCard tone="umum" shadow="md">
          <SiswaCardHeader>
            <SiswaCardTitle className="flex items-center gap-2">
              <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                <Megaphone className="size-4" strokeWidth={2.5} />
              </span>
              Pengumuman kelas
            </SiswaCardTitle>
            <SiswaCardDescription>
              Update terbaru dari guru. Pengumuman bab tersedia di halaman bab masing-masing.
            </SiswaCardDescription>
          </SiswaCardHeader>
          <SiswaCardBody>
            <PengumumanReadList kelasID={kelasID} babID={null} emptyState="Belum ada pengumuman dari guru." expandFirst />
          </SiswaCardBody>
        </SiswaCard>
      ) : null}

      {activeTab === 'chat' ? <SiswaChatBox kelasID={kelasID} /> : null}

      {activeTab === 'nilai' ? (
        <SiswaCard tone="nilai" shadow="md">
          <SiswaCardHeader>
            <SiswaCardTitle className="flex items-center gap-2">
              <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                <TrendingUp className="size-4" strokeWidth={2.5} />
              </span>
              Nilai kelas
            </SiswaCardTitle>
            <SiswaCardDescription>
              Buka halaman rekap nilai lengkap untuk kelas ini.
            </SiswaCardDescription>
          </SiswaCardHeader>
          <SiswaCardBody>
            <SiswaButton asChild tone="primary" size="sm">
              <Link href={`/siswa/kelas/detail/nilai?id=${kelasID}`}>
                <TrendingUp className="size-4" strokeWidth={2.5} />
                Lihat nilai kelas ini
              </Link>
            </SiswaButton>
          </SiswaCardBody>
        </SiswaCard>
      ) : null}
    </div>
  );
}

function BabListBody({
  kelasID,
  query,
}: {
  kelasID: string;
  query: UseQueryResult<SiswaBabListResponse, Error>;
}) {
  if (query.isPending) {
    return (
      <div className="space-y-2">
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            className="h-24 animate-pulse rounded-siswa siswa-border bg-siswa-surface/50"
          />
        ))}
      </div>
    );
  }

  if (query.isError) {
    const err = query.error;
    const apiErr = err instanceof ApiError ? err : null;
    const isForbidden = apiErr?.code === 'forbidden';
    return (
      <div className="space-y-3 rounded-siswa border-2 border-siswa-danger bg-siswa-surface p-4 text-sm">
        <p className="font-bold">
          {isForbidden ? 'Akses ditolak' : 'Gagal memuat bab'}
        </p>
        <p className="text-siswa-text-muted">
          {isForbidden
            ? 'Kamu tidak terdaftar aktif di kelas ini. Hubungi guru atau admin.'
            : apiErr?.message ?? 'Terjadi kesalahan tidak terduga.'}
          {apiErr?.requestId ? ` (req: ${apiErr.requestId})` : ''}
        </p>
        <SiswaButton
          type="button"
          tone="surface"
          size="sm"
          onClick={() => query.refetch()}
          disabled={query.isFetching}
        >
          <RotateCcw className="size-4" />
          Coba lagi
        </SiswaButton>
      </div>
    );
  }

  const items = query.data?.items ?? [];

  if (items.length === 0) {
    return (
      <div className="rounded-siswa border-2 border-dashed border-siswa-border-soft bg-siswa-surface/60 p-8 text-center">
        <BookOpen className="mx-auto mb-2 size-8 text-siswa-text-muted" strokeWidth={2.5} />
        <p className="text-sm text-siswa-text-muted">
          Belum ada bab yang dipublish di kelas ini. Tunggu guru kamu nge-publish bab.
        </p>
      </div>
    );
  }

  return (
    <ul className="space-y-3">
      {items.map((bab) => (
        <li key={bab.id}>
          <BabCard kelasID={kelasID} bab={bab} />
        </li>
      ))}
    </ul>
  );
}

export default function SiswaKelasDetailPage() {
  const searchParams = useSearchParams();
  const id = searchParams?.get('id') ?? '';
  const tabParam = searchParams?.get('tab') ?? null;
  const initialTab = isDetailTab(tabParam) ? tabParam : 'materi';

  if (!id) {
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>ID kelas tidak ada</SiswaCardTitle>
          <SiswaCardDescription>
            URL ini butuh parameter <code>?id=...</code>. Kembali ke daftar kelas
            untuk pilih satu.
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          <SiswaButton asChild tone="surface" size="sm">
            <Link href="/siswa">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </SiswaButton>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  return <SiswaKelasDetailContent kelasID={id} initialTab={initialTab} />;
}
