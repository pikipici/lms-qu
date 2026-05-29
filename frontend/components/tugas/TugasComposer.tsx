'use client';

/**
 * TugasComposer — dialog buat bikin tugas baru.
 *
 * Scope auto-derived dari prop `babID`:
 *   - babID === null → tugas kelas-wide (di tab Tugas /guru/kelas/detail)
 *   - babID === <uuid> → tugas bab-scoped (di tab Tugas /guru/kelas/detail/bab)
 *
 * Default status: draft (guru bisa publish nanti via Edit dialog). Attachment
 * upload happens AFTER create (butuh tugas_id), jadi composer cuma bikin
 * skeleton; attachment manager ada di TugasEditDialog.
 *
 * Server cap: judul 200 chars, deskripsi 50KB. FE pre-validate untuk UX cepat.
 *
 * On success: invalidate query keys + toast + close.
 * On `kelas_archived` / `bab_not_in_kelas` → friendly toast.
 */

import * as React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';

import { ApiError } from '@/lib/api';
import {
  type CreateTugasBody,
  type Tugas,
  MAX_TUGAS_DESKRIPSI_BYTES,
  MAX_TUGAS_JUDUL_LENGTH,
  createTugas,
  friendlyTugasError,
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

export interface TugasComposerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kelasID: string;
  /** UUID bab tempat tugas nempel; null = kelas-wide. */
  babID: string | null;
  /** Query keys untuk invalidate setelah sukses. */
  invalidateKeys: readonly (readonly unknown[])[];
  /** Label scope untuk dialog description. */
  scopeLabel: string;
  /**
   * Optional callback dipanggil dengan tugas baru setelah create sukses,
   * sebelum dialog close. Dipakai oleh TugasList untuk auto-open Edit
   * dialog supaya guru langsung bisa upload attachment.
   */
  onCreated?: (tugas: Tugas) => void;
}

export function TugasComposer({
  open,
  onOpenChange,
  kelasID,
  babID,
  invalidateKeys,
  scopeLabel,
  onCreated,
}: TugasComposerProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const [judul, setJudul] = React.useState('');
  const [deskripsi, setDeskripsi] = React.useState('');
  const [deadlineLocal, setDeadlineLocal] = React.useState('');
  const [izinkanLate, setIzinkanLate] = React.useState(false);
  const [penaltyPersen, setPenaltyPersen] = React.useState(0);
  const [wajibAttachment, setWajibAttachment] = React.useState(false);
  const [bobot, setBobot] = React.useState(100);
  const [publishImmediately, setPublishImmediately] = React.useState(false);
  const [judulError, setJudulError] = React.useState<string | null>(null);
  const [penaltyError, setPenaltyError] = React.useState<string | null>(null);

  // Reset state setiap dialog di-open. Hindari leftover dari sesi sebelumnya.
  React.useEffect(() => {
    if (open) {
      setJudul('');
      setDeskripsi('');
      setDeadlineLocal('');
      setIzinkanLate(false);
      setPenaltyPersen(0);
      setWajibAttachment(false);
      setBobot(100);
      setPublishImmediately(false);
      setJudulError(null);
      setPenaltyError(null);
    }
  }, [open]);

  const mutation = useMutation({
    mutationFn: (body: CreateTugasBody) => createTugas(kelasID, body),
    onSuccess: ({ tugas }) => {
      for (const key of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      toast({
        title:
          tugas.status === 'published' ? 'Tugas dipublish' : 'Tugas tersimpan',
        description:
          tugas.status === 'published'
            ? `"${tugas.judul}" terbit ke siswa enrolled.`
            : `"${tugas.judul}" tersimpan sebagai draft. Buka untuk upload lampiran.`,
      });
      onOpenChange(false);
      onCreated?.(tugas);
    },
    onError: (err) => {
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyTugasError(apiErr, 'create')
        : 'Gagal membuat tugas.';
      const requestId = apiErr?.requestId;
      toast({
        title: 'Gagal membuat tugas',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
    },
  });

  const isPending = mutation.isPending;

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

    // Convert datetime-local (no TZ) → ISO with local TZ offset preserved.
    let deadlineISO: string | null | undefined = undefined;
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
      deadlineISO = dt.toISOString();
    }

    mutation.mutate({
      bab_id: babID,
      judul: judul.trim(),
      deskripsi,
      deadline: deadlineISO,
      izinkan_late: izinkanLate,
      penalty_persen: izinkanLate ? penaltyPersen : 0,
      wajib_attachment: wajibAttachment,
      bobot,
      status: publishImmediately ? 'published' : 'draft',
    });
  }

  const submitDisabled = isPending || !judul.trim();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[92svh] w-[calc(100vw-1rem)] flex-col overflow-hidden p-0 sm:max-w-2xl">
        <DialogHeader className="border-b px-4 py-4 sm:px-6">
          <DialogTitle>Tugas baru</DialogTitle>
          <DialogDescription>
            Scope: {scopeLabel}. Default tersimpan sebagai draft — siswa belum
            lihat sampai kamu publish. Lampiran (PDF/DOCX/gambar) bisa diunggah
            setelah tugas dibuat lewat Edit.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={onSubmit} className="flex min-h-0 flex-1 flex-col">
          <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-4 sm:px-6">
          <div className="space-y-1.5">
            <Label htmlFor="tugas-judul">Judul</Label>
            <Input
              id="tugas-judul"
              value={judul}
              onChange={(e) => {
                setJudul(e.target.value);
                if (judulError) setJudulError(null);
              }}
              disabled={isPending}
              placeholder="cth. Tugas Bab 1 — Esai Perubahan Iklim"
              maxLength={MAX_TUGAS_JUDUL_LENGTH}
              autoFocus
              aria-invalid={!!judulError}
            />
            {judulError && (
              <p className="text-xs text-destructive">{judulError}</p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="tugas-deskripsi">Deskripsi (markdown, opsional)</Label>
            <MarkdownEditor
              id="tugas-deskripsi"
              value={deskripsi}
              onChange={setDeskripsi}
              disabled={isPending}
              rows={6}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="tugas-deadline">Deadline (opsional)</Label>
            <Input
              id="tugas-deadline"
              type="datetime-local"
              value={deadlineLocal}
              onChange={(e) => setDeadlineLocal(e.target.value)}
              disabled={isPending}
            />
            <p className="text-xs text-muted-foreground">
              Kosongkan kalau tugas always-open (tanpa batas waktu).
            </p>
          </div>

          <div className="space-y-2 rounded-md border p-3">
            <label className="flex items-start gap-2">
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
              <div className="space-y-1.5 sm:ml-6">
                <Label htmlFor="tugas-penalty">Penalty (% dari nilai)</Label>
                <Input
                  id="tugas-penalty"
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
                  className="w-full sm:max-w-[120px]"
                  aria-invalid={!!penaltyError}
                />
                <p className="text-xs text-muted-foreground">
                  0 = boleh telat tanpa potongan. Misal 20 = nilai dipotong 20%.
                </p>
                {penaltyError && (
                  <p className="text-xs text-destructive">{penaltyError}</p>
                )}
              </div>
            )}
            <p className="text-xs text-muted-foreground">
              Kalau tidak dicentang dan deadline lewat, siswa hard-block tidak
              bisa submit (locked #71).
            </p>
          </div>

          <label className="flex items-start gap-2">
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
            <Label htmlFor="tugas-bobot">Bobot tugas</Label>
            <Input
              id="tugas-bobot"
              type="number"
              min={0}
              value={bobot}
              onChange={(e) => {
                const n = Number(e.target.value);
                setBobot(Number.isFinite(n) ? Math.max(0, n) : 0);
              }}
              disabled={isPending}
              className="w-full sm:max-w-[140px]"
            />
            <p className="text-xs text-muted-foreground">
              Dipakai untuk rata-rata nilai tugas di bab ini. Default 100; 0 = tidak ikut bobot.
            </p>
          </div>

          <label className="flex items-start gap-2">
            <input
              type="checkbox"
              className="size-4 rounded border-input"
              checked={publishImmediately}
              onChange={(e) => setPublishImmediately(e.target.checked)}
              disabled={isPending}
            />
            <span className="text-sm">
              Langsung publish (siswa enrolled langsung lihat)
            </span>
          </label>

          </div>

          <DialogFooter className="gap-2 border-t px-4 py-4 sm:px-6">
            <Button
              type="button"
              variant="outline"
              className="w-full sm:w-auto"
              onClick={() => onOpenChange(false)}
              disabled={isPending}
            >
              Batal
            </Button>
            <Button type="submit" className="w-full sm:w-auto" disabled={submitDisabled}>
              {isPending ? 'Menyimpan…' : 'Simpan'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
