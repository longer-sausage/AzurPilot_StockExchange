"use client";
import { ThemeProvider } from "../src/theme";
import { Toasty } from "@cloudflare/kumo";
export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <ThemeProvider>
      <Toasty>{children}</Toasty>
    </ThemeProvider>
  );
}
