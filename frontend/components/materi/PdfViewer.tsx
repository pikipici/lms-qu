'use client';

/**
 * PdfViewer — render materi tipe='pdf' read-only untuk siswa.
 *
 * Visual: neo-brutalism + pastel pop. Header chip dengan filename +
 * tombol "Buka di tab baru". Iframe wrapped dalam siswa-border.
 *
 * Flow: presigned R2 GET URL via TanStack Query (staleTime 10min) →
 * iframe browser-native → debounced mark-as-read 2s setelah mount.
 */

import * as React from 'react';
import { useQuery } from '@tanstack/react-query';
import { ExternalLink, FileText, Loader2 } from 'lucide-react';

import { ApiError } from '@/lib/api';
import { friendlyMateriError, getMateriFileURL } from '@/lib/materi-api';
import { SiswaButton } from '@/components/siswa-ui';
import { useMarkMateriRead } from './useMarkMateriRead';

interface PdfViewerProps {
  materiID: string;
  /** Filename asli untuk a11y + tombol download. */
  originalFilename?: string | null;
}

const READ_DEBOUNCE_MS = 2000;

export function PdfViewer({ materiID, originalFilename }: PdfViewerProps) {
  const { markRead } = useMarkMateriRead(materiID);

  const urlQuery = useQuery({
    queryKey: ['siswa', 'materi', 'file-url', materiID],
    queryFn: () => getMateriFileURL(materiID),
    staleTime: 10 * 60 * 1000,
    retry: false,
  });

  React.useEffect(() => {
    const t = setTimeout(() => markRead(), READ_DEBOUNCE_MS);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [materiID]);

  if (urlQuery.isPending) {
    return (
      <div className="flex h-64 items-center justify-center rounded-siswa border-2 border-siswa-border-soft bg-siswa-surface/60">
        <div className="flex items-center gap-2 text-sm text-siswa-text-muted">
          <Loader2 className="size-4 animate-spin" />
          Menyiapkan PDF…
        </div>
      </div>
    );
  }

  if (urlQuery.isError) {
    const apiErr = urlQuery.error instanceof ApiError ? urlQuery.error : null;
    const message = apiErr
      ? friendlyMateriError(apiErr, 'file-url')
      : 'Gagal memuat URL PDF.';
    return (
      <div className="space-y-2 rounded-siswa border-2 border-siswa-danger bg-siswa-surface p-4 text-sm">
        <p className="font-semibold">{message}</p>
        <SiswaButton
          type="button"
          tone="surface"
          size="sm"
          onClick={() => urlQuery.refetch()}
        >
          Coba lagi
        </SiswaButton>
      </div>
    );
  }

  const { url } = urlQuery.data;
  const safeName =
    originalFilename && originalFilename.trim() !== ''
      ? originalFilename
      : 'materi.pdf';

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-siswa border-2 border-siswa-border-soft bg-siswa-cream/40 px-3 py-2 text-xs text-siswa-text-muted">
        <div className="flex min-w-0 items-center gap-1.5">
          <FileText className="size-3.5 shrink-0" strokeWidth={2.5} />
          <span title={safeName} className="truncate font-semibold text-siswa-text">
            {safeName}
          </span>
        </div>
        <SiswaButton asChild tone="ghost" size="sm">
          <a href={url} target="_blank" rel="noopener noreferrer">
            <ExternalLink className="size-3.5" strokeWidth={2.5} />
            Buka di tab baru
          </a>
        </SiswaButton>
      </div>
      <div className="overflow-hidden rounded-siswa siswa-border bg-siswa-surface siswa-shadow-sm">
        <iframe
          src={url}
          title={`PDF materi: ${safeName}`}
          className="h-[70vh] min-h-[480px] w-full"
        />
      </div>
    </div>
  );
}
