'use client';

/**
 * TugasEditDialog — edit dialog + attachment manager untuk tugas yang sudah ada.
 *
 * Section 1 — Edit metadata: judul, deskripsi, deadline, izinkan_late,
 * penalty_persen, wajib_attachment, status (draft↔published↔archived).
 * Server pakai optimistic concurrency (#56) — kirim version dari snapshot.
 * 409 version_conflict → toast + invalidate untuk refetch + form re-sync.
 *
 * Section 2 — Attachments: list + upload (multipart) + delete + buka URL
 * (presigned 15-min). Cap 5 file × 20MB. Allowlist: PDF/DOCX/JPG/PNG/ZIP.
 *
 * Kelas/bab scope tidak bisa pindah lewat dialog ini — backend support PATCH
 * bab_id null (kelas-wide) tapi UX edit-as-move kebanyakan friksi; user
 * delete + create ulang kalau perlu pindah scope.
 */

import * as React from 'react';
import {
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import {
  Archive,
  ClipboardList,
  Download,
  FileText,
  Loader2,
  Trash2,
  Upload,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type Tugas,
  type TugasAttachment,
  type TugasStatus,
  MAX_TUGAS_ATTACHMENTS,
  MAX_TUGAS_ATTACHMENT_BYTES,
  MAX_TUGAS_DESKRIPSI_BYTES,
  MAX_TUGAS_JUDUL_LENGTH,
  TUGAS_ATTACHMENT_ACCEPT,
  deleteAttachment,
  friendlyTugasError,
  getAttachmentURL,
  listAttachments,
  updateTugas,
  uploadAttachment,
} from '@/lib/tugas-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { MarkdownEditor } from '@/components/materi/MarkdownEditor';
import { cn } from '@/lib/utils';

export interface TugasEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  tugas: Tugas;
  invalidateKeys: readonly (readonly unknown[])[];
}

// Convert ISO → datetime-local string (yyyy-MM-ddTHH:mm in local TZ).
function isoToLocalInput(iso: string | null): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export function TugasEditDialog({
  open,
  onOpenChange,
  tugas,
  invalidateKeys,
}: TugasEditDialogProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const [judul, setJudul] = React.useState(tugas.judul);
  const [deskripsi, setDeskripsi] = React.useState(tugas.deskripsi);
  const [deadlineLocal, setDeadlineLocal] = React.useState(
    isoToLocalInput(tugas.deadline),
  );
  const [izinkanLate, setIzinkanLate] = React.useState(tugas.izinkan_late);
  const [penaltyPersen, setPenaltyPersen] = React.useState(
    tugas.penalty_persen,
  );
  const [wajibAttachment, setWajibAttachment] = React.useState(
    tugas.wajib_attachment,
  );
  const [bobot, setBobot] = React.useState(tugas.bobot ?? 100);
  const [status, setStatus] = React.useState<TugasStatus>(tugas.status);
  const [judulError, setJudulError] = React.useState<string | null>(null);
  const [penaltyError, setPenaltyError] = React.useState<string | null>(null);

  // Re-sync setiap dialog di-open atau tugas berubah (mis. abis 409 refetch).
  React.useEffect(() => {
    if (open) {
      setJudul(tugas.judul);
      setDeskripsi(tugas.deskripsi);
      setDeadlineLocal(isoToLocalInput(tugas.deadline));
      setIzinkanLate(tugas.izinkan_late);
      setPenaltyPersen(tugas.penalty_persen);
      setWajibAttachment(tugas.wajib_attachment);
      setBobot(tugas.bobot ?? 100);
      setStatus(tugas.status);
      setJudulError(null);
      setPenaltyError(null);
    }
  }, [
    open,
    tugas.id,
    tugas.judul,
    tugas.deskripsi,
    tugas.deadline,
    tugas.izinkan_late,
    tugas.penalty_persen,
    tugas.wajib_attachment,
    tugas.bobot,
    tugas.status,
    tugas.version,
  ]);

  const attachmentsKey = React.useMemo(
    () => ['guru', 'tugas', 'attachments', tugas.id] as const,
    [tugas.id],
  );

  const attachmentsQuery = useQuery({
    queryKey: attachmentsKey,
    queryFn: () => listAttachments(tugas.id),
    enabled: open,
    staleTime: 10_000,
  });

  const updateMutation = useMutation({
    mutationFn: () => {
      const trimmedJudul = judul.trim();
      const newDeadlineISO = deadlineLocal
        ? new Date(deadlineLocal).toISOString()
        : null;
      const oldDeadlineISO = tugas.deadline ?? null;
      // Kirim hanya field yang berubah supaya backend audit bisa decide
      // archived vs updated.
      return updateTugas(tugas.id, {
        version: tugas.version,
        judul: trimmedJudul !== tugas.judul ? trimmedJudul : undefined,
        deskripsi: deskripsi !== tugas.deskripsi ? deskripsi : undefined,
        deadline:
          newDeadlineISO !== oldDeadlineISO ? newDeadlineISO : undefined,
        izinkan_late:
          izinkanLate !== tugas.izinkan_late ? izinkanLate : undefined,
        penalty_persen:
          penaltyPersen !== tugas.penalty_persen ? penaltyPersen : undefined,
        wajib_attachment:
          wajibAttachment !== tugas.wajib_attachment
            ? wajibAttachment
            : undefined,
        bobot: bobot !== (tugas.bobot ?? 100) ? bobot : undefined,
        status: status !== tugas.status ? status : undefined,
      });
    },
    onSuccess: ({ tugas: updated }) => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      const archivedNow =
        updated.status === 'archived' && tugas.status !== 'archived';
      const publishedNow =
        updated.status === 'published' && tugas.status !== 'published';
      toast({
        title: archivedNow
          ? 'Tugas diarsipkan'
          : publishedNow
            ? 'Tugas dipublish'
            : 'Tugas diperbarui',
        description: archivedNow
          ? `"${updated.judul}" disembunyiin dari siswa.`
          : publishedNow
            ? `"${updated.judul}" terbit ke siswa enrolled.`
            : `"${updated.judul}" tersimpan (versi ${updated.version}).`,
      });
      onOpenChange(false);
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      if (apiErr?.code === 'version_conflict') {
        for (const key of invalidateKeys) {
          queryClient.invalidateQueries({ queryKey: key });
        }
      }
      const message = apiErr
        ? friendlyTugasError(apiErr, 'update')
        : 'Gagal menyimpan perubahan.';
      const requestId = apiErr?.requestId;
      toast({
        title:
          apiErr?.code === 'version_conflict'
            ? 'Tugas sudah berubah'
            : 'Gagal menyimpan tugas',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const uploadMutation = useMutation({
    mutationFn: (file: File) => uploadAttachment({ tugasID: tugas.id, file }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: attachmentsKey });
      // Detail tugas attachments embedded juga perlu refresh.
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: 'Lampiran terunggah',
        description: 'File berhasil diunggah ke object store.',
      });
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyTugasError(apiErr, 'upload-attachment')
        : 'Gagal mengunggah lampiran.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal mengunggah',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const deleteAttachmentMutation = useMutation({
    mutationFn: (attID: string) => deleteAttachment(tugas.id, attID),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: attachmentsKey });
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title: 'Lampiran dihapus',
        description: 'File sudah dihapus dari object store.',
      });
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyTugasError(apiErr, 'delete-attachment')
        : 'Gagal menghapus lampiran.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal menghapus lampiran',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const isPending = updateMutation.isPending;
  const isUploading = uploadMutation.isPending;

  function validate(): boolean {
    const trimmed = judul.trim();
    if (!trimmed) {
      setJudulError('Judul wajib diisi.');
      return false;
    }
    if (trimmed.length > MAX_TUGAS_JUDUL_LENGTH) {
      setJudulError(`Judul maksimal ${MAX_TUGAS_JUDUL_LENGTH} karakter.`);
      return false;
    }
    setJudulError(null);
    if (izinkanLate && (penaltyPersen < 0 || penaltyPersen > 100)) {
      setPenaltyError('Penalty harus 0-100.');
      return false;
    }
    setPenaltyError(null);
    return true;
  }

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!validate()) return;

    const sizeBytes = new TextEncoder().encode(deskripsi).length;
    if (sizeBytes > MAX_TUGAS_DESKRIPSI_BYTES) {
      toast({
        title: 'Deskripsi terlalu panjang',
        description: `Isi melebihi batas ${MAX_TUGAS_DESKRIPSI_BYTES / 1024} KB.`,
        variant: 'destructive',
      });
      return;
    }

    if (deadlineLocal) {
      const dt = new Date(deadlineLocal);
      if (Number.isNaN(dt.getTime())) {
        toast({
          title: 'Deadline tidak valid',
          description: 'Pilih tanggal & jam dengan format yang benar.',
          variant: 'destructive',
        });
        return;
      }
    }

    updateMutation.mutate();
  }

  function onUploadFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = ''; // allow re-upload same filename
    if (!file) return;
    if (file.size > MAX_TUGAS_ATTACHMENT_BYTES) {
      toast({
        title: 'File terlalu besar',
        description: `Maksimal ${MAX_TUGAS_ATTACHMENT_BYTES / (1024 * 1024)} MB per attachment.`,
        variant: 'destructive',
      });
      return;
    }
    uploadMutation.mutate(file);
  }

  async function openAttachmentURL(att: TugasAttachment) {
    try {
      const res = await getAttachmentURL(tugas.id, att.id);
      window.open(res.url, '_blank', 'noopener,noreferrer');
    } catch (err) {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyTugasError(apiErr, 'attachment-url')
        : 'Gagal mendapatkan URL attachment.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal membuka lampiran',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    }
  }

  const oldDeadlineISO = tugas.deadline ?? null;
  const newDeadlineISO = deadlineLocal
    ? (() => {
        try {
          return new Date(deadlineLocal).toISOString();
        } catch {
          return deadlineLocal;
        }
      })()
    : null;
  const dirty =
    judul.trim() !== tugas.judul ||
    deskripsi !== tugas.deskripsi ||
    newDeadlineISO !== oldDeadlineISO ||
    izinkanLate !== tugas.izinkan_late ||
    penaltyPersen !== tugas.penalty_persen ||
    wajibAttachment !== tugas.wajib_attachment ||
    bobot !== (tugas.bobot ?? 100) ||
    status !== tugas.status;

  const submitDisabled = isPending || !judul.trim() || !dirty;

  const attachments = attachmentsQuery.data?.items ?? [];
  const attachmentLimitReached = attachments.length >= MAX_TUGAS_ATTACHMENTS;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ClipboardList className="size-5 text-muted-foreground" aria-hidden />
            Edit tugas
          </DialogTitle>
          <DialogDescription>
            Versi saat ini: {tugas.version}. Status archive nyembunyiin tugas
            dari siswa, tapi masih lu lihat di list guru. Lampiran dikelola
            terpisah di bawah.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="tugas-edit-judul">Judul</Label>
            <Input
              id="tugas-edit-judul"
              value={judul}
              onChange={(e) => {
                setJudul(e.target.value);
                if (judulError) setJudulError(null);
              }}
              disabled={isPending}
              maxLength={MAX_TUGAS_JUDUL_LENGTH}
              autoFocus
              aria-invalid={!!judulError}
            />
            {judulError && (
              <p className="text-xs text-destructive">{judulError}</p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="tugas-edit-deskripsi">Deskripsi (markdown)</Label>
            <MarkdownEditor
              id="tugas-edit-deskripsi"
              value={deskripsi}
              onChange={setDeskripsi}
              disabled={isPending}
              rows={6}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="tugas-edit-deadline">Deadline</Label>
            <div className="flex items-center gap-2">
              <Input
                id="tugas-edit-deadline"
                type="datetime-local"
                value={deadlineLocal}
                onChange={(e) => setDeadlineLocal(e.target.value)}
                disabled={isPending}
              />
              {deadlineLocal && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setDeadlineLocal('')}
                  disabled={isPending}
                >
                  Clear
                </Button>
              )}
            </div>
            <p className="text-xs text-muted-foreground">
              Kosongkan untuk tugas always-open (tanpa batas waktu).
            </p>
          </div>

          <div className="space-y-2 rounded-md border p-3">
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                className="size-4 rounded border-input"
                checked={izinkanLate}
                onChange={(e) => setIzinkanLate(e.target.checked)}
                disabled={isPending}
              />
              <span className="text-sm">Izinkan submit telat</span>
            </label>
            {izinkanLate && (
              <div className="ml-6 space-y-1.5">
                <Label htmlFor="tugas-edit-penalty">Penalty (% dari nilai)</Label>
                <Input
                  id="tugas-edit-penalty"
                  type="number"
                  min={0}
                  max={100}
                  value={penaltyPersen}
                  onChange={(e) => {
                    const n = Number(e.target.value);
                    setPenaltyPersen(Number.isFinite(n) ? n : 0);
                    if (penaltyError) setPenaltyError(null);
                  }}
                  disabled={isPending}
                  className="max-w-[120px]"
                  aria-invalid={!!penaltyError}
                />
                {penaltyError && (
                  <p className="text-xs text-destructive">{penaltyError}</p>
                )}
              </div>
            )}
          </div>

          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              className="size-4 rounded border-input"
              checked={wajibAttachment}
              onChange={(e) => setWajibAttachment(e.target.checked)}
              disabled={isPending}
            />
            <span className="text-sm">
              Siswa wajib upload minimal 1 lampiran saat submit
            </span>
          </label>

          <div className="space-y-1.5">
            <Label htmlFor="tugas-edit-bobot">Bobot tugas</Label>
            <Input
              id="tugas-edit-bobot"
              type="number"
              min={0}
              value={bobot}
              onChange={(e) => {
                const n = Number(e.target.value);
                setBobot(Number.isFinite(n) ? Math.max(0, n) : 0);
              }}
              disabled={isPending}
              className="max-w-[140px]"
            />
            <p className="text-xs text-muted-foreground">
              Nilai tugas di bab ini dirata-rata sesuai bobot masing-masing.
            </p>
          </div>

          <div className="space-y-1.5">
            <Label>Status</Label>
            <div className="grid gap-2 sm:grid-cols-3">
              <StatusOption
                active={status === 'draft'}
                onClick={() => setStatus('draft')}
                disabled={isPending}
                title="Draft"
                desc="Hanya guru yang lihat."
                Icon={FileText}
              />
              <StatusOption
                active={status === 'published'}
                onClick={() => setStatus('published')}
                disabled={isPending}
                title="Published"
                desc="Tampil ke siswa enrolled."
                Icon={ClipboardList}
              />
              <StatusOption
                active={status === 'archived'}
                onClick={() => setStatus('archived')}
                disabled={isPending}
                title="Archived"
                desc="Disembunyiin dari siswa."
                Icon={Archive}
              />
            </div>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isPending}
            >
              Tutup
            </Button>
            <Button type="submit" disabled={submitDisabled}>
              {isPending ? 'Menyimpan…' : 'Simpan'}
            </Button>
          </DialogFooter>
        </form>

        {/* Attachment manager */}
        <div className="mt-2 space-y-3 border-t pt-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <h3 className="text-sm font-medium">
                Lampiran ({attachments.length}/{MAX_TUGAS_ATTACHMENTS})
              </h3>
              <p className="text-xs text-muted-foreground">
                PDF/DOCX/JPG/PNG/ZIP, maksimal{' '}
                {MAX_TUGAS_ATTACHMENT_BYTES / (1024 * 1024)} MB per file.
              </p>
            </div>
            <label
              className={cn(
                'inline-flex cursor-pointer items-center gap-2 rounded-md border bg-background px-3 py-1.5 text-sm transition-colors hover:bg-muted/40',
                (attachmentLimitReached || isUploading) &&
                  'pointer-events-none opacity-60',
              )}
            >
              {isUploading ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Upload className="size-4" />
              )}
              {isUploading ? 'Mengunggah…' : 'Upload'}
              <input
                type="file"
                accept={TUGAS_ATTACHMENT_ACCEPT}
                className="sr-only"
                onChange={onUploadFile}
                disabled={attachmentLimitReached || isUploading}
              />
            </label>
          </div>

          {attachmentsQuery.isPending && (
            <div className="space-y-1.5">
              <div className="h-10 animate-pulse rounded-md border bg-muted/40" />
              <div className="h-10 animate-pulse rounded-md border bg-muted/40" />
            </div>
          )}

          {attachmentsQuery.isError && (
            <div className="rounded-md border border-destructive/50 bg-destructive/5 p-3 text-sm text-destructive">
              Gagal memuat lampiran.{' '}
              <Button
                variant="link"
                size="sm"
                className="h-auto p-0 align-baseline"
                onClick={() => attachmentsQuery.refetch()}
              >
                Coba lagi
              </Button>
              .
            </div>
          )}

          {attachmentsQuery.isSuccess && attachments.length === 0 && (
            <div className="rounded-md border border-dashed p-4 text-center text-sm text-muted-foreground">
              Belum ada lampiran. Klik tombol Upload untuk menambahkan.
            </div>
          )}

          {attachmentsQuery.isSuccess && attachments.length > 0 && (
            <ul className="space-y-1.5">
              {attachments.map((att) => (
                <li
                  key={att.id}
                  className="flex items-center justify-between gap-2 rounded-md border bg-background px-3 py-2"
                >
                  <div className="flex min-w-0 items-center gap-2">
                    <FileText className="size-4 shrink-0 text-muted-foreground" />
                    <div className="min-w-0">
                      <p
                        className="truncate text-sm font-medium"
                        title={att.original_filename}
                      >
                        {att.original_filename}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {att.mime_type} · {formatBytes(att.size_bytes)}
                      </p>
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => openAttachmentURL(att)}
                      aria-label={`Buka ${att.original_filename}`}
                    >
                      <Download className="size-4" />
                      Buka
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                      onClick={() => {
                        if (
                          window.confirm(
                            `Hapus lampiran "${att.original_filename}"? Tidak bisa di-undo.`,
                          )
                        ) {
                          deleteAttachmentMutation.mutate(att.id);
                        }
                      }}
                      disabled={deleteAttachmentMutation.isPending}
                      aria-label={`Hapus ${att.original_filename}`}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

interface StatusOptionProps {
  active: boolean;
  onClick: () => void;
  disabled?: boolean;
  title: string;
  desc: string;
  Icon: React.ComponentType<{ className?: string }>;
}

function StatusOption({
  active,
  onClick,
  disabled,
  title,
  desc,
  Icon,
}: StatusOptionProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={cn(
        'flex items-start gap-2 rounded-md border p-3 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-60',
        active
          ? 'border-primary bg-primary/5 ring-1 ring-primary'
          : 'border-input bg-background hover:bg-muted/40',
      )}
    >
      <Icon
        className={cn(
          'mt-0.5 size-4 shrink-0',
          active ? 'text-primary' : 'text-muted-foreground',
        )}
      />
      <div className="min-w-0">
        <p className="text-sm font-medium">{title}</p>
        <p className="text-xs text-muted-foreground">{desc}</p>
      </div>
    </button>
  );
}
