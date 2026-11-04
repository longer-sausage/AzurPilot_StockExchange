import { cp, mkdir, rm } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
// 只清理构建生成且被 Git 忽略的 dist，避免遗留旧多页面产物。
const frontend = path.resolve(fileURLToPath(new URL("../", import.meta.url)));
const destination = path.resolve(frontend, "dist");
if (
  path.dirname(destination) !== frontend ||
  path.basename(destination) !== "dist"
)
  throw Error("构建目录越界");
await rm(destination, { recursive: true, force: true });
await mkdir(destination, { recursive: true });
await cp(new URL("../out/", import.meta.url), destination, { recursive: true });
