/**
 * YouTube URL parser — FE-side mirror dari backend `parseYouTubeID`
 * (locked decision #65). Dipakai untuk live-validate input URL di
 * `MateriCreateDialog` sebelum kirim ke backend.
 *
 * Wajib mirror persis dengan `backend/internal/materi/youtube.go` —
 * 4 format yang diterima:
 *   - youtube.com/watch?v=<id>      (with optional extra params)
 *   - youtu.be/<id>                 (short share link)
 *   - youtube.com/shorts/<id>       (Shorts)
 *   - youtube.com/embed/<id>        (embed link)
 *
 * id MUST be exactly 11 chars dari [A-Za-z0-9_-]. Schemes: http/https +
 * optional www atau m subdomain. Scheme-less input (`youtu.be/abc...`)
 * di-prepend `https://`.
 *
 * Kalau backend nolak input yg passed FE, itu schema mismatch — file
 * ini harus tetap konsisten sama backend setiap update.
 */

const VIDEO_ID_RE = /^[A-Za-z0-9_-]{11}$/;

/**
 * Parse a YouTube URL ke 11-char video_id atau lempar Error untuk URL
 * yang invalid.
 */
export function parseYouTubeID(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) {
    throw new Error('URL kosong');
  }

  // Allow scheme-less input dengan prepend https://.
  const withScheme = trimmed.includes('://') ? trimmed : `https://${trimmed}`;

  let u: URL;
  try {
    u = new URL(withScheme);
  } catch {
    throw new Error('URL tidak valid');
  }

  if (u.protocol !== 'http:' && u.protocol !== 'https:') {
    throw new Error('URL harus pakai http:// atau https://');
  }

  let host = u.host.toLowerCase();
  if (host.startsWith('www.')) host = host.slice(4);
  if (host.startsWith('m.')) host = host.slice(2);

  let id = '';
  switch (host) {
    case 'youtu.be': {
      // youtu.be/<id>[?...]
      let path = u.pathname.replace(/^\/+/, '');
      const slash = path.indexOf('/');
      if (slash >= 0) path = path.slice(0, slash);
      id = path;
      break;
    }
    case 'youtube.com':
    case 'youtube-nocookie.com': {
      const path = u.pathname;
      if (path === '/watch') {
        id = u.searchParams.get('v') ?? '';
      } else if (path.startsWith('/embed/')) {
        id = path.slice('/embed/'.length);
        const slash = id.indexOf('/');
        if (slash >= 0) id = id.slice(0, slash);
      } else if (path.startsWith('/shorts/')) {
        id = path.slice('/shorts/'.length);
        const slash = id.indexOf('/');
        if (slash >= 0) id = id.slice(0, slash);
      } else if (path.startsWith('/v/')) {
        id = path.slice('/v/'.length);
        const slash = id.indexOf('/');
        if (slash >= 0) id = id.slice(0, slash);
      } else {
        throw new Error('Format URL YouTube tidak dikenal');
      }
      break;
    }
    default:
      throw new Error('Host bukan youtube.com / youtu.be');
  }

  if (!VIDEO_ID_RE.test(id)) {
    throw new Error('Video ID YouTube tidak valid (harus 11 karakter)');
  }
  return id;
}

/**
 * Try-parse helper: return id atau null. Untuk live-feedback di input
 * tanpa harus catch error per render.
 */
export function tryParseYouTubeID(raw: string): string | null {
  try {
    return parseYouTubeID(raw);
  } catch {
    return null;
  }
}

/**
 * Build YouTube embed URL pakai youtube-nocookie domain (privacy-first,
 * locked roadmap §3.D.2).
 */
export function youtubeEmbedURL(videoID: string): string {
  return `https://www.youtube-nocookie.com/embed/${videoID}`;
}

/**
 * Build a YouTube watch URL untuk display ke user (e.g. saat edit dialog
 * ingin tampilin URL aslinya yang re-buildable dari video_id).
 */
export function youtubeWatchURL(videoID: string): string {
  return `https://www.youtube.com/watch?v=${videoID}`;
}
