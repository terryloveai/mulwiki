import type { NextConfig } from "next";

const apiBaseUrl = (
  process.env.MULWIKI_API_URL ||
  process.env.NEXT_PUBLIC_API_URL ||
  "http://localhost:8080"
).replace(/\/$/, "");

const nextConfig: NextConfig = {
  output: "standalone",
  transpilePackages: ["@mulwiki/core", "@mulwiki/ui"],
  async rewrites() {
    return {
      afterFiles: [
        { source: "/api/:path*", destination: `${apiBaseUrl}/api/:path*` },
        { source: "/ws", destination: `${apiBaseUrl}/ws` },
        { source: "/uploads/:path*", destination: `${apiBaseUrl}/uploads/:path*` },
      ],
    };
  },
};

export default nextConfig;
