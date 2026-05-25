/**
 * API client — thin fetch wrapper with refresh-token interceptor.
 *
 * Locked decisions referenced:
 *   - #4  Static export: API base baked at build time (#60).
 *   - #42 Refresh token rotation: 401 triggers a single refresh attempt then
 *         retries the original request. If refresh fails, redirect to /login.
 *   - #49 Request ID: server returns it; we surface in error messages so
 *         users can quote it to admin.
 *
 * Fase 1.G.2: refresh interceptor lives here. A module-level promise serves
 * as a mutex so parallel 401s share a single refresh round-trip. On refresh
 * failure the store is cleared and the user is bounced to /login.
 */

import { useAuthStore, type AuthUser, type Role } from '@/lib/auth';

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
  /**
   * When true, skip the 401 → refresh → retry interceptor. Used internally
   * by the refresh call itself to avoid recursion. Callers should not set
   * this directly.
   */
  skipRefresh?: boolean;
}

interface RefreshResponseUser {
  id: string;
  name: string;
  email: string;
  role: Role;
  status: 'active' | 'suspended' | 'locked';
  must_change_password: boolean;
}

interface RefreshResponse {
  user: RefreshResponseUser;
  tokens: {
    access_token: string;
    access_expires_at: string;
    refresh_token: string;
    refresh_expires_at: string;
  };
}

function getAccessToken(): string | null {
  // Reads outside of React tree; Zustand exposes getState() for that.
  return useAuthStore.getState().access;
}

function getRefreshToken(): string | null {
  return useAuthStore.getState().refresh;
}

/** Single in-flight refresh promise — module-level mutex. */
let refreshInFlight: Promise<string | null> | null = null;

async function performRefresh(): Promise<string | null> {
  const refresh = getRefreshToken();
  if (!refresh) return null;

  try {
    const data = await api<RefreshResponse>('/auth/refresh', {
      method: 'POST',
      body: { refresh_token: refresh },
      anon: true,
      skipRefresh: true,
    });
    const u = data.user;
    const user: AuthUser = {
      id: u.id,
      name: u.name,
      email: u.email,
      role: u.role,
      status: u.status,
      mustChangePassword: u.must_change_password,
    };
    useAuthStore.getState().setSession({
      access: data.tokens.access_token,
      refresh: data.tokens.refresh_token,
      user,
    });
    return data.tokens.access_token;
  } catch (err) {
    // Refresh failed — token expired/revoked/reused. Wipe session and bounce.
    useAuthStore.getState().clear();
    if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
      window.location.replace('/login');
    }
    return null;
  }
}

function refreshAccessToken(): Promise<string | null> {
  if (!refreshInFlight) {
    refreshInFlight = performRefresh().finally(() => {
      refreshInFlight = null;
    });
  }
  return refreshInFlight;
}

/**
 * `api()` — primary HTTP entrypoint.
 *
 * Usage:
 *   const me = await api<MeResponse>('/auth/me');
 *   await api('/auth/login', { method: 'POST', body: { email, password }, anon: true });
 *
 * On 401 for an authenticated request the wrapper transparently refreshes
 * the access token (single in-flight promise) and retries once. If refresh
 * itself fails, the auth store is cleared and the user is redirected to
 * /login; callers receive the original 401 ApiError.
 */
export async function api<T = unknown>(path: string, init: ApiInit = {}): Promise<T> {
  return apiInner<T>(path, init, false);
}

async function apiInner<T>(path: string, init: ApiInit, retried: boolean): Promise<T> {
  const url = `${API_BASE}${path.startsWith('/') ? path : `/${path}`}`;

  const headers = new Headers(init.headers as HeadersInit | undefined);
  const isFormData = typeof FormData !== 'undefined' && init.body instanceof FormData;
  if (init.body !== undefined && !isFormData && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  if (!init.anon) {
    const token = getAccessToken();
    if (token) headers.set('Authorization', `Bearer ${token}`);
  }

  const res = await fetch(url, {
    ...init,
    headers,
    body: init.body !== undefined ? (isFormData ? (init.body as BodyInit) : JSON.stringify(init.body)) : undefined,
  });

  const requestId = res.headers.get('X-Request-ID') ?? undefined;

  // 401 interceptor — only for authenticated requests, only once.
  if (
    res.status === 401 &&
    !init.anon &&
    !init.skipRefresh &&
    !retried
  ) {
    const newToken = await refreshAccessToken();
    if (newToken) {
      return apiInner<T>(path, init, true);
    }
    // Fall through to error handling below; performRefresh already
    // cleared the store and (on the browser) kicked off the redirect.
  }

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
