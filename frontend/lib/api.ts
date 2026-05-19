/**
 * API client — thin fetch wrapper.
 *
 * Locked decisions referenced:
 *   - #4  Static export: API base baked at build time (#60).
 *   - #42 Refresh token rotation: 401 triggers a single refresh attempt then
 *         retries the original request. If refresh fails, redirect to /login.
 *   - #49 Request ID: server returns it; we surface in error messages so
 *         users can quote it to admin.
 *
 * Fase 0 status: scaffolding only. The interceptor for auto-refresh and
 * Zustand auth store integration land in Fase 1.
 */

export const API_BASE = process.env.NEXT_PUBLIC_API_BASE ?? '/api/v1';

export class ApiError extends Error {
  status: number;
  code: string;
  requestId?: string;
  details?: unknown;

  constructor(opts: {
    status: number;
    code: string;
    message: string;
    requestId?: string;
    details?: unknown;
  }) {
    super(opts.message);
    this.name = 'ApiError';
    this.status = opts.status;
    this.code = opts.code;
    this.requestId = opts.requestId;
    this.details = opts.details;
  }
}

interface ApiInit extends Omit<RequestInit, 'body'> {
  body?: unknown;
  /** When true, do NOT add the `Authorization` header. */
  anon?: boolean;
}

function getAccessToken(): string | null {
  if (typeof window === 'undefined') return null;
  // Fase 1 will swap this for the Zustand auth store (lib/auth.ts).
  return window.localStorage.getItem('lms.access');
}

/**
 * `api()` — primary HTTP entrypoint.
 *
 * Usage:
 *   const me = await api<MeResponse>('/auth/me');
 *   await api('/auth/login', { method: 'POST', body: { email, password }, anon: true });
 */
export async function api<T = unknown>(path: string, init: ApiInit = {}): Promise<T> {
  const url = `${API_BASE}${path.startsWith('/') ? path : `/${path}`}`;

  const headers = new Headers(init.headers as HeadersInit | undefined);
  if (init.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  if (!init.anon) {
    const token = getAccessToken();
    if (token) headers.set('Authorization', `Bearer ${token}`);
  }

  const res = await fetch(url, {
    ...init,
    headers,
    body: init.body !== undefined ? JSON.stringify(init.body) : undefined,
  });

  const requestId = res.headers.get('X-Request-ID') ?? undefined;

  if (res.status === 204) {
    return undefined as T;
  }

  const contentType = res.headers.get('Content-Type') ?? '';
  const payload = contentType.includes('application/json')
    ? await res.json().catch(() => null)
    : await res.text().catch(() => null);

  if (!res.ok) {
    const message =
      (payload && typeof payload === 'object' && 'error' in payload
        ? String((payload as { error: unknown }).error)
        : null) ?? `Request failed (${res.status})`;
    const code =
      (payload && typeof payload === 'object' && 'code' in payload
        ? String((payload as { code: unknown }).code)
        : null) ?? 'unknown';
    throw new ApiError({
      status: res.status,
      code,
      message,
      requestId,
      details: payload,
    });
  }

  return payload as T;
}
