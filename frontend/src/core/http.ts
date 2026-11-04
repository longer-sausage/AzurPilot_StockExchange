export class RequestError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
  ) {
    super(message);
  }
}
const responses = new Map<string, { tag: string; value: unknown }>();
export function clearPrivateCache() {
  responses.delete("/account");
}
export async function request<T>(
  path: string,
  body?: unknown,
  method = body === undefined ? "GET" : "POST",
): Promise<T> {
  const cacheable =
      method === "GET" && (path.startsWith("/public/") || path === "/account"),
    cached = cacheable ? responses.get(path) : undefined;
  const res = await fetch(`/api/v2${path}`, {
    method,
    credentials: "same-origin",
    headers: {
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...(cached ? { "If-None-Match": cached.tag } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(15000),
  });
  if (res.status === 304 && cached) return cached.value as T;
  const data = await res.json();
  if (!res.ok) {
    if (res.status === 401) {
      clearPrivateCache();
      window.dispatchEvent(
        new CustomEvent("exchange:unauthorized", {
          detail: path.startsWith("/admin/") ? "admin" : "player",
        }),
      );
    }
    throw new RequestError(
      data.error?.message ?? "请求失败，请重试",
      res.status,
      data.error?.code ?? "HTTP_ERROR",
    );
  }
  if (path.startsWith("/auth/")) clearPrivateCache();
  const tag = res.headers.get("ETag");
  if (cacheable && tag) {
    responses.set(path, { tag, value: data });
    if (responses.size > 48) responses.delete(responses.keys().next().value!);
  }
  return data as T;
}
