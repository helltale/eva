import React, { createContext, useCallback, useContext, useMemo, useState } from "react";
import * as api from "./api/client";

type AuthState = {
  access: string | null;
  refresh: string | null;
  email: string | null;
};

const Ctx = createContext<{
  state: AuthState;
  login: (e: string, p: string) => Promise<void>;
  logout: () => void;
} | null>(null);

const storageKey = "eva_auth";

function load(): AuthState {
  try {
    const raw = localStorage.getItem(storageKey);
    if (!raw) return { access: null, refresh: null, email: null };
    return JSON.parse(raw) as AuthState;
  } catch {
    return { access: null, refresh: null, email: null };
  }
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<AuthState>(() => load());

  const persist = useCallback((s: AuthState) => {
    setState(s);
    localStorage.setItem(storageKey, JSON.stringify(s));
  }, []);

  const login = useCallback(
    async (email: string, password: string) => {
      const r = await api.login({ email_or_username: email, password });
      persist({
        access: r.tokens.accessToken,
        refresh: r.tokens.refreshToken,
        email: r.user.emailOrUsername,
      });
    },
    [persist]
  );

  const logout = useCallback(() => {
    persist({ access: null, refresh: null, email: null });
  }, [persist]);

  const v = useMemo(
    () => ({
      state,
      login,
      logout,
    }),
    [state, login, logout]
  );

  return <Ctx.Provider value={v}>{children}</Ctx.Provider>;
}

export function useAuth() {
  const x = useContext(Ctx);
  if (!x) throw new Error("useAuth");
  return x;
}
