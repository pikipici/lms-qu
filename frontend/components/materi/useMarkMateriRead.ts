'use client';

/**
 * useMarkMateriRead — fire-and-forget hook untuk siswa mark-as-read.
 *
 * Endpoint backend idempotent (locked #25, ON CONFLICT DO NOTHING) — tapi
 * untuk hindari spam request saat user scroll-by atau remount kilat,
 * dipakai debounce di sisi PDF viewer (locked roadmap §3.D.2 — debounce
 * 2s untuk PDF).
 *
 * Untuk YouTube + Markdown viewer, mark-read fire on mount langsung tanpa
 * debounce (view dianggap intensional).
 *
 * Hook strategy:
 *   - One mutation per materi instance, di-call lewat `markRead()`.
 *   - Tidak invalidate query apa-apa di FE — read state ditampilin via
 *     bab detail siswa endpoint (Task 3.E.1) yang refetch saat user
 *     navigate balik. Mark-read di sini cukup fire-and-forget.
 *   - Error di-toast hanya kalau `notify=true`. Default silent — read
 *     bukan tindakan kritis, kalau gagal user gak perlu tahu.
 */

import * as React from 'react';
import { useMutation } from '@tanstack/react-query';

import { ApiError } from '@/lib/api';
import { friendlyMateriError, markMateriRead } from '@/lib/materi-api';
import { useToast } from '@/hooks/use-toast';

interface UseMarkMateriReadOptions {
  /** Tampilin toast kalau request gagal. Default false (silent). */
  notifyOnError?: boolean;
  /** Callback setelah sukses (pass was_new). */
  onSuccess?: (wasNew: boolean) => void;
}

export function useMarkMateriRead(
  materiID: string,
  opts: UseMarkMateriReadOptions = {},
) {
  const { toast } = useToast();
  const { notifyOnError, onSuccess } = opts;

  const mutation = useMutation({
    mutationFn: () => markMateriRead(materiID),
    onSuccess: (resp) => {
      onSuccess?.(resp.was_new);
    },
    onError: (err) => {
      if (!notifyOnError) return;
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyMateriError(apiErr, 'update')
        : 'Gagal menandai materi sebagai dibaca.';
      toast({
        title: 'Gagal menandai dibaca',
        description: message,
        variant: 'destructive',
      });
    },
    // Hindari retry — kalau gagal sekali, bukan masalah besar.
    retry: false,
  });

  // Stable handle untuk caller useEffect.
  const markRead = React.useCallback(() => {
    if (!materiID) return;
    mutation.mutate();
  }, [materiID, mutation]);

  return { markRead, isPending: mutation.isPending, isSuccess: mutation.isSuccess };
}
