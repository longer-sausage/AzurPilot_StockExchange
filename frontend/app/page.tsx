"use client";
import dynamic from "next/dynamic";
const Application = dynamic(() => import("../src/Application"), { ssr: false });
export default function Page() {
  return <Application />;
}
