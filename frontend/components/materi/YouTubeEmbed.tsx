'use client';

/**
 * YouTubeEmbed — render materi tipe='youtube' read-only untuk siswa.
 *
 * Visual: neo-brutalism + pastel pop. youtube-nocookie iframe wrapped
 * dalam siswa-border + hard shadow. Auto mark-as-read on mount.
 */

import * as React from 'react';

import { youtubeEmbedURL, youtubeWatchURL } from '@/lib/youtube';
import { useMarkMateriRead } from './useMarkMateriRead';

interface YouTubeEmbedProps {
  materiID: string;
  videoID: string;
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
      <div className="rounded-siswa border-2 border-siswa-danger bg-siswa-surface p-4 text-sm font-semibold">
        Video ID YouTube tidak valid ({videoID || 'kosong'}). Hubungi guru
        kalau materi ini seharusnya bisa diputar.
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <div className="aspect-video w-full overflow-hidden rounded-siswa siswa-border bg-black siswa-shadow-sm">
        <iframe
          src={youtubeEmbedURL(videoID)}
          title={title}
          className="h-full w-full"
          loading="lazy"
          allow="accelerometer; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
          allowFullScreen
        />
      </div>
      <p className="text-xs text-siswa-text-muted">
        Buka di YouTube:{' '}
        <a
          href={youtubeWatchURL(videoID)}
          target="_blank"
          rel="noopener noreferrer"
          className="font-semibold underline-offset-2 hover:underline"
        >
          {youtubeWatchURL(videoID)}
        </a>
      </p>
    </div>
  );
}
