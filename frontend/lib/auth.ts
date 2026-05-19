/**
 * Auth store — Zustand.
 *
 * Holds access token + minimal user (role, must_change_password) so layouts
 * can decide redirects. Refresh token rotation logic lives in `lib/api.ts`
 * (Fase 1). Persisted to localStorage so the user stays logged in across
 * tabs (#5.0).
 *
 * Fase 0 status: type contract + empty store. Wiring with API in Fase 1.
 */

import { create } from 'zustand';
import { persist, createJSONStorage } from 'zustand/middleware';

export type Role = 'admin' | 'guru' | 'siswa';

export interface AuthUser {
  id: string;
  name: string;
  email: string;
  role: Role;
  status: 'active' | 'suspended' | 'locked';
  mustChangePassword: boolean;
}

interface AuthState {
  access: string | null;
  refresh: string | null;
  user: AuthUser | null;
  setSession: (s: { access: string; refresh: string; user: AuthUser }) => void;
  clear: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      access: null,
      refresh: null,
      user: null,
      setSession: ({ access, refresh, user }) =>
        set({ access, refresh, user }),
      clear: () => set({ access: null, refresh: null, user: null }),
    }),
    {
      name: 'lms.auth',
      storage: createJSONStorage(() => {
        if (typeof window === 'undefined') {
          // SSG-safe noop — static export evaluates at build time.
          return {
            getItem: () => null,
            setItem: () => undefined,
            removeItem: () => undefined,
          };
        }
        return window.localStorage;
      }),
    },
  ),
);
