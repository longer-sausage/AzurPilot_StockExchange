"use client";
import dynamic from "next/dynamic";
// Go 的既有静态路由对 /console 返回根 HTML；浏览器按实际地址选择入口。
const Exchange = dynamic(
  () =>
    location.pathname.replace(/\/$/, "") === "/console"
      ? import("../src/Admin").then((module) => module.Admin)
      : import("../src/main"),
  { ssr: false },
);
export default function Page() {
  return <Exchange />;
}
