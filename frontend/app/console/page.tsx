"use client";
import dynamic from "next/dynamic";
const Console = dynamic(
  () => import("../../src/Admin").then((module) => module.Admin),
  { ssr: false },
);
export default function Page() {
  return <Console />;
}
