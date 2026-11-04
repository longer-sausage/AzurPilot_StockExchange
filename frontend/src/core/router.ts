"use client";
import { useEffect, useSyncExternalStore } from "react";
// 所有视图共用 /；history.state 保存视图，支持刷新和浏览器前进、后退。
const listeners = new Set<() => void>();
function notify() {
  listeners.forEach((change) => change());
}
function restore() {
  // 浏览器访问旧片段链接时会先触发 popstate；先导入片段，避免恢复旧视图。
  if (location.hash) {
    importHash();
    return;
  }
  current = history.state?.exchangeView ?? "/";
  notify();
}
function importHash() {
  if (!location.hash) return;
  current = location.hash.slice(1);
  history.replaceState({ ...history.state, exchangeView: current }, "", "/");
  notify();
}
function subscribe(change: () => void) {
  listeners.add(change);
  if (listeners.size === 1) {
    window.addEventListener("popstate", restore);
    window.addEventListener("hashchange", importHash);
  }
  return () => {
    listeners.delete(change);
    if (!listeners.size) {
      window.removeEventListener("popstate", restore);
      window.removeEventListener("hashchange", importHash);
    }
  };
}
let current: string | undefined;
function snapshot() {
  if (current !== undefined) return current;
  const legacy =
    location.hash.slice(1) ||
    (location.pathname === "/console"
      ? "/console"
      : new URLSearchParams(location.search).has("stock")
        ? `/security?${new URLSearchParams(location.search)}`
        : "");
  current = legacy || history.state?.exchangeView || "/";
  return current!;
}
function navigate(next: string) {
  if (next === snapshot()) return;
  current = next;
  history.pushState({ ...history.state, exchangeView: next }, "", "/");
  notify();
  window.scrollTo({ top: 0, behavior: "instant" });
}
export function useRoute() {
  const value = useSyncExternalStore(subscribe, snapshot, () => "/");
  useEffect(() => {
    history.replaceState(
      { ...history.state, exchangeView: snapshot() },
      "",
      "/",
    );
  }, []);
  const [pathname, query = ""] = value.split("?");
  return { path: pathname || "/", query: new URLSearchParams(query), navigate };
}
