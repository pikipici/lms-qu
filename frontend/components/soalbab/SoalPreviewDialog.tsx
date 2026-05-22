/**
 * SoalPreviewDialog — guru preview soal pov siswa, dengan jawaban benar
 * di-highlight emerald + label "(jawaban benar)".
 *
 * Tujuan: verifikasi soal sebelum dipublish ke ulangan/latihan tanpa
 * harus impersonate siswa.
 *
 * Renders:
 *   - Pertanyaan teks + gambar pertanyaan (kalau ada)
 *   - 5 opsi A-E inline. Opsi yang cocok dengan `soal.jawaban` dapat ring
 *     emerald + badge "Jawaban benar". Image slot opsi otomatis di-fetch
 *     presigned URL.
 *
 * Catatan: presigned URL backend short-lived (~5 min default); kalau
 * dialog dibuka lama image bisa expire. Solusi simple — re-open dialog
 * untuk re-fetch.
 */

'use client';

import * as React from 'react';
import { useQuery } from '@tanstack/react-query';
import { CheckCircle2, ImageOff, Loader2 } from 'lucide-react';

import {
  getSoalImageURL,
  type SoalBab,
  type SoalImageSlot,
  type SoalJawaban,
} from '@/lib/soalbab-api';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

export interface SoalPreviewDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  soal: SoalBab | null;
  /** Index urutan untuk header. */
  index?: number;
}

const OPSI_LIST: { key: SoalJawaban; slot: SoalImageSlot; label: string }[] = [
  { key: 'a', slot: 'a', label: 'A' },
  { key: 'b', slot: 'b', label: 'B' },
  { key: 'c', slot: 'c', label: 'C' },
  { key: 'd', slot: 'd', label: 'D' },
  { key: 'e', slot: 'e', label: 'E' },
];

function objectKeyForSlot(s: SoalBab, slot: SoalImageSlot): string | undefined {
  switch (slot) {
    case 'pertanyaan':
      return s.pertanyaan_object_key;
    case 'a':
      return s.opsi_a_object_key;
    case 'b':
      return s.opsi_b_object_key;
    case 'c':
      return s.opsi_c_object_key;
    case 'd':
      return s.opsi_d_object_key;
    case 'e':
      return s.opsi_e_object_key;
  }
}

function modeBadgeClass(m: SoalBab['mode']): string {
  switch (m) {
    case 'latihan':
      return 'bg-sky-50 text-sky-700 border-sky-200';
    case 'ulangan':
      return 'bg-amber-50 text-amber-700 border-amber-200';
    case 'keduanya':
      return 'bg-emerald-50 text-emerald-700 border-emerald-200';
  }
}

function modeLabel(m: SoalBab['mode']): string {
  switch (m) {
    case 'latihan':
      return 'Latihan';
    case 'ulangan':
      return 'Ulangan';
    case 'keduanya':
      return 'Latihan + Ulangan';
  }
}

function opsiText(s: SoalBab, k: SoalJawaban): string {
  switch (k) {
    case 'a':
      return s.opsi_a;
    case 'b':
      return s.opsi_b;
    case 'c':
      return s.opsi_c;
    case 'd':
      return s.opsi_d;
    case 'e':
      return s.opsi_e;
  }
}

export function SoalPreviewDialog({
  open,
  onOpenChange,
  soal,
  index,
}: SoalPreviewDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            Preview soal{typeof index === 'number' ? ` #${index + 1}` : ''}
          </DialogTitle>
          <DialogDescription>
            Tampilan soal dari sisi siswa. Jawaban benar ditandai emerald.
          </DialogDescription>
        </DialogHeader>

        {soal ? (
          <div className="space-y-4">
            <div className="flex flex-wrap items-center gap-2 text-xs">
              <span
                className={cn(
                  'inline-flex rounded-full border px-2 py-0.5',
                  modeBadgeClass(soal.mode),
                )}
              >
                {modeLabel(soal.mode)}
              </span>
              <span className="rounded-full border bg-muted px-2 py-0.5 text-muted-foreground">
                Poin: <strong className="text-foreground">{soal.poin}</strong>
              </span>
            </div>

            <div className="space-y-2">
              <p className="text-sm font-medium uppercase tracking-wide text-muted-foreground">
                Pertanyaan
              </p>
              <div className="rounded-md border bg-card p-3">
                {soal.pertanyaan ? (
                  <p className="whitespace-pre-wrap text-sm">{soal.pertanyaan}</p>
                ) : (
                  <p className="text-sm italic text-muted-foreground">(tanpa teks)</p>
                )}
                <SlotImage soal={soal} slot="pertanyaan" />
              </div>
            </div>

            <div className="space-y-2">
              <p className="text-sm font-medium uppercase tracking-wide text-muted-foreground">
                Opsi
              </p>
              <ul className="space-y-2">
                {OPSI_LIST.map(({ key, slot, label }) => {
                  const isCorrect = soal.jawaban === key;
                  const text = opsiText(soal, key);
                  return (
                    <li
                      key={key}
                      className={cn(
                        'flex gap-3 rounded-md border p-3',
                        isCorrect
                          ? 'border-emerald-300 bg-emerald-50 ring-1 ring-emerald-200'
                          : 'bg-card',
                      )}
                    >
                      <span
                        className={cn(
                          'flex size-7 shrink-0 items-center justify-center rounded-full border text-sm font-semibold',
                          isCorrect
                            ? 'border-emerald-400 bg-emerald-500 text-white'
                            : 'border-input bg-muted text-foreground',
                        )}
                      >
                        {label}
                      </span>
                      <div className="min-w-0 flex-1 space-y-1.5">
                        {text ? (
                          <p className="whitespace-pre-wrap text-sm">{text}</p>
                        ) : (
                          <p className="text-xs italic text-muted-foreground">(tanpa teks)</p>
                        )}
                        <SlotImage soal={soal} slot={slot} />
                        {isCorrect ? (
                          <p className="inline-flex items-center gap-1 text-xs font-medium text-emerald-700">
                            <CheckCircle2 className="size-3.5" />
                            Jawaban benar
                          </p>
                        ) : null}
                      </div>
                    </li>
                  );
                })}
              </ul>
            </div>
          </div>
        ) : (
          <div className="py-6 text-center text-sm text-muted-foreground">
            Tidak ada soal yang dipilih.
          </div>
        )}

        <DialogFooter>
          <Button type="button" onClick={() => onOpenChange(false)}>
            Tutup
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

interface SlotImageProps {
  soal: SoalBab;
  slot: SoalImageSlot;
}

function SlotImage({ soal, slot }: SlotImageProps) {
  const key = objectKeyForSlot(soal, slot);
  const enabled = Boolean(key);

  const presign = useQuery({
    queryKey: ['soal', 'preview', 'image-url', soal.id, slot, soal.version],
    queryFn: () => getSoalImageURL(soal.id, slot),
    enabled,
    staleTime: 4 * 60_000,
  });

  if (!enabled) return null;

  if (presign.isPending) {
    return (
      <div className="flex h-32 items-center justify-center rounded-md border bg-muted/40 text-xs text-muted-foreground">
        <Loader2 className="size-4 animate-spin" />
      </div>
    );
  }

  if (presign.isError || !presign.data?.url) {
    return (
      <div className="flex h-20 items-center justify-center gap-2 rounded-md border border-dashed text-xs text-muted-foreground">
        <ImageOff className="size-4" />
        Gambar gagal dimuat
      </div>
    );
  }

  // Static export → tidak pakai next/image; <img> aman dari rules.
  // eslint-disable-next-line @next/next/no-img-element
  return (
    <img
      src={presign.data.url}
      alt={`Gambar slot ${slot}`}
      className="max-h-64 rounded-md border bg-card object-contain"
    />
  );
}
