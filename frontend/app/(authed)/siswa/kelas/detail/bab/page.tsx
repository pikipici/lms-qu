'use client';

/**
 * /siswa/kelas/detail/bab?id=:kelasID&bid=:babID — siswa bab detail page
 * (Task 3.E.2).
 *
 * Static export (Next 14 `output: 'export'`) tidak izinkan dynamic route
 * tanpa generateStaticParams. Mirror pola query-string seperti
 * /guru/kelas/detail/bab.
 *
 * Render:
 *   - Header: nama bab + nomor + breadcrumb (kelas → bab) + progress bar.
 *   - Tab Materi (default + sole): list materi card. Klik card → expand
 *     pakai <MateriViewer hideHeader> sebagai body. Mark-as-read fired
 *     by viewer subcomponents (mount-side / debounced).
 *   - Tab placeholder Soal/Tugas/Pengumuman → pointer ke fase berikutnya
 *     (mirror GuruBabDetail pola).
 *
 * Backend dependency:
 *   - GET /siswa/bab/:id (Task 3.E.1) returns SiswaBabDetail
 *     { bab: SiswaBabItem, materi: SiswaMateriCard[] }.
 *   - POST /siswa/materi/:id/read (Task 3.C.4) idempotent — auto-fired
 *     by viewer per-tipe.
 *
 * Read state UX:
 *   - Card tampilin badge "✓ dibaca" kalau sudah_dibaca=true.
 *   - Setelah viewer fire markRead, kita refetch detail biar progress +
 *     read-set ke-update. Refetch debounced 3s setelah pertama buka
 *     (cukup buat PdfViewer 2s debounce + 1s buffer).
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

import { ApiError } from '@/lib/api';
import { listMyKelas, type MyKelasItem } from '@/lib/siswa-api';
import {
  getSiswaBab,
  type SiswaMateriCard,
  type SiswaMateriTipe,
} from '@/lib/siswa-bab-api';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { MateriViewer } from '@/components/materi/MateriViewer';
import { SiswaBabProgressBar } from '@/components/siswa/SiswaBabProgressBar';
import { siswaCardToMateri } from '@/components/siswa/siswaCardToMateri';
import { PengumumanReadList } from '@/components/pengumuman/PengumumanReadList';

// ---------- Sub-tab definition ----------

type SubTabKey = 'materi' | 'soal' | 'tugas' | 'pengumuman';

const SUB_TABS: {
  key: SubTabKey;
  label: string;
  Icon: React.ComponentType<{ className?: string }>;
}[] = [
  { key: 'materi', label: 'Materi', Icon: FileText },
  { key: 'soal', label: 'Soal', Icon: BookOpen },
  { key: 'tugas', label: 'Tugas', Icon: ClipboardList },
  { key: 'pengumuman', label: 'Pengumuman', Icon: Megaphone },
];

// ---------- Helpers ----------

function tipeIcon(t: SiswaMateriTipe): React.ComponentType<{ className?: string }> {
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

function tipeBadgeColor(t: SiswaMateriTipe): string {
  switch (t) {
    case 'pdf':
      return 'bg-rose-50 text-rose-700 dark:bg-rose-950 dark:text-rose-300';
    case 'youtube':
      return 'bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300';
    case 'markdown':
      return 'bg-sky-50 text-sky-700 dark:bg-sky-950 dark:text-sky-300';
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

  // Schedule a refetch after the user opens this card the first time so the
  // sudah_dibaca flag + parent progress catch up. Debounce 3s ≥ PDF mark-read
  // debounce (2s) + small buffer. Subsequent toggles tidak trigger ulang.
  React.useEffect(() => {
    if (expanded && !wasExpandedRef.current) {
      wasExpandedRef.current = true;
      const t = window.setTimeout(() => onOpened(), 3000);
      return () => window.clearTimeout(t);
    }
    return undefined;
  }, [expanded, onOpened]);

  return (
    <div className="rounded-md border bg-card">
      <button
        type="button"
        onClick={onToggle}
        className="flex w-full items-start gap-3 p-3 text-left hover:bg-accent/40"
      >
        <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="truncate text-sm font-medium">{card.judul}</span>
            <span
              className={cn(
                'rounded-full px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide',
                tipeBadgeColor(card.tipe),
              )}
            >
              {tipeLabel(card.tipe)}
            </span>
            {card.sudah_dibaca && (
              <span className="inline-flex items-center gap-1 rounded-full bg-emerald-50 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300">
                <CheckCircle2 className="size-3" />
                Dibaca
              </span>
            )}
          </div>
        </div>
        {expanded ? (
          <ChevronDown className="mt-1 size-4 shrink-0 text-muted-foreground" />
        ) : (
          <ChevronRight className="mt-1 size-4 shrink-0 text-muted-foreground" />
        )}
      </button>
      {expanded && (
        <div className="border-t bg-background p-3">
          <MateriViewer materi={siswaCardToMateri(card)} hideHeader />
        </div>
      )}
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
      <div className="rounded-md border border-dashed p-8 text-center">
        <FileText className="mx-auto mb-2 size-6 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          Belum ada materi di bab ini. Tunggu guru lu nge-upload.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {materi.map((card) => (
        <MateriRow
          key={card.id}
          card={card}
          expanded={openID === card.id}
          onToggle={() => setOpenID((prev) => (prev === card.id ? null : card.id))}
          onOpened={onRefresh}
        />
      ))}
    </div>
  );
}

// ---------- Page content ----------

function PlaceholderTab({
  Icon,
  title,
  body,
  taskRef,
}: {
  Icon: React.ComponentType<{ className?: string }>;
  title: string;
  body: string;
  taskRef: string;
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Icon className="size-5 text-muted-foreground" />
          <CardTitle className="text-base">{title}</CardTitle>
        </div>
        <CardDescription>{body}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
          Akan tersedia di {taskRef}.
        </div>
      </CardContent>
    </Card>
  );
}

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

  // Refresh handler dipanggil oleh MateriRow setelah viewer di-buka
  // (debounced 3s). Invalidate kedua query: detail (read state) + parent
  // list (progress per bab di /siswa/kelas/detail page).
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
        <div className="h-6 w-48 animate-pulse rounded bg-muted" />
        <div className="h-32 animate-pulse rounded-md border bg-muted/40" />
        <div className="h-64 animate-pulse rounded-md border bg-muted/40" />
      </div>
    );
  }

  if (detailQuery.isError) {
    const err = detailQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    const isNotFound = apiErr?.code === 'not_found';
    const isForbidden = apiErr?.code === 'forbidden';
    const requestId = apiErr?.requestId;
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {isNotFound
              ? 'Bab tidak ditemukan'
              : isForbidden
                ? 'Akses ditolak'
                : 'Gagal memuat bab'}
          </CardTitle>
          <CardDescription>
            {isNotFound
              ? 'Bab ini belum dipublish atau sudah dihapus oleh guru.'
              : isForbidden
                ? 'Lu tidak terdaftar aktif di kelas bab ini.'
                : apiErr?.message ?? 'Terjadi kesalahan tidak terduga.'}
            {requestId ? ` (req: ${requestId})` : ''}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline" size="sm">
            <Link href={`/siswa/kelas/detail?id=${kelasID}`}>
              <ArrowLeft className="size-4" />
              Kembali ke kelas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  const detail = detailQuery.data!;
  const bab = detail.bab;
  const kelasNama = enrollment?.kelas.nama ?? 'kelas';

  return (
    <div className="space-y-6">
      <header className="space-y-3">
        <Button asChild variant="ghost" size="sm" className="-ml-3">
          <Link href={`/siswa/kelas/detail?id=${kelasID}`}>
            <ArrowLeft className="size-4" />
            Kelas {kelasNama}
          </Link>
        </Button>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <span className="text-sm text-muted-foreground">Bab {bab.nomor}</span>
            <h1 className="text-2xl font-semibold tracking-tight">{bab.judul}</h1>
            {bab.deskripsi && (
              <p className="max-w-3xl text-sm text-muted-foreground">
                {bab.deskripsi}
              </p>
            )}
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => detailQuery.refetch()}
            disabled={detailQuery.isFetching}
          >
            <RotateCcw className="size-4" />
            Refresh
          </Button>
        </div>
        <div className="max-w-md">
          <SiswaBabProgressBar
            persen={bab.progress.persen}
            materiRead={bab.progress.materi_read}
            materiTotal={bab.progress.materi_total}
            babKosong={bab.progress.bab_kosong}
            size="md"
          />
        </div>
      </header>

      {/* Tab nav */}
      <div className="flex gap-1 overflow-x-auto border-b">
        {SUB_TABS.map(({ key, label, Icon }) => {
          const active = tab === key;
          return (
            <button
              key={key}
              type="button"
              onClick={() => setTab(key)}
              className={cn(
                'flex items-center gap-1.5 whitespace-nowrap border-b-2 px-3 py-2 text-sm transition-colors',
                active
                  ? 'border-primary font-medium text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground',
              )}
            >
              <Icon className="size-4" />
              {label}
            </button>
          );
        })}
      </div>

      {/* Tab content */}
      {tab === 'materi' && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Materi bab</CardTitle>
            <CardDescription>
              Klik card buat baca / tonton. Status bacaan otomatis ke-track.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <MateriTab
              materi={detail.materi}
              onRefresh={handleMateriOpened}
            />
          </CardContent>
        </Card>
      )}

      {tab === 'soal' && (
        <PlaceholderTab
          Icon={BookOpen}
          title="Soal"
          body="Latihan soal + ulangan bab. Akan tersedia di Fase 5."
          taskRef="Fase 5 (Soal Bab)"
        />
      )}

      {tab === 'tugas' && (
        <PlaceholderTab
          Icon={ClipboardList}
          title="Tugas"
          body="Tugas + submit + nilai. Akan tersedia di Fase 4."
          taskRef="Fase 4 (Tugas)"
        />
      )}

      {tab === 'pengumuman' && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Pengumuman bab</CardTitle>
            <CardDescription>
              Pengumuman dari guru terkait bab ini.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <PengumumanReadList
              kelasID={kelasID}
              babID={babID}
              emptyState="Belum ada pengumuman untuk bab ini."
            />
          </CardContent>
        </Card>
      )}
    </div>
  );
}

export default function SiswaBabDetailPage() {
  const searchParams = useSearchParams();
  const kelasID = searchParams?.get('id') ?? '';
  const babID = searchParams?.get('bid') ?? '';

  if (!kelasID || !babID) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Parameter tidak lengkap</CardTitle>
          <CardDescription>
            URL ini butuh <code>?id=:kelasID&bid=:babID</code>. Kembali ke
            daftar kelas untuk pilih bab.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline" size="sm">
            <Link href="/siswa">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  return <SiswaBabDetailContent kelasID={kelasID} babID={babID} />;
}
