import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  transpilePackages: ["@mulwiki/core", "@mulwiki/ui"],
  async rewrites() {
    return {
      afterFiles: [
        { source: "/api/:path*", destination: "http://localhost:8080/api/:path*" },
        { source: "/ws", destination: "http://localhost:8080/ws" },
        { source: "/uploads/:path*", destination: "http://localhost:8080/uploads/:path*" },
      ],
    };
  },
};

export default nextConfig;
