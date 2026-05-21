'use client';

/**
 * GradeSubmissionDialog — guru kasih nilai + feedback ke 1 submission.
 *
 * UX:
 *   - Header: judul tugas + siswa_id.
 *   - Body: catatan siswa (read-only) + lampiran siswa (presigned download)
 *     + form grade (nilai_asli 0-100 step 0.01, feedback textarea).
 *   - Penalty preview: kalau is_late + tugasPenaltyPersen > 0, tampil
 *     "Nilai akhir = nilai * (1 - X%) = Y" inline di bawah input.
 *   - Submit: POST /submission/:id/grade — server return updated row,
 *     invalidate list query.
 *   - Status=graded mode: form pre-filled + read-only (cuma "Tutup").
 */

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Download, FileText, Loader2 } from 'lucide-react';

import { ApiError } from '@/lib/api';
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
import { useToast } from '@/hooks/use-toast';

import {
  type Submission,
  MAX_SUBMISSION_FEEDBACK_BYTES,
  formatSubmissionTimestamp,
  friendlySubmissionError,
  getSubmissionAttachmentURL,
  gradeSubmission,
  previewNilaiSetelahPenalty,
} from '@/lib/submission-api';

interface GradeSubmissionDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  submission: Submission;
  tugasID: string;
  tugasJudul?: string;
  /** Penalty hint for preview. Default 0. */
  tugasPenaltyPersen?: number;
}

export function GradeSubmissionDialog({
  open,
  onOpenChange,
  submission,
  tugasID,
  tugasJudul,
  tugasPenaltyPersen = 0,
}: GradeSubmissionDialogProps) {
  const { toast } = useToast();
  const qc = useQueryClient();

  const isGraded = submission.status === 'graded';
  const [nilaiInput, setNilaiInput] = React.useState<string>(
    () => submission.nilai_asli?.toString() ?? '',
  );
  const [feedback, setFeedback] = React.useState<string>(
    submission.feedback ?? '',
  );
  const [error, setError] = React.useState<string | null>(null);

  // Re-sync state kalau submission berubah (re-open dialog dengan target lain).
  React.useEffect(() => {
    setNilaiInput(submission.nilai_asli?.toString() ?? '');
    setFeedback(submission.feedback ?? '');
    setError(null);
  }, [submission.id, submission.version]);

  const nilaiNum = Number(nilaiInput);
  const nilaiValid =
    nilaiInput !== '' &&
    !Number.isNaN(nilaiNum) &&
    nilaiNum >= 0 &&
    nilaiNum <= 100;

  const showPenaltyPreview =
    submission.is_late && tugasPenaltyPersen > 0 && nilaiValid;
  const previewFinal = showPenaltyPreview
    ? previewNilaiSetelahPenalty(nilaiNum, true, tugasPenaltyPersen)
    : null;

  const gradeMu = useMutation({
    mutationFn: () =>
      gradeSubmission(submission.id, {
        nilai_asli: nilaiNum,
        feedback: feedback.trim(),
        version: submission.version,
      }),
    onSuccess: () => {
      qc.invalidateQueries({
        queryKey: ['guru', 'tugas', tugasID, 'submissions'],
      });
      qc.invalidateQueries({ queryKey: ['submission', submission.id] });
      toast({
        title: 'Nilai tersimpan',
        description: `${nilaiNum.toFixed(2)} kepada siswa.`,
      });
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      const apiErr = err instanceof ApiError ? err : null;
      const friendly = apiErr
        ? friendlySubmissionError(apiErr, 'grade')
        : err instanceof Error
          ? err.message
          : 'Gagal kasih nilai.';
      setError(friendly);
      toast({
        title: 'Gagal kasih nilai',
        description: friendly,
        variant: 'destructive',
      });
    },
  });

  const onSubmit = () => {
    if (!nilaiValid) {
      setError('Nilai harus 0..100 (decimal 2).');
      return;
    }
    if (feedback.length > MAX_SUBMISSION_FEEDBACK_BYTES) {
      setError(
        `Feedback melebihi batas ${MAX_SUBMISSION_FEEDBACK_BYTES / 1024} KB.`,
      );
      return;
    }
    setError(null);
    gradeMu.mutate();
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {isGraded ? 'Nilai tersimpan' : 'Beri nilai submission'}
          </DialogTitle>
          <DialogDescription className="flex flex-wrap items-center gap-2 text-xs">
            {tugasJudul && <span className="font-medium">{tugasJudul}</span>}
            <code>{submission.siswa_id.slice(0, 8)}…</code>
            <span>v{submission.version}</span>
            <span>
              Terkirim: {formatSubmissionTimestamp(submission.submitted_at)}
            </span>
            {submission.is_late && (
              <span className="rounded-full bg-rose-100 px-2 py-0.5 text-[10px] font-medium text-rose-700 dark:bg-rose-900/40 dark:text-rose-300">
                LATE
              </span>
            )}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {submission.catatan && (
            <div className="space-y-1">
              <p className="text-xs font-medium text-muted-foreground">
                Catatan siswa
              </p>
              <div className="max-h-40 overflow-auto rounded-md border bg-muted/30 p-2 text-sm whitespace-pre-wrap">
                {submission.catatan}
              </div>
            </div>
          )}

          {submission.attachments && submission.attachments.length > 0 && (
            <div className="space-y-1">
              <p className="text-xs font-medium text-muted-foreground">
                Lampiran siswa
              </p>
              <SubmissionAttachmentLinks
                submissionID={submission.id}
                attachments={submission.attachments}
              />
            </div>
          )}

          <div className="grid gap-2 sm:grid-cols-[1fr_auto_1fr] sm:items-end">
            <div className="space-y-1.5">
              <Label htmlFor="nilai-asli">Nilai (0–100)</Label>
              <Input
                id="nilai-asli"
                type="number"
                step="0.01"
                min={0}
                max={100}
                value={nilaiInput}
                onChange={(e) => setNilaiInput(e.target.value)}
                disabled={isGraded || gradeMu.isPending}
                placeholder="mis. 87.5"
              />
            </div>
            {showPenaltyPreview ? (
              <div className="hidden sm:block text-center text-xs text-muted-foreground">
                ×<br />
                ({100 - tugasPenaltyPersen}%)<br />
                =
              </div>
            ) : (
              <div className="hidden sm:block" />
            )}
            <div className="space-y-1.5">
              <Label className="text-muted-foreground">Nilai akhir</Label>
              <div className="rounded-md border bg-muted/40 px-3 py-2 text-sm">
                {nilaiValid
                  ? showPenaltyPreview
                    ? `${previewFinal!.toFixed(2)} (penalty ${tugasPenaltyPersen}%)`
                    : nilaiNum.toFixed(2)
                  : '—'}
              </div>
            </div>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="feedback">Feedback (opsional)</Label>
            <textarea
              id="feedback"
              className="flex min-h-[100px] w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              placeholder="Tulis catatan / koreksi untuk siswa…"
              value={feedback}
              onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
                setFeedback(e.target.value)
              }
              disabled={isGraded || gradeMu.isPending}
              rows={4}
            />
            <p className="text-xs text-muted-foreground">
              {(feedback.length / 1024).toFixed(1)} KB /{' '}
              {MAX_SUBMISSION_FEEDBACK_BYTES / 1024} KB
            </p>
          </div>

          {isGraded && submission.graded_at && (
            <p className="text-xs text-muted-foreground">
              Dinilai pada {formatSubmissionTimestamp(submission.graded_at)} —
              edit grade defer ke v0.10. Kalau salah, hapus submission ini
              dan minta siswa resubmit.
            </p>
          )}

          {error && (
            <p className="text-sm text-destructive" role="alert">
              {error}
            </p>
          )}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={gradeMu.isPending}
          >
            {isGraded ? 'Tutup' : 'Batal'}
          </Button>
          {!isGraded && (
            <Button
              type="button"
              onClick={onSubmit}
              disabled={!nilaiValid || gradeMu.isPending}
            >
              {gradeMu.isPending && (
                <Loader2 className="size-4 animate-spin" />
              )}
              Simpan nilai
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SubmissionAttachmentLinks({
  submissionID,
  attachments,
}: {
  submissionID: string;
  attachments: NonNullable<Submission['attachments']>;
}) {
  const { toast } = useToast();
  const [downloadingID, setDownloadingID] = React.useState<string | null>(null);

  if (attachments.length === 0) return null;

  const onDownload = async (attID: string) => {
    setDownloadingID(attID);
    try {
      const res = await getSubmissionAttachmentURL(submissionID, attID);
      window.open(res.url, '_blank', 'noopener,noreferrer');
    } catch (err) {
      toast({
        title: 'Gagal generate link',
        description: err instanceof Error ? err.message : 'Coba lagi.',
        variant: 'destructive',
      });
    } finally {
      setDownloadingID(null);
    }
  };

  return (
    <ul className="space-y-1">
      {attachments.map((att) => (
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
