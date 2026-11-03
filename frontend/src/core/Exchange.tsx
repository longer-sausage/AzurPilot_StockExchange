"use client";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Toasty, useKumoToastManager } from "@cloudflare/kumo";
import { request, RequestError, clearPrivateCache } from "./http";
import type { Account, Ledger, Market, Seasons, Settings } from "./contracts";
interface ExchangeState {
  market?: Market;
  settings?: Settings;
  seasons?: Seasons;
  account?: Account;
  ledger?: Ledger;
  connected: boolean;
  error: string;
  now: number;
  loginOpen: boolean;
  setLoginOpen: (open: boolean) => void;
  bindingOpen: boolean;
  setBindingOpen: (open: boolean) => void;
  setAccount: (account: Account | undefined) => void;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
  theme: "light" | "dark";
  toggleTheme: () => void;
}
const Context = createContext<ExchangeState | null>(null);
function State({ children }: { children: ReactNode }) {
  const [market, setMarket] = useState<Market>(),
    [settings, setSettings] = useState<Settings>(),
    [seasons, setSeasons] = useState<Seasons>(),
    [account, setAccountState] = useState<Account>(),
    [ledger, setLedger] = useState<Ledger>(),
    [connected, setConnected] = useState(false),
    [error, setError] = useState(""),
    [now, setNow] = useState(Date.now()),
    [loginOpen, setLoginOpen] = useState(false),
    [bindingOpen, setBindingOpen] = useState(false),
    [theme, setTheme] = useState<"light" | "dark">("light");
  const accountRef = useRef<Account | undefined>(undefined),
    active = useRef(true),
    inflight = useRef<Promise<void> | undefined>(undefined),
    queued = useRef(false);
  const setAccount = useCallback((value: Account | undefined) => {
    accountRef.current = value;
    setAccountState(value);
    if (!value) setLedger(undefined);
  }, []);
  const refresh = useCallback(async () => {
    queued.current = true;
    if (inflight.current) return inflight.current;
    const run = async () => {
      do {
        queued.current = false;
        try {
          const [m, r, s] = await Promise.all([
            request<Market>("/public/market"),
            request<Settings>("/public/rules"),
            request<Seasons>("/public/seasons"),
          ]);
          if (!active.current) return;
          setMarket(m);
          setSettings(r);
          setSeasons(s);
          setError("");
          if (accountRef.current) {
            const me = await request<{ account: Account }>("/auth/me");
            if (!active.current) return;
            setAccount(me.account);
            if (!me.account.forcePasswordChange && me.account.boundStockId) {
              const data = await request<Ledger>("/account");
              if (active.current) {
                setLedger(data);
                setAccount(data.account);
              }
            } else setLedger(undefined);
          }
        } catch (e) {
          if (e instanceof RequestError && e.status === 401)
            setAccount(undefined);
          else if (active.current) setError((e as Error).message);
        }
      } while (queued.current && active.current);
    };
    inflight.current = run();
    try {
      await inflight.current;
    } finally {
      inflight.current = undefined;
    }
  }, [setAccount]);
  useEffect(() => {
    active.current = true;
    void request<{ account: Account }>("/auth/me")
      .then(({ account: a }) => {
        if (active.current) {
          setAccount(a);
          void refresh();
        }
      })
      .catch(() => {});
    void refresh();
    const events = new EventSource("/api/v2/public/events");
    let scheduled: ReturnType<typeof setTimeout> | undefined;
    const schedule = () => {
      if (scheduled) return;
      scheduled = setTimeout(() => {
        scheduled = undefined;
        void refresh();
      }, 150);
    };
    events.onopen = () => {
      setConnected(true);
      schedule();
    };
    events.onerror = () => setConnected(false);
    events.addEventListener("change", schedule);
    const visible = () => {
      if (document.visibilityState === "visible") schedule();
    };
    window.addEventListener("online", schedule);
    document.addEventListener("visibilitychange", visible);
    const clock = setInterval(() => setNow(Date.now()), 1000);
    return () => {
      active.current = false;
      clearTimeout(scheduled);
      clearInterval(clock);
      events.close();
      window.removeEventListener("online", schedule);
      document.removeEventListener("visibilitychange", visible);
    };
  }, [refresh, setAccount]);
  useEffect(() => {
    const lost = (event: Event) => {
      if ((event as CustomEvent).detail === "player") setAccount(undefined);
    };
    window.addEventListener("exchange:unauthorized", lost);
    return () => window.removeEventListener("exchange:unauthorized", lost);
  }, [setAccount]);
  useEffect(() => {
    try {
      const saved = localStorage.getItem("mmex-theme");
      if (saved === "light" || saved === "dark") setTheme(saved);
      else if (matchMedia("(prefers-color-scheme: dark)").matches)
        setTheme("dark");
    } catch {}
  }, []);
  useEffect(() => {
    document.documentElement.dataset.mode = theme;
    document.documentElement.dataset.stockTheme = theme;
    document.documentElement.style.colorScheme = theme;
    try {
      localStorage.setItem("mmex-theme", theme);
    } catch {}
  }, [theme]);
  const logout = async () => {
    await request("/auth/logout", {});
    clearPrivateCache();
    setAccount(undefined);
    setBindingOpen(false);
  };
  return (
    <Context.Provider
      value={{
        market,
        settings,
        seasons,
        account,
        ledger,
        connected,
        error,
        now,
        loginOpen,
        setLoginOpen,
        bindingOpen,
        setBindingOpen,
        setAccount,
        refresh,
        logout,
        theme,
        toggleTheme: () => setTheme((v) => (v === "light" ? "dark" : "light")),
      }}
    >
      {children}
    </Context.Provider>
  );
}
export function Exchange({ children }: { children: ReactNode }) {
  return (
    <Toasty>
      <State>{children}</State>
    </Toasty>
  );
}
export function useExchange() {
  const state = useContext(Context);
  if (!state) throw Error("缺少交易所上下文");
  return state;
}
export function useNotice() {
  const manager = useKumoToastManager();
  return (title: string, error = false) =>
    manager.add({ title, variant: error ? "error" : "success" });
}
