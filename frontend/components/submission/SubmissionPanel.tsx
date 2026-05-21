'use client';

/**
 * SubmissionPanel — siswa-facing panel untuk satu tugas:
 *   - Header: judul + deskripsi + deadline + late banner
 *   - List lampiran soal (presigned download via getAttachmentURL tugas)
 *   - Status submission (kalau ada): version, submitted_at, status, nilai
 *   - Composer: catatan textarea + multi-file picker → submitTugas
 *
 * UX rules:
 *   - Belum submit + tugas published → tampil composer (default).
 *   - Sudah submit, status=submitted → tampil rekap + tombol "Resubmit"
 *     (toggle composer dengan pre-filled catatan).
 *   - Status=graded → tampil rekap + nilai + feedback, composer disabled
 *     (BE 409 already_graded — defensive).
 *   - is_late=true → banner "Late submission, penalty xx%".
 *   - deadline lewat + izinkan_late=false → composer disabled, banner
 *     "Deadline lewat, submit ditutup" (BE 403 deadline_passed — defensive).
 *
 * Locked decisions: #70 (single-row resubmit) | #71 (late) | #72 (attachment)
 * | #73 (already_graded guard).
 */

import * as React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Calendar,
  CheckCircle2,
  Clock,
  Download,
  FileText,
  Loader2,
  Paperclip,
  Send,
  Upload,
  X,
} from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type MySubmissionResponse,
  MAX_SUBMISSION_ATTACHMENT_BYTES,
  MAX_SUBMISSION_ATTACHMENTS,
  MAX_SUBMISSION_CATATAN_BYTES,
  SUBMISSION_ATTACHMENT_ACCEPT,
  formatSubmissionTimestamp,
  friendlySubmissionError,
  getMySubmission,
  getSubmissionAttachmentURL,
  isTugasOverdue,
  statusLabel,
  submitTugas,
} from '@/lib/submission-api';
import {
  formatDeadline,
  getAttachmentURL as getTugasAttachmentURL,
  listAttachments as listTugasAttachments,
  type TugasAttachment,
} from '@/lib/tugas-api';
import { useToast } from '@/hooks/use-toast';

import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';

export const submissionQueryKey = (tugasID: string) =>
  ['submission', 'mine', tugasID] as const;

export const tugasAttachmentsQueryKey = (tugasID: string) =>
  ['tugas', tugasID, 'attachments'] as const;

interface SubmissionPanelProps {
  tugasID: string;
  /** Optional initial deskripsi (kalau parent sudah hydrate dari listSiswaTugas). */
  initialDeskripsi?: string;
}

export function SubmissionPanel({ tugasID, initialDeskripsi }: SubmissionPanelProps) {
  const { toast } = useToast();
  const qc = useQueryClient();

  const myQuery = useQuery({
    queryKey: submissionQueryKey(tugasID),
    queryFn: () => getMySubmission(tugasID),
  });

  const tugasAttQuery = useQuery({
    queryKey: tugasAttachmentsQueryKey(tugasID),
    queryFn: () => listTugasAttachments(tugasID),
    enabled: Boolean(myQuery.data),
  });

  const [catatan, setCatatan] = React.useState('');
  const [files, setFiles] = React.useState<File[]>([]);
  const [composerOpen, setComposerOpen] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  // Sync catatan dari existing submission when first hydrate.
  React.useEffect(() => {
    if (myQuery.data?.submission && !composerOpen) {
      setCatatan(myQuery.data.submission.catatan ?? '');
    }
  }, [myQuery.data, composerOpen]);

  const submitMu = useMutation({
    mutationFn: () =>
      submitTugas({
        tugasID,
        catatan,
        files,
      }),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: submissionQueryKey(tugasID) });
      setFiles([]);
      setComposerOpen(false);
      setError(null);
      toast({
        title: data.is_resubmit ? 'Submission ter-update' : 'Tugas terkirim',
        description: data.is_resubmit
          ? 'Versi baru sudah ke-save. Tunggu guru kasih nilai.'
          : 'Tugas lu sudah terkirim. Tunggu guru kasih nilai.',
      });
    },
    onError: (err: unknown) => {
      const apiErr = err instanceof ApiError ? err : null;
      const friendly = apiErr
        ? friendlySubmissionError(apiErr, files.length > 0 ? 'submit' : 'resubmit')
        : err instanceof Error
          ? err.message
          : 'Gagal kirim tugas.';
      setError(friendly);
      toast({
        title: 'Gagal kirim',
        description: friendly,
        variant: 'destructive',
      });
    },
  });

  if (myQuery.isPending) {
    return (
      <Card>
        <CardContent className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" />
          Memuat tugas…
        </CardContent>
      </Card>
    );
  }

  if (myQuery.isError) {
    const err = myQuery.error;
    const apiErr = err instanceof ApiError ? err : null;
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Gagal memuat tugas</CardTitle>
          <CardDescription>
            {apiErr ? friendlySubmissionError(apiErr, 'get') : (err as Error).message}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => myQuery.refetch()}
          >
            Coba lagi
          </Button>
        </CardContent>
      </Card>
    );
  }

  const data = myQuery.data as MySubmissionResponse;
  const tugas = data.tugas;
  const sub = data.submission;
  const overdue = isTugasOverdue(tugas);
  const blockedSubmit = overdue && !tugas.izinkan_late;
  const isGraded = sub?.status === 'graded';

  const onPickFiles = (input: FileList | null) => {
    if (!input) return;
    const next = [...files];
    for (let i = 0; i < input.length; i++) {
      const f = input[i];
      if (!f) continue;
      if (next.length >= MAX_SUBMISSION_ATTACHMENTS) break;
      if (f.size > MAX_SUBMISSION_ATTACHMENT_BYTES) {
        setError(
          `File ${f.name} melebihi batas ${
            MAX_SUBMISSION_ATTACHMENT_BYTES / (1024 * 1024)
          } MB`,
        );
        continue;
      }
      next.push(f);
    }
    setFiles(next.slice(0, MAX_SUBMISSION_ATTACHMENTS));
  };

  const removeFileAt = (i: number) => {
    setFiles((prev) => prev.filter((_, idx) => idx !== i));
  };

  const onSubmitClick = () => {
    if (catatan.length > MAX_SUBMISSION_CATATAN_BYTES) {
      setError(`Catatan melebihi batas ${MAX_SUBMISSION_CATATAN_BYTES / 1024} KB.`);
      return;
    }
    if (tugas.wajib_attachment && files.length === 0) {
      setError('Tugas ini wajib upload minimal 1 lampiran.');
      return;
    }
    setError(null);
    submitMu.mutate();
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">{tugas.judul}</CardTitle>
          <CardDescription className="flex flex-wrap items-center gap-3 text-xs">
            <span className="inline-flex items-center gap-1">
              <Calendar className="size-3.5" />
              Deadline: {formatDeadline(tugas.deadline)}
            </span>
            {tugas.izinkan_late && tugas.penalty_persen > 0 && (
              <span className="inline-flex items-center gap-1 text-amber-600">
                <Clock className="size-3.5" />
                Late penalty {tugas.penalty_persen}%
              </span>
            )}
            {tugas.wajib_attachment && (
              <span className="inline-flex items-center gap-1 text-muted-foreground">
                <Paperclip className="size-3.5" />
                Wajib lampiran
              </span>
            )}
          </CardDescription>
        </CardHeader>
        {initialDeskripsi && initialDeskripsi.trim() && (
          <CardContent className="text-sm text-foreground/90 whitespace-pre-wrap">
            {initialDeskripsi}
          </CardContent>
        )}
      </Card>

      {/* Lampiran soal dari guru */}
      <TugasAttachments tugasID={tugasID} query={tugasAttQuery} />

      {/* Status submission existing */}
      {sub && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <CheckCircle2 className="size-4 text-emerald-600" />
              {statusLabel(sub.status)}
            </CardTitle>
            <CardDescription className="flex flex-wrap items-center gap-3 text-xs">
              <span>Versi {sub.version}</span>
              <span>Terkirim: {formatSubmissionTimestamp(sub.submitted_at)}</span>
              {sub.is_late && (
                <span className="inline-flex items-center gap-1 text-amber-600">
                  <Clock className="size-3.5" /> LATE
                </span>
              )}
              {sub.graded_at && (
                <span>Dinilai: {formatSubmissionTimestamp(sub.graded_at)}</span>
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {sub.catatan && (
              <div>
                <p className="text-xs font-medium text-muted-foreground">
                  Catatan lu
                </p>
                <p className="text-sm whitespace-pre-wrap">{sub.catatan}</p>
              </div>
            )}
            {sub.attachments && sub.attachments.length > 0 && (
              <div>
                <p className="text-xs font-medium text-muted-foreground">
                  Lampiran lu
                </p>
                <SubmissionAttachmentLinks
                  submissionID={sub.id}
                  attachments={sub.attachments}
                />
              </div>
            )}
            {isGraded && (
              <>
                <Separator />
                <div className="grid gap-2 rounded-md bg-emerald-50 p-3 text-sm dark:bg-emerald-900/20">
                  <div className="flex items-center justify-between">
                    <span className="font-medium">Nilai</span>
                    <span className="text-2xl font-bold">
                      {sub.nilai_setelah_penalty?.toFixed(2) ?? '—'}
                    </span>
                  </div>
                  {sub.is_late &&
                    sub.penalty_persen_applied != null &&
                    sub.penalty_persen_applied > 0 && (
                      <div className="text-xs text-muted-foreground">
                        Nilai asli {sub.nilai_asli?.toFixed(2)} × (1 -{' '}
                        {sub.penalty_persen_applied}%) = nilai akhir.
                      </div>
                    )}
                  {sub.feedback && (
                    <div className="space-y-1">
                      <p className="text-xs font-medium text-muted-foreground">
                        Feedback guru
                      </p>
                      <p className="whitespace-pre-wrap">{sub.feedback}</p>
                    </div>
                  )}
                </div>
              </>
            )}
          </CardContent>
        </Card>
      )}

      {/* Composer */}
      {!isGraded && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              {sub ? 'Kirim ulang (resubmit)' : 'Kirim tugas'}
            </CardTitle>
            <CardDescription>
              {blockedSubmit
                ? 'Deadline sudah lewat dan late submission tidak diizinkan.'
                : sub
                  ? 'Resubmit overwrite versi lama. Lampiran lama bakal diganti yang baru.'
                  : 'Tulis catatan + opsional upload lampiran (PDF, DOCX, JPG, PNG, ZIP).'}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {!sub || composerOpen ? (
              <>
                <div className="space-y-1.5">
                  <Label htmlFor="submission-catatan">Catatan</Label>
                  <Textarea
                    id="submission-catatan"
                    placeholder="Tulis jawaban / catatan lu di sini…"
                    value={catatan}
                    onChange={(e) => setCatatan(e.target.value)}
                    disabled={blockedSubmit || submitMu.isPending}
                    rows={6}
                  />
                  <p className="text-xs text-muted-foreground">
                    {(catatan.length / 1024).toFixed(1)} KB /{' '}
                    {MAX_SUBMISSION_CATATAN_BYTES / 1024} KB
                  </p>
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="submission-files">
                    Lampiran (opsional, max {MAX_SUBMISSION_ATTACHMENTS} file ×{' '}
                    {MAX_SUBMISSION_ATTACHMENT_BYTES / (1024 * 1024)} MB)
                  </Label>
                  <Input
                    id="submission-files"
                    type="file"
                    multiple
                    accept={SUBMISSION_ATTACHMENT_ACCEPT}
                    disabled={
                      blockedSubmit ||
                      submitMu.isPending ||
                      files.length >= MAX_SUBMISSION_ATTACHMENTS
                    }
                    onChange={(e) => {
                      onPickFiles(e.target.files);
                      e.target.value = '';
                    }}
                  />
                  {files.length > 0 && (
                    <ul className="space-y-1 rounded-md border bg-muted/30 p-2 text-sm">
                      {files.map((f, i) => (
                        <li
                          key={`${f.name}-${i}`}
                          className="flex items-center justify-between gap-2"
                        >
                          <span className="flex items-center gap-2 truncate">
                            <FileText className="size-3.5 shrink-0" />
                            <span className="truncate">{f.name}</span>
                            <span className="shrink-0 text-xs text-muted-foreground">
                              {(f.size / 1024).toFixed(1)} KB
                            </span>
                          </span>
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            onClick={() => removeFileAt(i)}
                            disabled={submitMu.isPending}
                          >
                            <X className="size-3.5" />
                          </Button>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>

                {error && (
                  <p className="text-sm text-destructive" role="alert">
                    {error}
                  </p>
                )}

                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    onClick={onSubmitClick}
                    disabled={blockedSubmit || submitMu.isPending}
                  >
                    {submitMu.isPending ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : sub ? (
                      <Upload className="size-4" />
                    ) : (
                      <Send className="size-4" />
                    )}
                    {sub ? 'Resubmit' : 'Kirim'}
                  </Button>
                  {sub && composerOpen && (
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => {
                        setComposerOpen(false);
                        setCatatan(sub.catatan ?? '');
                        setFiles([]);
                        setError(null);
                      }}
                      disabled={submitMu.isPending}
                    >
                      Batal
                    </Button>
                  )}
                </div>
              </>
            ) : (
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setComposerOpen(true);
                  setError(null);
                }}
                disabled={blockedSubmit}
              >
                <Upload className="size-4" />
                Kirim ulang
              </Button>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

/** TugasAttachments — list lampiran soal dari guru, tombol download presigned. */
function TugasAttachments({
  tugasID,
  query,
}: {
  tugasID: string;
  query: ReturnType<typeof useQuery<{ items: TugasAttachment[]; total: number }>>;
}) {
  const { toast } = useToast();
  const [downloadingID, setDownloadingID] = React.useState<string | null>(null);

  if (query.isPending) {
    return (
      <Card>
        <CardContent className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" />
          Memuat lampiran soal…
        </CardContent>
      </Card>
    );
  }

  if (query.isError) {
    return null;
  }

  const items = query.data?.items ?? [];
  if (items.length === 0) return null;

  const onDownload = async (att: TugasAttachment) => {
    setDownloadingID(att.id);
    try {
      const res = await getTugasAttachmentURL(tugasID, att.id);
      window.open(res.url, '_blank', 'noopener,noreferrer');
    } catch (err) {
      toast({
        title: 'Gagal generate link download',
        description: err instanceof Error ? err.message : 'Coba lagi.',
        variant: 'destructive',
      });
    } finally {
      setDownloadingID(null);
    }
  };

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Lampiran soal dari guru</CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="space-y-1">
          {items.map((att) => (
            <li
              key={att.id}
              className="flex items-center justify-between gap-2 rounded-md border bg-card p-2"
            >
              <span className="flex min-w-0 items-center gap-2 truncate">
                <FileText className="size-4 shrink-0" />
                <span className="truncate text-sm">{att.original_filename}</span>
                <span className="shrink-0 text-xs text-muted-foreground">
                  {(att.size_bytes / 1024).toFixed(1)} KB
                </span>
              </span>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => onDownload(att)}
                disabled={downloadingID === att.id}
              >
                {downloadingID === att.id ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : (
                  <Download className="size-3.5" />
                )}
              </Button>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}

/** Inline link list untuk submission attachments siswa (presigned download). */
function SubmissionAttachmentLinks({
  submissionID,
  attachments,
}: {
  submissionID: string;
  attachments: NonNullable<MySubmissionResponse['submission']>['attachments'];
}) {
  const { toast } = useToast();
  const [downloadingID, setDownloadingID] = React.useState<string | null>(null);

  const list = attachments ?? [];
  if (list.length === 0) return null;

  const onDownload = async (attID: string) => {
    setDownloadingID(attID);
    try {
      const res = await getSubmissionAttachmentURL(submissionID, attID);
      window.open(res.url, '_blank', 'noopener,noreferrer');
    } catch (err) {
      toast({
        title: 'Gagal generate link download',
        description: err instanceof Error ? err.message : 'Coba lagi.',
        variant: 'destructive',
      });
    } finally {
      setDownloadingID(null);
    }
  };

  return (
    <ul className="mt-1 space-y-1">
      {list.map((att) => (
        <li
          key={att.id}
          className="flex items-center justify-between gap-2 rounded-md border bg-card p-2"
        >
          <span className="flex min-w-0 items-center gap-2 truncate">
            <FileText className="size-4 shrink-0" />
            <span className="truncate text-sm">{att.original_filename}</span>
            <span className="shrink-0 text-xs text-muted-foreground">
              {(att.size_bytes / 1024).toFixed(1)} KB
            </span>
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => onDownload(att.id)}
            disabled={downloadingID === att.id}
          >
            {downloadingID === att.id ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : (
              <Download className="size-3.5" />
            )}
          </Button>
        </li>
      ))}
    </ul>
  );
}
