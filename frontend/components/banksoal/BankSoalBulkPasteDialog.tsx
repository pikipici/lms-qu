'use client';

/**
 * BankSoalBulkPasteDialog — bulk-create soal pipe-delimited (Task 6.F.1).
 *
 * Format per baris (8 kolom):
 *   pertanyaan|opsi_a|opsi_b|opsi_c|opsi_d|opsi_e|jawaban|poin
 *
 * Beda dari SoalBab BulkPasteDialog:
 *   - 8 kolom (drop `mode` — BankSoal cross-bab tanpa mode field)
 *   - Top-level body kirim mapel/tingkat/topik default → di-apply ke semua
 *     soal hasil parse. Hemat user input. Bisa edit per-soal lewat PATCH
 *     setelahnya.
 *
 * Escape: `\|` literal pipe. Skip blank + lines starting `#`. Cap 200.
 * Backend validate per-line + return partial success (locked #77 mirror).
 */

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { CheckCircle2, Copy, Loader2, XCircle } from 'lucide-react';

import { ApiError } from '@/lib/api';
import {
  type BankSoalBulkResponse,
  bulkCreateBankSoal,
  friendlyBankSoalError,
} from '@/lib/banksoal-api';
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

const SAMPLE = [
  '# Format: pertanyaan|opsi_a|opsi_b|opsi_c|opsi_d|opsi_e|jawaban|poin',
  '# jawaban: a/b/c/d/e — poin: 1-100 (kosong = 1)',
  '# pakai \\| untuk pipe literal di teks',
  '# baris diawali # akan di-skip',
  '# Tag mapel/tingkat/topik di-apply ke semua soal dari field di atas',
  '',
  'Apa ibukota Indonesia?|Jakarta|Surabaya|Bandung|Medan|Bali|a|10',
  '2 + 2 = ?|3|4|5|6|7|b|5',
].join('\n');

export interface BankSoalBulkPasteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Default mapel/tingkat/topik/tags dari filter aktif di parent. */
  defaultMapel?: string;
  defaultTingkat?: string;
  defaultTopik?: string;
  defaultTags?: string[];
  invalidateKeys: readonly (readonly unknown[])[];
}

function countDataLines(raw: string): number {
  return raw
    .split('\n')
    .filter((l) => l.trim() && !l.trim().startsWith('#')).length;
}

export function BankSoalBulkPasteDialog({
  open,
  onOpenChange,
  defaultMapel,
  defaultTingkat,
  defaultTopik,
  defaultTags,
  invalidateKeys,
}: BankSoalBulkPasteDialogProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [raw, setRaw] = React.useState('');
  const [mapel, setMapel] = React.useState(defaultMapel ?? '');
  const [tingkat, setTingkat] = React.useState(defaultTingkat ?? '');
  const [topik, setTopik] = React.useState(defaultTopik ?? '');
  const [tagsInput, setTagsInput] = React.useState('');
  const [result, setResult] = React.useState<BankSoalBulkResponse | null>(null);

  React.useEffect(() => {
    if (open) {
      setRaw('');
      setResult(null);
      setMapel(defaultMapel ?? '');
      setTingkat(defaultTingkat ?? '');
      setTopik(defaultTopik ?? '');
      setTagsInput((defaultTags ?? []).join(', '));
    }
  }, [open, defaultMapel, defaultTingkat, defaultTopik, defaultTags]);

  const dataLines = React.useMemo(() => countDataLines(raw), [raw]);

  const mutation = useMutation({
    mutationFn: () =>
      bulkCreateBankSoal({
        rows: raw,
        mapel: mapel.trim() || undefined,
        tingkat: tingkat.trim() || undefined,
        topik: topik.trim() || undefined,
        tags: tagsInput
          .split(',')
          .map((t) => t.trim())
          .filter(Boolean),
      }),
    onSuccess: (res) => {
      setResult(res);
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      const errCount = res.errors?.length ?? 0;
      if (errCount === 0) {
        toast({
          title: 'Bulk paste berhasil',
          description: `${res.created} soal masuk Bank Soal.`,
        });
      } else {
        toast({
          title: 'Bulk paste partial-success',
          description: `${res.created} masuk, ${errCount} baris error — lihat detail di bawah.`,
        });
      }
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyBankSoalError(apiErr, 'bulk')
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
          <DialogTitle>Bulk paste Bank Soal</DialogTitle>
          <DialogDescription>
            Tempel beberapa soal sekaligus dalam format pipe-delimited.
            Maksimal 200 baris per batch. Tag mapel/tingkat/topik di bawah
            akan di-apply ke semua soal hasil parse.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
            <div className="space-y-1">
              <Label htmlFor="bulkMapel" className="text-xs">
                Mapel default
              </Label>
              <Input
                id="bulkMapel"
                placeholder="Matematika"
                value={mapel}
                onChange={(e) => setMapel(e.target.value)}
                disabled={mutation.isPending}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="bulkTingkat" className="text-xs">
                Tingkat default
              </Label>
              <Input
                id="bulkTingkat"
                placeholder="X / XI / XII"
                value={tingkat}
                onChange={(e) => setTingkat(e.target.value)}
                disabled={mutation.isPending}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="bulkTopik" className="text-xs">
                Topik default
              </Label>
              <Input
                id="bulkTopik"
                placeholder="Aljabar"
                value={topik}
                onChange={(e) => setTopik(e.target.value)}
                disabled={mutation.isPending}
              />
            </div>
          </div>

          <div className="space-y-1">
            <Label htmlFor="bulkTags" className="text-xs">
              Tags (pisahkan dengan koma)
            </Label>
            <Input
              id="bulkTags"
              placeholder="hots, remedial, grafik"
              value={tagsInput}
              onChange={(e) => setTagsInput(e.target.value)}
              disabled={mutation.isPending}
            />
            <p className="text-[11px] text-muted-foreground">
              Tag bebas dipisahkan koma, akan dinormalisasi otomatis. Di-apply ke semua soal.
            </p>
          </div>

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
              <span
                className={
                  dataLines > 200
                    ? 'font-semibold text-destructive'
                    : 'font-semibold'
                }
              >
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
                  {result.created} soal berhasil dibuat
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
                          <span className="text-muted-foreground">
                            {' '}
                            — {e.raw.slice(0, 80)}
                          </span>
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
              {mutation.isPending && (
                <Loader2 className="size-4 animate-spin" />
              )}
              Proses {dataLines} baris
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
