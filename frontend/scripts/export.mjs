import { cp, mkdir } from "node:fs/promises";
// 保留 Go 既有静态挂载；Go 的 /console HTML fallback 由浏览器入口识别。
await mkdir(new URL("../dist/", import.meta.url), { recursive: true });
await cp(
  new URL("../out/", import.meta.url),
  new URL("../dist/", import.meta.url),
  { recursive: true },
);
