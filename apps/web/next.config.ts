import type { NextConfig } from "next";
import { config } from "dotenv";
import { resolve } from "path";
import { resolveDevRemoteApiUrl, resolveRemoteApiUrl } from "./config/runtime-urls";

config({ path: resolve(__dirname, "../../.env") });

const remoteApiUrl = process.env.NODE_ENV === "development"
  ? resolveDevRemoteApiUrl(process.env)
  : resolveRemoteApiUrl(process.env);

const nextConfig: NextConfig = {
  ...(process.env.STANDALONE === "true" ? { output: "standalone" as const } : {}),
  allowedDevOrigins: ["127.0.0.1"],
  transpilePackages: ["@dars/core", "@dars/ui", "@dars/views"],
  async rewrites() {
    return remoteApiUrl
      ? [
          { source: "/api/:path*", destination: `${remoteApiUrl}/api/:path*` },
          { source: "/ws", destination: `${remoteApiUrl}/ws` },
          { source: "/auth/:path*", destination: `${remoteApiUrl}/auth/:path*` },
        ]
      : [];
  },
};

export default nextConfig;
