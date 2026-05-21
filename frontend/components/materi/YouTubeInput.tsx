'use client';

/**
 * YouTubeInput — text input URL + live parser + embed preview thumbnail.
 *
 * Pakai `parseYouTubeID` dari `@/lib/youtube` (FE-side mirror dari backend
 * locked #65). Saat URL valid → tampilin thumbnail `i.ytimg.com/vi/<id>/...`
 * sebagai konfirmasi visual sebelum user submit.
 *
 * Controlled component — caller hold value (URL atau video_id mentah).
 * Pakai `onParsedChange(id|null)` supaya parent bisa enable/disable submit
 * berdasarkan parse status.
 *
 * Catatan: backend yang validate akhirnya — FE ini cuma UX guard.
 */

import * as React from 'react';
import { Youtube } from 'lucide-react';

import { tryParseYouTubeID, youtubeEmbedURL } from '@/lib/youtube';
import { cn } from '@/lib/utils';
import { Input } from '@/components/ui/input';

interface YouTubeInputProps {
  value: string;
  onChange: (value: string) => void;
  /** Notify parent setiap parse status berubah (`null` = invalid/empty). */
  onParsedChange?: (videoID: string | null) => void;
  disabled?: boolean;
  errorMessage?: string;
  id?: string;
}

export function YouTubeInput({
  value,
  onChange,
  onParsedChange,
  disabled,
  errorMessage,
  id,
}: YouTubeInputProps) {
  const parsed = React.useMemo(() => tryParseYouTubeID(value), [value]);

  React.useEffect(() => {
    onParsedChange?.(parsed);
  }, [parsed, onParsedChange]);

  const showError = !!value.trim() && !parsed && !disabled;

  return (
    <div className="space-y-2">
      <Input
        id={id}
        type="url"
        inputMode="url"
        autoComplete="off"
        placeholder="https://youtu.be/abc12345678 atau https://www.youtube.com/watch?v=..."
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        aria-invalid={showError || !!errorMessage}
      />

      {parsed ? (
        <div className="overflow-hidden rounded-md border bg-muted/30">
          <div className="aspect-video w-full">
            <iframe
              src={youtubeEmbedURL(parsed)}
              title="Pratinjau YouTube"
              className="h-full w-full"
              allow="accelerometer; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
              allowFullScreen
            />
          </div>
          <div className="flex items-center gap-2 border-t bg-background px-3 py-1.5 text-xs text-muted-foreground">
            <Youtube className="size-3.5" />
            <span>
              Video ID terdeteksi: <code className="rounded bg-muted px-1 py-0.5">{parsed}</code>
            </span>
          </div>
        </div>
      ) : (
        <div className="rounded-md border border-dashed p-3 text-xs text-muted-foreground">
          Tempel URL YouTube — pratinjau akan muncul otomatis kalau format
          valid (youtu.be / youtube.com/watch / shorts / embed).
        </div>
      )}

      {errorMessage && (
        <p className={cn('text-xs text-destructive')}>{errorMessage}</p>
      )}
      {!errorMessage && showError && (
        <p className="text-xs text-destructive">
          URL YouTube tidak valid. Pastikan video_id 11 karakter (cth.
          dQw4w9WgXcQ).
        </p>
      )}
    </div>
  );
}
