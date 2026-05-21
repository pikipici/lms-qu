'use client';

/**
 * YouTubeEmbed — render materi tipe='youtube' read-only untuk siswa.
 *
 * Pakai youtube-nocookie iframe (privacy-first locked roadmap §3.D.2 +
 * #65). Auto mark-as-read on mount langsung (no debounce — load embed
 * sudah cukup signal user mau nonton).
 *
 * Fallback: kalau `videoID` invalid format, tampilin pesan error
 * (defensive — backend simpan video_id terverifikasi via parseYouTubeID,
 * tapi kalau mismatch terjadi, render error daripada iframe rusak).
 */

import * as React from 'react';

import { youtubeEmbedURL, youtubeWatchURL } from '@/lib/youtube';
import { useMarkMateriRead } from './useMarkMateriRead';

interface YouTubeEmbedProps {
  materiID: string;
  /** 11-char YouTube video_id (tersimpan di materi.konten untuk tipe=youtube). */
  videoID: string;
  /** Title untuk iframe a11y. */
  title?: string;
}

const VIDEO_ID_RE = /^[A-Za-z0-9_-]{11}$/;

export function YouTubeEmbed({
  materiID,
  videoID,
  title = 'Video YouTube materi',
}: YouTubeEmbedProps) {
  const { markRead } = useMarkMateriRead(materiID);

  React.useEffect(() => {
    if (!VIDEO_ID_RE.test(videoID)) return;
    markRead();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [materiID, videoID]);

  if (!VIDEO_ID_RE.test(videoID)) {
    return (
      <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
        Video ID YouTube tidak valid ({videoID || 'kosong'}). Hubungi guru
        kalau materi ini seharusnya bisa diputar.
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <div className="aspect-video w-full overflow-hidden rounded-md border bg-black">
        <iframe
          src={youtubeEmbedURL(videoID)}
          title={title}
          className="h-full w-full"
          loading="lazy"
          allow="accelerometer; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
          allowFullScreen
        />
      </div>
      <p className="text-xs text-muted-foreground">
        Buka di YouTube:{' '}
        <a
          href={youtubeWatchURL(videoID)}
          target="_blank"
          rel="noopener noreferrer"
          className="underline-offset-2 hover:underline"
        >
          {youtubeWatchURL(videoID)}
        </a>
      </p>
    </div>
  );
}
