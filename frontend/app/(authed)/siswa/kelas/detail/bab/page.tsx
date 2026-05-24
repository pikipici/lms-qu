'use client';

/**
 * /siswa/kelas/detail/bab?id=:kelasID&bid=:babID — siswa bab detail page.
 *
 * Refactor: neo-brutalism + pastel pop.
 *
 * Header: tone deterministic dari kelas_id (konsistensi sama header
 * /siswa/kelas/detail). Tab nav pill-style berwarna per section accent:
 *   - Materi   → blue
 *   - Soal     → yellow
 *   - Tugas    → pink
 *   - Pengumuman → cream
 *
 * Materi card pakai SiswaCard surface + press feedback. Mark-as-read
 * still fired by viewer subcomponents on mount (debounced for PDF).
 */

import * as React from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  ArrowLeft,
  BookOpen,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  ClipboardList,
  FileText,
  Megaphone,
  RotateCcw,
  Type,
  Youtube,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

import { ApiError } from '@/lib/api';
import { listMyKelas, type MyKelasItem } from '@/lib/siswa-api';
import {
  getSiswaBab,
  type SiswaMateriCard,
  type SiswaMateriTipe,
} from '@/lib/siswa-bab-api';
import { cn } from '@/lib/utils';
import { MateriViewer } from '@/components/materi/MateriViewer';
import { SiswaBabProgressBar } from '@/components/siswa/SiswaBabProgressBar';
import { siswaCardToMateri } from '@/components/siswa/siswaCardToMateri';
import { PengumumanReadList } from '@/components/pengumuman/PengumumanReadList';
import { SiswaTugasList } from '@/components/submission/SiswaTugasList';
import { LatihanPlayer } from '@/components/soalbab/LatihanPlayer';
import { UlanganSection } from '@/components/soalbab/UlanganSection';
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
import type { SiswaSectionKind } from '@/components/siswa-ui';

// ---------- Sub-tab definition ----------

type SubTabKey = 'materi' | 'soal' | 'tugas' | 'pengumuman';

const SUB_TABS: {
  key: SubTabKey;
  label: string;
  Icon: LucideIcon;
  section: SiswaSectionKind;
}[] = [
  { key: 'materi', label: 'Materi', Icon: FileText, section: 'materi' },
  { key: 'soal', label: 'Soal', Icon: BookOpen, section: 'ulangan' },
  { key: 'tugas', label: 'Tugas', Icon: ClipboardList, section: 'tugas' },
  { key: 'pengumuman', label: 'Pengumuman', Icon: Megaphone, section: 'umum' },
];

// ---------- Helpers ----------

function tipeIcon(t: SiswaMateriTipe): LucideIcon {
  switch (t) {
    case 'pdf':
      return FileText;
    case 'youtube':
      return Youtube;
    case 'markdown':
      return Type;
  }
}

function tipeLabel(t: SiswaMateriTipe): string {
  switch (t) {
    case 'pdf':
      return 'PDF';
    case 'youtube':
      return 'YouTube';
    case 'markdown':
      return 'Markdown';
  }
}

function tipeBadgeTone(t: SiswaMateriTipe): React.ComponentProps<typeof SiswaBadge>['tone'] {
  switch (t) {
    case 'pdf':
      return 'pink';
    case 'youtube':
      return 'danger';
    case 'markdown':
      return 'blue';
  }
}

// ---------- Materi card (expandable) ----------

function MateriRow({
  card,
  expanded,
  onToggle,
  onOpened,
}: {
  card: SiswaMateriCard;
  expanded: boolean;
  onToggle: () => void;
  onOpened: () => void;
}) {
  const Icon = tipeIcon(card.tipe);
  const wasExpandedRef = React.useRef(false);

  React.useEffect(() => {
    if (expanded && !wasExpandedRef.current) {
      wasExpandedRef.current = true;
      const t = window.setTimeout(() => onOpened(), 3000);
      return () => window.clearTimeout(t);
    }
    return undefined;
  }, [expanded, onOpened]);

  return (
    <div className="overflow-hidden rounded-siswa siswa-border bg-siswa-surface siswa-shadow-sm">
      <button
        type="button"
        onClick={onToggle}
        className="flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-siswa-cream/60 focus-visible:outline-none focus-visible:bg-siswa-cream/70"
      >
        <span className="grid size-9 shrink-0 place-items-center rounded-siswa border-2 border-siswa-border bg-siswa-surface">
          <Icon className="size-4" strokeWidth={2.5} />
        </span>
        <div className="min-w-0 flex-1 space-y-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <span className="siswa-display truncate text-sm font-bold leading-tight">
              {card.judul}
            </span>
            <SiswaBadge tone={tipeBadgeTone(card.tipe)}>
              {tipeLabel(card.tipe)}
            </SiswaBadge>
            {card.sudah_dibaca ? (
              <SiswaBadge tone="success" className="text-siswa-text">
                <CheckCircle2 className="size-3" strokeWidth={2.5} />
                Dibaca
              </SiswaBadge>
            ) : null}
          </div>
        </div>
        {expanded ? (
          <ChevronDown
            className="mt-1 size-4 shrink-0 text-siswa-text-muted"
            strokeWidth={2.5}
          />
        ) : (
          <ChevronRight
            className="mt-1 size-4 shrink-0 text-siswa-text-muted"
            strokeWidth={2.5}
          />
        )}
      </button>
      {expanded ? (
        <div className="border-t-2 border-siswa-border bg-siswa-bg p-4">
          <MateriViewer materi={siswaCardToMateri(card)} hideHeader />
        </div>
      ) : null}
    </div>
  );
}

// ---------- Materi tab ----------

function MateriTab({
  materi,
  onRefresh,
}: {
  materi: SiswaMateriCard[];
  onRefresh: () => void;
}) {
  const [openID, setOpenID] = React.useState<string | null>(null);

  if (materi.length === 0) {
    return (
      <div className="rounded-siswa border-2 border-dashed border-siswa-border-soft bg-siswa-surface/60 p-8 text-center">
        <FileText
          className="mx-auto mb-2 size-8 text-siswa-text-muted"
          strokeWidth={2.5}
        />
        <p className="text-sm text-siswa-text-muted">
          Belum ada materi di bab ini. Tunggu guru lu nge-upload.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {materi.map((card) => (
        <MateriRow
          key={card.id}
          card={card}
          expanded={openID === card.id}
          onToggle={() =>
            setOpenID((prev) => (prev === card.id ? null : card.id))
          }
          onOpened={onRefresh}
        />
      ))}
    </div>
  );
}

// ---------- Soal tab (Latihan + Ulangan sub-switch) ----------

type SoalSubTab = 'latihan' | 'ulangan';

function SoalTabContent({ babID }: { babID: string }) {
  const [sub, setSub] = React.useState<SoalSubTab>('latihan');
  return (
    <div className="space-y-4">
      <div className="inline-flex gap-1 rounded-siswa siswa-border bg-siswa-surface p-1 siswa-shadow-sm">
        {(
          [
            { key: 'latihan', label: 'Latihan', tone: 'bg-siswa-green' },
            { key: 'ulangan', label: 'Ulangan Bab', tone: 'bg-siswa-yellow' },
          ] as const
        ).map(({ key, label, tone }) => {
          const active = sub === key;
          return (
            <button
              key={key}
              type="button"
              onClick={() => setSub(key)}
              className={cn(
                'rounded-[calc(var(--siswa-radius)-4px)] px-4 py-1.5 text-sm font-semibold transition-colors',
                active
                  ? cn('border-2 border-siswa-border', tone)
                  : 'text-siswa-text/70 hover:bg-siswa-cream/60',
              )}
            >
              {label}
            </button>
          );
        })}
      </div>

      {sub === 'latihan' ? (
        <LatihanPlayer babID={babID} disabled={false} />
      ) : (
        <UlanganSection babID={babID} disabled={false} />
      )}
    </div>
  );
}

// ---------- Page content ----------

function SiswaBabDetailContent({
  kelasID,
  babID,
}: {
  kelasID: string;
  babID: string;
}) {
  const queryClient = useQueryClient();
  const [tab, setTab] = React.useState<SubTabKey>('materi');

  const enrollmentQuery = useQuery({
    queryKey: ['siswa', 'kelas', 'list'],
    queryFn: () => listMyKelas({ page: 1, pageSize: 50 }),
    staleTime: 30_000,
  });

  const detailQuery = useQuery({
    queryKey: ['siswa', 'bab', 'detail', babID],
    queryFn: () => getSiswaBab(babID),
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

  const handleMateriOpened = React.useCallback(() => {
    queryClient.invalidateQueries({
      queryKey: ['siswa', 'bab', 'detail', babID],
    });
    queryClient.invalidateQueries({
      queryKey: ['siswa', 'kelas', 'bab', kelasID],
    });
  }, [babID, kelasID, queryClient]);

  const enrollment: MyKelasItem | undefined = React.useMemo(() => {
    return enrollmentQuery.data?.items.find((it) => it.kelas.id === kelasID);
  }, [enrollmentQuery.data, kelasID]);

  if (detailQuery.isPending || enrollmentQuery.isPending) {
    return (
      <div className="space-y-4">
        <div className="h-6 w-48 animate-pulse rounded bg-siswa-text/10" />
        <div className="h-32 animate-pulse rounded-siswa siswa-border bg-siswa-surface" />
        <div className="h-64 animate-pulse rounded-siswa siswa-border bg-siswa-surface" />
      </div>
    );
  }

  if (detailQuery.isError) {
    const err = detailQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    const isNotFound = apiErr?.code === 'not_found';
    const isForbidden = apiErr?.code === 'forbidden';
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>
            {isNotFound
              ? 'Bab tidak ditemukan'
              : isForbidden
                ? 'Akses ditolak'
                : 'Gagal memuat bab'}
          </SiswaCardTitle>
          <SiswaCardDescription>
            {isNotFound
              ? 'Bab ini belum dipublish atau sudah dihapus oleh guru.'
              : isForbidden
                ? 'Lu tidak terdaftar aktif di kelas bab ini.'
                : apiErr?.message ?? 'Terjadi kesalahan tidak terduga.'}
            {apiErr?.requestId ? ` (req: ${apiErr.requestId})` : ''}
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          <SiswaButton asChild tone="surface" size="sm">
            <Link href={`/siswa/kelas/detail?id=${kelasID}`}>
              <ArrowLeft className="size-4" />
              Kembali ke kelas
            </Link>
          </SiswaButton>
        </SiswaCardBody>
      </SiswaCard>
    );
  }

  const detail = detailQuery.data!;
  const bab = detail.bab;
  const kelasNama = enrollment?.kelas.nama ?? 'kelas';
  const tone = kelasToneFromId(kelasID);
  const meta = SECTION_META[tone];
  const KelasIcon = meta.Icon;

  return (
    <div className="space-y-6">
      <SiswaButton asChild tone="ghost" size="sm" className="-ml-2">
        <Link href={`/siswa/kelas/detail?id=${kelasID}`}>
          <ArrowLeft className="size-4" />
          Kelas {kelasNama}
        </Link>
      </SiswaButton>

      {/* Header card */}
      <SiswaCard tone={tone} shadow="lg" className="overflow-hidden">
        <div className="flex items-start gap-4 border-b-2 border-siswa-border bg-siswa-surface/70 px-6 py-5">
          <span className="grid size-12 shrink-0 place-items-center rounded-siswa siswa-border bg-siswa-surface siswa-shadow-sm">
            <KelasIcon className="size-6" strokeWidth={2.5} />
          </span>
          <div className="min-w-0 flex-1 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs font-semibold uppercase tracking-[0.18em] text-siswa-text-muted">
                Bab {bab.nomor}
              </span>
              <SiswaBadge tone="cream">{kelasNama}</SiswaBadge>
            </div>
            <h1 className="siswa-display text-2xl font-bold leading-tight sm:text-3xl">
              {bab.judul}
            </h1>
            {bab.deskripsi ? (
              <p className="max-w-3xl text-sm text-siswa-text">
                {bab.deskripsi}
              </p>
            ) : null}
          </div>
          <SiswaButton
            type="button"
            tone="surface"
            size="sm"
            onClick={() => detailQuery.refetch()}
            disabled={detailQuery.isFetching}
          >
            <RotateCcw className="size-4" />
            Refresh
          </SiswaButton>
        </div>
        <div className="px-6 py-5">
          <div className="max-w-md">
            <SiswaBabProgressBar
              persen={bab.progress.persen}
              materiRead={bab.progress.materi_read}
              materiTotal={bab.progress.materi_total}
              babKosong={bab.progress.bab_kosong}
              size="md"
              variant="siswa"
            />
          </div>
        </div>
      </SiswaCard>

      {/* Tab nav — pill-style berwarna per section accent */}
      <div
        className="flex flex-wrap gap-2 rounded-siswa siswa-border bg-siswa-surface p-2 siswa-shadow-sm"
        role="tablist"
        aria-label="Bagian bab"
      >
        {SUB_TABS.map(({ key, label, Icon, section }) => {
          const active = tab === key;
          const sectionMeta = SECTION_META[section];
          return (
            <button
              key={key}
              type="button"
              role="tab"
              aria-selected={active}
              onClick={() => setTab(key)}
              className={cn(
                'flex items-center gap-2 rounded-[calc(var(--siswa-radius)-4px)] px-3 py-2 text-sm font-semibold transition-all',
                active
                  ? cn(
                      'border-2 border-siswa-border siswa-shadow-sm',
                      sectionMeta.solid,
                    )
                  : 'border-2 border-transparent text-siswa-text/70 hover:bg-siswa-cream/60 hover:text-siswa-text',
              )}
            >
              <Icon className="size-4" strokeWidth={2.5} />
              {label}
            </button>
          );
        })}
      </div>

      {/* Tab content */}
      {tab === 'materi' ? (
        <SiswaCard tone="materi" shadow="md">
          <SiswaCardHeader>
            <SiswaCardTitle className="flex items-center gap-2">
              <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                <FileText className="size-4" strokeWidth={2.5} />
              </span>
              Materi bab
            </SiswaCardTitle>
            <SiswaCardDescription>
              Klik card buat baca atau tonton. Status bacaan otomatis ke-track.
            </SiswaCardDescription>
          </SiswaCardHeader>
          <SiswaCardBody>
            <MateriTab materi={detail.materi} onRefresh={handleMateriOpened} />
          </SiswaCardBody>
        </SiswaCard>
      ) : null}

      {tab === 'soal' ? (
        <SiswaCard tone="ulangan" shadow="md">
          <SiswaCardHeader>
            <SiswaCardTitle className="flex items-center gap-2">
              <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                <BookOpen className="size-4" strokeWidth={2.5} />
              </span>
              Soal bab
            </SiswaCardTitle>
            <SiswaCardDescription>
              Latihan tanpa nilai (formative) — atau Ulangan Bab yang masuk
              rapor.
            </SiswaCardDescription>
          </SiswaCardHeader>
          <SiswaCardBody>
            <SoalTabContent babID={babID} />
          </SiswaCardBody>
        </SiswaCard>
      ) : null}

      {tab === 'tugas' ? (
        <SiswaCard tone="tugas" shadow="md">
          <SiswaCardHeader>
            <SiswaCardTitle className="flex items-center gap-2">
              <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                <ClipboardList className="size-4" strokeWidth={2.5} />
              </span>
              Tugas bab
            </SiswaCardTitle>
            <SiswaCardDescription>
              Tugas yang ditujukan untuk bab ini. Buka untuk submit/resubmit.
            </SiswaCardDescription>
          </SiswaCardHeader>
          <SiswaCardBody>
            <SiswaTugasList
              kelasID={kelasID}
              babID={babID}
              emptyState="Belum ada tugas untuk bab ini."
            />
          </SiswaCardBody>
        </SiswaCard>
      ) : null}

      {tab === 'pengumuman' ? (
        <SiswaCard tone="umum" shadow="md">
          <SiswaCardHeader>
            <SiswaCardTitle className="flex items-center gap-2">
              <span className="grid size-9 place-items-center rounded-siswa siswa-border bg-siswa-surface">
                <Megaphone className="size-4" strokeWidth={2.5} />
              </span>
              Pengumuman bab
            </SiswaCardTitle>
            <SiswaCardDescription>
              Pengumuman dari guru terkait bab ini.
            </SiswaCardDescription>
          </SiswaCardHeader>
          <SiswaCardBody>
            <PengumumanReadList
              kelasID={kelasID}
              babID={babID}
              emptyState="Belum ada pengumuman untuk bab ini."
            />
          </SiswaCardBody>
        </SiswaCard>
      ) : null}
    </div>
  );
}

export default function SiswaBabDetailPage() {
  const searchParams = useSearchParams();
  const kelasID = searchParams?.get('id') ?? '';
  const babID = searchParams?.get('bid') ?? '';

  if (!kelasID || !babID) {
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>Parameter tidak lengkap</SiswaCardTitle>
          <SiswaCardDescription>
            URL ini butuh <code>?id=:kelasID&bid=:babID</code>. Kembali ke
            daftar kelas untuk pilih bab.
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

  return <SiswaBabDetailContent kelasID={kelasID} babID={babID} />;
}
