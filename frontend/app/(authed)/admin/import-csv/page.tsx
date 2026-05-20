'use client';

/**
 * /admin/import-csv — bulk-import CSV admin page (Tasks 2.E.1 + 2.E.2 + 2.E.3).
 *
 * Single-page state machine driven by `?job_id=` query string (mirrors the
 * /admin/pengguna/detail pattern — Next 14 static export forbids dynamic
 * route segments without generateStaticParams). Three modes:
 *
 *   1. Empty (no job_id)             → drag-drop upload card
 *   2. Preview (status=preview)      → preview table + Cancel + Confirm
 *   3. Done (status=completed)       → success modal + Download credentials
 *      Other terminal states (cancelled/expired/failed) → empty CTA "upload baru"
 *
 * Backend contracts: see `@/lib/import-api`.
 *
 * Pitfalls:
 *   - Multipart uploads cannot reuse the JSON `api()` wrapper — uploadImportCSV
 *     hand-rolls fetch with bearer header.
 *   - Credentials.csv download follows a 302 to a short-lived (15m) R2
 *     presigned URL. We open it in a new tab so the browser saves the file
 *     directly (Content-Disposition: attachment).
 *   - `?job_id=` deep links must validate via GET preview before rendering;
 *     410 expired or 409 not_in_preview drops the param and shows empty state.
 */

import * as React from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  AlertCircle,
  CheckCircle2,
  CloudUpload,
  Download,
  FileSpreadsheet,
  Loader2,
  RotateCcw,
  Trash2,
  Upload,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type ConfirmResponse,
  type ImportRow,
  type ImportRowStatus,
  type PreviewResponse,
  cancelImport,
  confirmImport,
  confirmReasonLabel,
  downloadCredentialsCSV,
  getImportPreview,
  uploadImportCSV,
} from '@/lib/import-api';
import { cn } from '@/lib/utils';
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

const MAX_BYTES = 5 * 1024 * 1024; // matches backend MaxCSVBytes (5MB)

const rowStatusTone: Record<ImportRowStatus, string> = {
  valid: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400',
  invalid: 'bg-rose-500/15 text-rose-700 dark:text-rose-400',
  duplicate: 'bg-amber-500/15 text-amber-700 dark:text-amber-400',
};

const rowStatusLabel: Record<ImportRowStatus, string> = {
  valid: 'Valid',
  invalid: 'Invalid',
  duplicate: 'Duplikat',
};

function formatExpires(input?: string | null): string {
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

function apiErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError) {
    const reqId = err.requestId ? ` (req: ${err.requestId})` : '';
    return `${err.message}${reqId}`;
  }
  if (err instanceof Error) return err.message;
  return fallback;
}

export default function AdminImportCSVPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const jobIDFromUrl = searchParams.get('job_id') ?? '';
  const queryClient = useQueryClient();
  const { toast } = useToast();

  const [confirmResult, setConfirmResult] = React.useState<ConfirmResponse | null>(null);

  // ----- Resume preview when ?job_id=… is present -----
  const previewQuery = useQuery({
    queryKey: ['admin', 'import-csv', jobIDFromUrl],
    queryFn: () => getImportPreview(jobIDFromUrl),
    enabled: Boolean(jobIDFromUrl),
    retry: false,
    staleTime: 5_000,
  });

  // Drop ?job_id from the URL when the resume returns a non-preview status
  // (expired / cancelled / completed-but-no-cached-result / etc).
  React.useEffect(() => {
    if (!previewQuery.error) return;
    const err = previewQuery.error;
    if (err instanceof ApiError) {
      const drop =
        err.status === 410 ||
        err.code === 'preview_expired' ||
        err.code === 'not_in_preview' ||
        err.code === 'not_found';
      if (drop) {
        toast({
          title: 'Preview tidak tersedia',
          description: err.message,
          variant: 'destructive',
        });
        router.replace('/admin/import-csv');
      }
    }
  }, [previewQuery.error, router, toast]);

  // ----- Upload mutation -----
  const uploadMut = useMutation({
    mutationFn: (file: File) => uploadImportCSV(file),
    onSuccess: (data) => {
      toast({
        title: 'Upload berhasil',
        description: `${data.valid_count} valid, ${data.invalid_count} ditolak. Cek preview di bawah.`,
      });
      queryClient.setQueryData(['admin', 'import-csv', data.job_id], data);
      router.replace(`/admin/import-csv?job_id=${data.job_id}`);
    },
    onError: (err) => {
      toast({
        title: 'Upload gagal',
        description: apiErrorMessage(err, 'Tidak dapat upload CSV.'),
        variant: 'destructive',
      });
    },
  });

  // ----- Cancel mutation -----
  const cancelMut = useMutation({
    mutationFn: (jobID: string) => cancelImport(jobID),
    onSuccess: () => {
      toast({ title: 'Preview dibatalkan' });
      router.replace('/admin/import-csv');
    },
    onError: (err) => {
      toast({
        title: 'Gagal cancel',
        description: apiErrorMessage(err, 'Cancel gagal.'),
        variant: 'destructive',
      });
    },
  });

  // ----- Confirm mutation -----
  const confirmMut = useMutation({
    mutationFn: (jobID: string) => confirmImport(jobID),
    onSuccess: (data) => {
      setConfirmResult(data);
      queryClient.invalidateQueries({ queryKey: ['admin', 'import-csv'] });
    },
    onError: (err) => {
      toast({
        title: 'Confirm gagal',
        description: apiErrorMessage(err, 'Tidak dapat finalisasi import.'),
        variant: 'destructive',
      });
    },
  });

  const preview = previewQuery.data;
  const showUploadCard = !jobIDFromUrl || (previewQuery.isError && !previewQuery.isFetching);

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">Import Pengguna (CSV)</h1>
          <p className="text-sm text-muted-foreground">
            Upload file CSV berisi <code className="rounded bg-muted px-1 text-xs">nama,email,kode_kelas</code>{' '}
            untuk membuat banyak akun siswa sekaligus. Preview otomatis kadaluarsa 1 jam.
          </p>
        </div>
        {jobIDFromUrl ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => router.replace('/admin/import-csv')}
          >
            <RotateCcw className="size-4" />
            Mulai upload baru
          </Button>
        ) : null}
      </header>

      {showUploadCard ? (
        <UploadCard
          uploading={uploadMut.isPending}
          onPick={(file) => uploadMut.mutate(file)}
        />
      ) : null}

      {jobIDFromUrl && previewQuery.isPending ? (
        <Card>
          <CardContent className="flex items-center gap-3 py-12 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
            Memuat preview…
          </CardContent>
        </Card>
      ) : null}

      {preview ? (
        <PreviewCard
          preview={preview}
          onCancel={() => cancelMut.mutate(preview.job_id)}
          cancelPending={cancelMut.isPending}
          onConfirm={() => confirmMut.mutate(preview.job_id)}
          confirmPending={confirmMut.isPending}
        />
      ) : null}

      <SuccessDialog
        result={confirmResult}
        onClose={() => {
          setConfirmResult(null);
          router.replace('/admin/import-csv');
        }}
      />
    </div>
  );
}

// ============================================================================
// Upload card (drag + drop, file input)
// ============================================================================

interface UploadCardProps {
  uploading: boolean;
  onPick: (file: File) => void;
}

function UploadCard({ uploading, onPick }: UploadCardProps) {
  const inputRef = React.useRef<HTMLInputElement>(null);
  const [hover, setHover] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  const validate = (file: File): string | null => {
    if (!/\.csv$/i.test(file.name) && file.type && !file.type.includes('csv') && !file.type.includes('text')) {
      return 'Hanya file CSV yang diterima (.csv)';
    }
    if (file.size > MAX_BYTES) {
      return `File melebihi batas ${(MAX_BYTES / 1024 / 1024).toFixed(0)} MB`;
    }
    if (file.size === 0) {
      return 'File kosong';
    }
    return null;
  };

  const handleFile = (file: File) => {
    const err = validate(file);
    if (err) {
      setError(err);
      return;
    }
    setError(null);
    onPick(file);
  };

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setHover(false);
    const file = e.dataTransfer.files?.[0];
    if (file) handleFile(file);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Upload CSV</CardTitle>
        <CardDescription>
          Header wajib: <code className="text-xs">nama, email</code>. Kolom <code className="text-xs">kode_kelas</code>{' '}
          opsional — kalau diisi, siswa langsung di-enroll ke kelas tersebut.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <button
          type="button"
          disabled={uploading}
          onClick={() => inputRef.current?.click()}
          onDragOver={(e) => {
            e.preventDefault();
            setHover(true);
          }}
          onDragLeave={() => setHover(false)}
          onDrop={onDrop}
          className={cn(
            'flex w-full flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed p-10 text-sm transition-colors',
            hover
              ? 'border-primary/60 bg-primary/5'
              : 'border-input hover:bg-muted/40',
            uploading && 'pointer-events-none opacity-60',
          )}
        >
          {uploading ? (
            <>
              <Loader2 className="size-7 animate-spin text-muted-foreground" />
              <div className="text-muted-foreground">Mengupload + parsing…</div>
            </>
          ) : (
            <>
              <CloudUpload className="size-7 text-muted-foreground" />
              <div className="font-medium">Drag &amp; drop CSV ke sini</div>
              <div className="text-xs text-muted-foreground">
                atau klik untuk pilih file (max {(MAX_BYTES / 1024 / 1024).toFixed(0)} MB)
              </div>
            </>
          )}
        </button>
        <input
          ref={inputRef}
          type="file"
          accept=".csv,text/csv,text/plain"
          className="hidden"
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file) handleFile(file);
            e.target.value = ''; // allow re-pick same filename
          }}
        />
        {error ? (
          <div className="flex items-center gap-2 rounded-md border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm text-destructive">
            <AlertCircle className="size-4" />
            {error}
          </div>
        ) : null}
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer">Contoh format CSV</summary>
          <pre className="mt-2 overflow-x-auto rounded bg-muted p-3 text-xs">
{`nama,email,kode_kelas
Budi Santoso,budi@sekolah.id,X-IPA-1
Sari Wati,sari@sekolah.id,
Andi Pratama,andi@sekolah.id,X-IPS-2`}
          </pre>
        </details>
      </CardContent>
    </Card>
  );
}

// ============================================================================
// Preview card (table + Cancel/Confirm buttons)
// ============================================================================

interface PreviewCardProps {
  preview: PreviewResponse;
  onCancel: () => void;
  cancelPending: boolean;
  onConfirm: () => void;
  confirmPending: boolean;
}

function PreviewCard({
  preview,
  onCancel,
  cancelPending,
  onConfirm,
  confirmPending,
}: PreviewCardProps) {
  const status = preview.status ?? 'preview';
  const isPreview = status === 'preview';
  const validRows = preview.preview_rows.filter((r) => r.status === 'valid').length;
  const previewedTotal = preview.preview_rows.length;
  const trimmed = previewedTotal < preview.total_rows;

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <CardTitle className="text-base">
            Preview · {preview.filename ?? 'upload.csv'}
          </CardTitle>
          <CardDescription>
            <span className="text-emerald-600 dark:text-emerald-400">
              {preview.valid_count} valid
            </span>{' '}
            ·{' '}
            <span className="text-rose-600 dark:text-rose-400">
              {preview.invalid_count} ditolak
            </span>{' '}
            · total {preview.total_rows} baris
            {isPreview ? <> · expired {formatExpires(preview.expires_at)}</> : null}
            {!isPreview ? <> · status {status}</> : null}
          </CardDescription>
        </div>
        <div className="flex flex-wrap gap-2">
          {isPreview ? (
            <>
              <Button
                variant="outline"
                size="sm"
                onClick={onCancel}
                disabled={cancelPending || confirmPending}
              >
                <Trash2 className="size-4" />
                Cancel
              </Button>
              <Button
                size="sm"
                onClick={onConfirm}
                disabled={confirmPending || cancelPending || preview.valid_count === 0}
              >
                {confirmPending ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
                Konfirmasi &amp; buat akun
              </Button>
            </>
          ) : null}
        </div>
      </CardHeader>
      <CardContent>
        <div className="overflow-x-auto rounded-md border">
          <table className="w-full text-sm">
            <thead className="bg-muted/40 text-left text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium w-12">Baris</th>
                <th className="px-3 py-2 font-medium">Nama</th>
                <th className="px-3 py-2 font-medium">Email</th>
                <th className="px-3 py-2 font-medium">Kode kelas</th>
                <th className="px-3 py-2 font-medium">Status</th>
                <th className="px-3 py-2 font-medium">Catatan</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {preview.preview_rows.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-3 py-10 text-center text-sm text-muted-foreground">
                    CSV tidak menghasilkan baris yang bisa di-preview.
                  </td>
                </tr>
              ) : (
                preview.preview_rows.map((row) => <PreviewRow key={row.line_no} row={row} />)
              )}
            </tbody>
          </table>
        </div>
        {trimmed ? (
          <p className="mt-3 text-xs text-muted-foreground">
            Menampilkan {previewedTotal} dari {preview.total_rows} baris (preview dipotong;
            confirm tetap memproses semua baris dari CSV asli).
          </p>
        ) : null}
        {isPreview && validRows === 0 ? (
          <p className="mt-3 text-xs text-amber-600 dark:text-amber-400">
            Tidak ada baris valid — perbaiki CSV-nya lalu upload ulang.
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}

function PreviewRow({ row }: { row: ImportRow }) {
  return (
    <tr className="hover:bg-muted/30">
      <td className="px-3 py-2 text-muted-foreground">{row.line_no}</td>
      <td className="px-3 py-2 font-medium">{row.nama || '—'}</td>
      <td className="px-3 py-2 break-all text-muted-foreground">{row.email || '—'}</td>
      <td className="px-3 py-2 text-muted-foreground">{row.kode_kelas || '—'}</td>
      <td className="px-3 py-2">
        <span
          className={cn(
            'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
            rowStatusTone[row.status],
          )}
        >
          {rowStatusLabel[row.status]}
        </span>
      </td>
      <td className="px-3 py-2 text-xs text-muted-foreground">
        {row.errors && row.errors.length > 0 ? row.errors.join('; ') : '—'}
      </td>
    </tr>
  );
}

// ============================================================================
// Success dialog (Task 2.E.3)
// ============================================================================

interface SuccessDialogProps {
  result: ConfirmResponse | null;
  onClose: () => void;
}

function SuccessDialog({ result, onClose }: SuccessDialogProps) {
  const { toast } = useToast();
  const [downloading, setDownloading] = React.useState(false);

  const onDownload = async () => {
    if (!result) return;
    setDownloading(true);
    try {
      const url = await downloadCredentialsCSV(result.job_id);
      // Open in new tab so the browser persists the download via the
      // attachment Content-Disposition embedded in the presigned URL.
      window.open(url, '_blank', 'noopener,noreferrer');
    } catch (err) {
      toast({
        title: 'Gagal generate link',
        description: apiErrorMessage(err, 'Tidak dapat unduh credentials.csv.'),
        variant: 'destructive',
      });
    } finally {
      setDownloading(false);
    }
  };

  const open = result !== null;

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <CheckCircle2 className="size-5 text-emerald-500" />
            Import selesai
          </DialogTitle>
          <DialogDescription>
            {result?.success_count ?? 0} akun berhasil dibuat
            {result && result.fail_count > 0 ? `, ${result.fail_count} gagal.` : '.'}{' '}
            Download credentials.csv sekarang — file ini berisi password
            plaintext dan otomatis terhapus setelah 1 jam.
          </DialogDescription>
        </DialogHeader>

        {result && result.failures.length > 0 ? (
          <div className="space-y-2">
            <div className="text-xs font-medium text-muted-foreground">
              Baris gagal ({result.failures.length})
            </div>
            <div className="max-h-48 overflow-auto rounded-md border">
              <table className="w-full text-xs">
                <thead className="bg-muted/40 text-left uppercase text-muted-foreground">
                  <tr>
                    <th className="px-2 py-1 w-10">#</th>
                    <th className="px-2 py-1">Email</th>
                    <th className="px-2 py-1">Alasan</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {result.failures.map((f, i) => (
                    <tr key={`${f.line_no}-${i}`}>
                      <td className="px-2 py-1 text-muted-foreground">{f.line_no}</td>
                      <td className="px-2 py-1 break-all">{f.email || '—'}</td>
                      <td className="px-2 py-1 text-muted-foreground">
                        {confirmReasonLabel[f.reason] ?? f.reason}
                        {f.detail ? ` — ${f.detail}` : ''}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ) : null}

        <DialogFooter className="gap-2 sm:gap-2">
          <Button variant="outline" onClick={onClose} disabled={downloading}>
            Tutup
          </Button>
          <Button onClick={onDownload} disabled={downloading || !result?.credentials_object_key}>
            {downloading ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Download className="size-4" />
            )}
            Download credentials.csv
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
