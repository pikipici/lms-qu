'use client';

/**
 * BulkPasteDialog — bulk-create soal pipe-delimited (Task 5.F.1).
 *
 * Format per baris (9 kolom):
 *   pertanyaan|opsi_a|opsi_b|opsi_c|opsi_d|opsi_e|jawaban|poin|mode
 *
 * Escape: `\|` literal pipe. Skip blank + lines starting `#`. Cap 200.
 * Backend validate per-line + return partial success (locked #77).
 */

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { CheckCircle2, Copy, Loader2, XCircle } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type BulkSoalResponse,
  bulkCreateSoal,
  friendlySoalError,
} from '@/lib/soalbab-api';
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
import { Label } from '@/components/ui/label';

const SAMPLE = [
  '# Format: pertanyaan|opsi_a|opsi_b|opsi_c|opsi_d|opsi_e|jawaban|poin|mode',
  '# jawaban: a/b/c/d/e — mode: latihan/ulangan/keduanya — poin: 1-100',
  '# pakai \\| untuk pipe literal di teks',
  '# baris diawali # akan di-skip',
  '',
  'Apa ibukota Indonesia?|Jakarta|Surabaya|Bandung|Medan|Bali|a|10|keduanya',
  '2 + 2 = ?|3|4|5|6|7|b|5|latihan',
].join('\n');

export interface BulkPasteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  babID: string;
  invalidateKeys: readonly (readonly unknown[])[];
}

function countDataLines(raw: string): number {
  return raw
    .split('\n')
    .filter((l) => l.trim() && !l.trim().startsWith('#')).length;
}

export function BulkPasteDialog({
  open,
  onOpenChange,
  babID,
  invalidateKeys,
}: BulkPasteDialogProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [raw, setRaw] = React.useState('');
  const [result, setResult] = React.useState<BulkSoalResponse | null>(null);

  React.useEffect(() => {
    if (open) {
      setRaw('');
      setResult(null);
    }
  }, [open]);

  const dataLines = React.useMemo(() => countDataLines(raw), [raw]);

  const mutation = useMutation({
    mutationFn: () => bulkCreateSoal(babID, raw),
    onSuccess: (res) => {
      setResult(res);
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      const errCount = res.errors?.length ?? 0;
      if (errCount === 0) {
        toast({
          title: 'Bulk paste berhasil',
          description: `${res.inserted} soal masuk bank.`,
        });
      } else {
        toast({
          title: 'Bulk paste partial-success',
          description: `${res.inserted} masuk, ${errCount} baris error — lihat detail di bawah.`,
        });
      }
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlySoalError(apiErr, 'bulk')
        : 'Gagal memproses bulk paste.';
      toast({
        title: 'Bulk paste ditolak',
        description: apiErr?.requestId
          ? `${message} (req: ${apiErr.requestId})`
          : message,
        variant: 'destructive',
      });
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Bulk paste soal</DialogTitle>
          <DialogDescription>
            Tempel beberapa soal sekaligus dalam format pipe-delimited.
            Maksimal 200 baris per batch. Hard error (format/format
            jawaban/mode/poin) akan abort batch sebelum insert; soft error
            per-baris akan di-skip dan masuk laporan.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="rounded-md border bg-muted/40 p-3">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium">Contoh format</span>
              <Button
                size="sm"
                variant="ghost"
                type="button"
                onClick={() => {
                  setRaw(SAMPLE);
                }}
              >
                <Copy className="size-3.5" />
                Pakai contoh
              </Button>
            </div>
            <pre className="mt-1 overflow-x-auto whitespace-pre-wrap text-xs font-mono text-muted-foreground">
              {SAMPLE}
            </pre>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="bulkRaw">Konten</Label>
            <textarea
              id="bulkRaw"
              className="flex min-h-[200px] w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              placeholder="Tempel baris pipe-delimited…"
              value={raw}
              onChange={(e) => setRaw(e.target.value)}
              disabled={mutation.isPending}
            />
            <p className="text-xs text-muted-foreground">
              Baris data terdeteksi:{' '}
              <span className={dataLines > 200 ? 'font-semibold text-destructive' : 'font-semibold'}>
                {dataLines}
              </span>{' '}
              / 200
            </p>
          </div>

          {result && (
            <div className="rounded-md border p-3 space-y-2">
              <div className="flex items-center gap-2">
                <CheckCircle2 className="size-4 text-emerald-600" />
                <span className="text-sm font-medium">
                  {result.inserted} soal berhasil dibuat
                </span>
              </div>
              {result.errors && result.errors.length > 0 && (
                <div>
                  <div className="flex items-center gap-2 mt-2">
                    <XCircle className="size-4 text-destructive" />
                    <span className="text-sm font-medium text-destructive">
                      {result.errors.length} baris error
                    </span>
                  </div>
                  <ul className="mt-2 space-y-1 max-h-40 overflow-y-auto text-xs">
                    {result.errors.map((e, idx) => (
                      <li
                        key={`${e.line}-${idx}`}
                        className="rounded bg-destructive/10 px-2 py-1 font-mono"
                      >
                        <span className="font-semibold">Line {e.line}:</span>{' '}
                        {e.reason}
                        {e.raw && (
                          <span className="text-muted-foreground"> — {e.raw.slice(0, 80)}</span>
                        )}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          )}
        </div>

        <DialogFooter className="gap-2">
          <Button
            type="button"
            variant="ghost"
            onClick={() => onOpenChange(false)}
            disabled={mutation.isPending}
          >
            {result ? 'Selesai' : 'Tutup'}
          </Button>
          {!result && (
            <Button
              type="button"
              onClick={() => mutation.mutate()}
              disabled={
                mutation.isPending ||
                dataLines === 0 ||
                dataLines > 200 ||
                !raw.trim()
              }
            >
              {mutation.isPending && <Loader2 className="size-4 animate-spin" />}
              Proses {dataLines} baris
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
