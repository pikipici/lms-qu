'use client';

/**
 * PdfViewer — render materi tipe='pdf' read-only untuk siswa.
 *
 * Flow:
 *   1. Fetch presigned R2 GET URL via TanStack Query (staleTime 10min,
 *      locked roadmap §3.D.2). URL TTL 15 menit di server (locked #62);
 *      staleTime 10min kasih buffer 5min sebelum expire.
 *   2. Render `<iframe src={url}>` (browser's built-in PDF renderer).
 *   3. Auto mark-as-read setelah debounce 2s (locked roadmap §3.D.2 —
 *      hindari fire saat user scroll-by tab atau swipe).
 *
 * Tradeoff: iframe sengaja simple — no PDF.js. Browser native cukup
 * untuk MVP, dan loading lebih cepet. Bisa upgrade ke PDF.js nanti
 * kalau perlu page navigation API atau text-search.
 */

import * as React from 'react';
import { useQuery } from '@tanstack/react-query';
import { ExternalLink, FileText, Loader2 } from 'lucide-react';

import { ApiError } from '@/lib/api';
import { friendlyMateriError, getMateriFileURL } from '@/lib/materi-api';
import { Button } from '@/components/ui/button';
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
    staleTime: 10 * 60 * 1000, // 10 menit; URL valid 15 menit di server.
    retry: false,
  });

  // Debounced mark-read: fire 2s setelah viewer mount (locked roadmap §3.D.2).
  React.useEffect(() => {
    const t = setTimeout(() => markRead(), READ_DEBOUNCE_MS);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [materiID]);

  if (urlQuery.isPending) {
    return (
      <div className="flex h-64 items-center justify-center rounded-md border bg-muted/20">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
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
      <div className="space-y-2 rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
        <p>{message}</p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => urlQuery.refetch()}
        >
          Coba lagi
        </Button>
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
      <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
        <div className="flex items-center gap-1.5">
          <FileText className="size-3.5" />
          <span title={safeName} className="truncate">
            {safeName}
          </span>
        </div>
        <Button
          asChild
          variant="ghost"
          size="sm"
          className="h-auto p-1 text-xs"
        >
          <a href={url} target="_blank" rel="noopener noreferrer">
            <ExternalLink className="size-3.5" />
            Buka di tab baru
          </a>
        </Button>
      </div>
      <div className="overflow-hidden rounded-md border bg-muted">
        <iframe
          src={url}
          title={`PDF materi: ${safeName}`}
          className="h-[70vh] min-h-[480px] w-full"
        />
      </div>
    </div>
  );
}
