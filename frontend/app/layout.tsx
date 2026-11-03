import type { Metadata } from "next";
import "./globals.css";
export const metadata: Metadata = {
  title: "茗喵证券交易所",
  description: "行动力定价的模拟证券市场",
  robots: { index: false, follow: false },
};
export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <body>{children}</body>
    </html>
  );
}
