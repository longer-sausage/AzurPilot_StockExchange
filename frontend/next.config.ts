import type { NextConfig } from "next";
const config = (phase: string): NextConfig => ({
  output: "export",
  poweredByHeader: false,
  images: { unoptimized: true },
  experimental: {
    optimizePackageImports: [
      "@cloudflare/kumo",
      "@phosphor-icons/react",
      "echarts",
    ],
  },
  ...(phase === "phase-development-server"
    ? {
        async rewrites() {
          return [
            {
              source: "/api/:path*",
              destination: `http://127.0.0.1:${process.env.EXCHANGE_API_PORT ?? 8888}/api/:path*`,
            },
            {
              source: "/healthz",
              destination: `http://127.0.0.1:${process.env.EXCHANGE_API_PORT ?? 8888}/healthz`,
            },
          ];
        },
      }
    : {}),
});
export default config;
