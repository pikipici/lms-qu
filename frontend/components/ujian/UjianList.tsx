'use client';

/**
 * UjianList — list semua Ujian Harian per kelas (Task 6.F.2). Mirror
 * Bab list pattern: card per ujian + status badge + dropdown actions
 * (Edit / Duplicate / Delete) + expand RekapHasilUjianTable inline.
 *
 * Source mode badge (manual/random) + jumlah_soal preview via
 * source_config_json embedded di response. Action map:
 *   - Tombol "Tambah ujian" → UjianFormDialog mode create
 *   - Card per ujian:
 *     - Klik card → expand panel dengan RekapHasilUjianTable
 *     - Dropdown: Edit / Duplikat / Hapus
 *
 * Locked #56 PATCH+DELETE bawa version. Soft-archive via tombol terpisah
 * (set status='archived' lewat form edit).
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  ChevronDown,
  ChevronRight,
  ClipboardCheck,
  Copy,
  Loader2,
  MoreVertical,
  Pencil,
  Plus,
  Trash2,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type Ujian,
  type UjianSourceConfig,
  type UjianSourceMode,
  deleteUjian,
  duplicateUjian,
  friendlyUjianError,
  listUjianByKelas,
} from '@/lib/ujian-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';

import { RekapHasilUjianTable } from './RekapHasilUjianTable';
import { UjianFormDialog } from './UjianFormDialog';

export interface UjianListProps {
  kelasID: string;
  /** Pass true kalau kelas archived → disable mutate actions. */
  disabled?: boolean;
}

export function UjianList({ kelasID, disabled }: UjianListProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const queryKey = React.useMemo(
    () => ['guru', 'kelas', kelasID, 'ujian'] as const,
    [kelasID],
  );
  const invalidateKeys = React.useMemo(() => [queryKey] as const, [queryKey]);

  const listQuery = useQuery({
    queryKey,
    queryFn: () => listUjianByKelas(kelasID, { limit: 50 }),
    staleTime: 10_000,
  });

  const [editing, setEditing] = React.useState<Ujian | null>(null);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [expandedID, setExpandedID] = React.useState<string | null>(null);

  const deleteMutation = useMutation({
    mutationFn: (u: Ujian) => deleteUjian(u.id, u.version),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
      toast({ title: 'Ujian dihapus' });
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyUjianError(apiErr, 'delete')
        : 'Gagal menghapus ujian.';
      toast({
        title: 'Gagal menghapus ujian',
        description: apiErr?.requestId
          ? `${message} (req: ${apiErr.requestId})`
          : message,
        variant: 'destructive',
      });
    },
  });

  const duplicateMutation = useMutation({
    mutationFn: (u: Ujian) => duplicateUjian(u.id),
    onSuccess: ({ ujian }) => {
      queryClient.invalidateQueries({ queryKey });
      toast({
        title: 'Ujian diduplikasi',
        description: `Salinan baru: "${ujian.judul}". Status di-reset ke draft.`,
      });
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyUjianError(apiErr, 'duplicate')
        : 'Gagal menduplikasi ujian.';
      toast({
        title: 'Gagal menduplikasi',
        description: apiErr?.requestId
          ? `${message} (req: ${apiErr.requestId})`
          : message,
        variant: 'destructive',
      });
    },
  });

  const items = listQuery.data?.items ?? [];

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <CardTitle className="text-base">Ulangan Harian</CardTitle>
            <CardDescription>
              Ulangan cross-bab yang menarik soal dari Bank Soal pribadi
              lu (per-guru). Bisa pilih manual atau random per filter tag.
            </CardDescription>
          </div>
          <Button
            size="sm"
            type="button"
            onClick={() => setCreateOpen(true)}
            disabled={disabled}
          >
            <Plus className="size-4" />
            Tambah ujian
          </Button>
        </div>
      </CardHeader>

      <CardContent>
        {listQuery.isPending ? (
          <div className="flex items-center justify-center py-12 text-muted-foreground">
            <Loader2 className="size-5 animate-spin" />
          </div>
        ) : listQuery.isError ? (
          <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
            Gagal memuat daftar ujian. Coba refresh.
          </div>
        ) : items.length === 0 ? (
          <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
            Belum ada ulangan. Mulai dengan tombol Tambah ujian — ujian
            akan masuk sebagai draft, tinggal set sumber soal lalu publish.
          </div>
        ) : (
          <ul className="space-y-2">
            {items.map((u) => (
              <UjianRow
                key={u.id}
                ujian={u}
                expanded={expandedID === u.id}
                onToggle={() =>
                  setExpandedID((prev) => (prev === u.id ? null : u.id))
                }
                onEdit={() => setEditing(u)}
                onDuplicate={() => duplicateMutation.mutate(u)}
                onDelete={() => {
                  if (
                    confirm(
                      `Hapus ujian "${u.judul}"? Ini permanen — kalau sudah dipakai siswa, gagal dengan attempts_exist.`,
                    )
                  ) {
                    deleteMutation.mutate(u);
                  }
                }}
                disabled={
                  disabled ||
                  deleteMutation.isPending ||
                  duplicateMutation.isPending
                }
              />
            ))}
          </ul>
        )}
      </CardContent>

      <UjianFormDialog
        key={`create-${createOpen}`}
        open={createOpen}
        onOpenChange={setCreateOpen}
        kelasID={kelasID}
        ujian={null}
        invalidateKeys={invalidateKeys}
      />
      {editing && (
        <UjianFormDialog
          key={`edit-${editing.id}-${editing.version}`}
          open={!!editing}
          onOpenChange={(o) => {
            if (!o) setEditing(null);
          }}
          kelasID={kelasID}
          ujian={editing}
          invalidateKeys={invalidateKeys}
        />
      )}
    </Card>
  );
}

// ---------- Row + expanded panel ----------

function UjianRow({
  ujian,
  expanded,
  onToggle,
  onEdit,
  onDuplicate,
  onDelete,
  disabled,
}: {
  ujian: Ujian;
  expanded: boolean;
  onToggle: () => void;
  onEdit: () => void;
  onDuplicate: () => void;
  onDelete: () => void;
  disabled?: boolean;
}) {
  const sourceMode: UjianSourceMode | '' =
    ujian.source_config_json &&
    typeof ujian.source_config_json === 'object' &&
    'mode' in ujian.source_config_json
      ? ((ujian.source_config_json as UjianSourceConfig).mode ?? '')
      : '';
  const jumlahSoal = describeJumlahSoal(
    ujian.source_config_json as UjianSourceConfig | Record<string, never>,
  );

  return (
    <li className="rounded-md border bg-background">
      <div className="flex items-start gap-2 p-3">
        <button
          type="button"
          onClick={onToggle}
          className="mt-0.5 rounded p-0.5 text-muted-foreground hover:bg-accent"
          aria-label={expanded ? 'Tutup detail' : 'Buka detail'}
        >
          {expanded ? (
            <ChevronDown className="size-4" />
          ) : (
            <ChevronRight className="size-4" />
          )}
        </button>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-sm font-semibold">{ujian.judul}</h3>
            <StatusBadge status={ujian.status} />
            {sourceMode && <SourceBadge mode={sourceMode} />}
            {jumlahSoal && (
              <span className="text-xs text-muted-foreground">
                {jumlahSoal}
              </span>
            )}
            <span className="text-xs text-muted-foreground">
              · Durasi {ujian.durasi_menit}m · v{ujian.version}
            </span>
          </div>
          {ujian.deskripsi && (
            <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
              {ujian.deskripsi}
            </p>
          )}
          <p className="mt-1 text-xs text-muted-foreground">
            {formatTimingRange(ujian.waktu_mulai, ujian.waktu_selesai)} ·
            Review{' '}
            {ujian.izinkan_review_setelah_submit ? (
              <span>
                aktif
                {ujian.waktu_buka_review
                  ? ` (buka ${formatRFC(ujian.waktu_buka_review)})`
                  : ''}
              </span>
            ) : (
              <span>dimatikan</span>
            )}
          </p>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              size="sm"
              variant="ghost"
              type="button"
              disabled={disabled}
            >
              <MoreVertical className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={onEdit} disabled={disabled}>
              <Pencil className="size-4" />
              Edit
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onDuplicate} disabled={disabled}>
              <Copy className="size-4" />
              Duplikat
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={onDelete}
              disabled={disabled}
              className="text-destructive focus:text-destructive"
            >
              <Trash2 className="size-4" />
              Hapus
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {expanded && (
        <div className="border-t bg-muted/20 p-3">
          <div className="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
            <ClipboardCheck className="size-3.5" />
            Rekap hasil siswa
          </div>
          <RekapHasilUjianTable ujianID={ujian.id} disabled={disabled} />
        </div>
      )}
    </li>
  );
}

function StatusBadge({ status }: { status: string }) {
  const cls =
    status === 'published'
      ? 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
      : status === 'archived'
        ? 'border-zinc-300 bg-zinc-50 text-zinc-700 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300'
        : 'border-amber-300 bg-amber-50 text-amber-700 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-300';
  return (
    <span
      className={cn(
        'inline-flex rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
        cls,
      )}
    >
      {status}
    </span>
  );
}

function SourceBadge({ mode }: { mode: UjianSourceMode }) {
  const cls =
    mode === 'manual'
      ? 'border-sky-300 bg-sky-50 text-sky-700 dark:border-sky-800 dark:bg-sky-950 dark:text-sky-300'
      : 'border-violet-300 bg-violet-50 text-violet-700 dark:border-violet-800 dark:bg-violet-950 dark:text-violet-300';
  return (
    <span
      className={cn(
        'inline-flex rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
        cls,
      )}
    >
      {mode}
    </span>
  );
}

function describeJumlahSoal(
  src: UjianSourceConfig | Record<string, never>,
): string {
  if (!src || typeof src !== 'object' || !('mode' in src)) return '';
  if (src.mode === 'manual') {
    const n = (src as Extract<UjianSourceConfig, { mode: 'manual' }>).soal_ids
      ?.length ?? 0;
    return `· ${n} soal dipilih`;
  }
  if (src.mode === 'random') {
    const n = (src as Extract<UjianSourceConfig, { mode: 'random' }>)
      .jumlah_soal ?? 0;
    return `· random ${n} soal`;
  }
  return '';
}

function formatTimingRange(
  start?: string | null,
  end?: string | null,
): string {
  if (!start && !end) return 'Tersedia kapanpun (status published)';
  const fmt = (rfc?: string | null) =>
    rfc ? formatRFC(rfc) : '—';
  return `${fmt(start)} → ${fmt(end)}`;
}

function formatRFC(rfc: string): string {
  try {
    return new Date(rfc).toLocaleString('id-ID', {
      dateStyle: 'short',
      timeStyle: 'short',
      timeZone: 'Asia/Jakarta',
    });
  } catch {
    return rfc;
  }
}
