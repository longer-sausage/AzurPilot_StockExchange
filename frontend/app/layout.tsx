import type { Metadata } from "next";
import { Providers } from "./providers";
import "./globals.css";
export const metadata: Metadata = {
  title: "茗喵证券交易所",
  description: "从行动力，看见价值。AzurPilot 模拟证券游戏。",
  robots: { index: false, follow: false },
};
export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
