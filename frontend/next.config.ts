import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // standalone output is for Docker image packaging; works in Linux build env.
  // On Windows local dev, the final symlink-copy step fails with EPERM but the
  // compile itself succeeds, so `pnpm dev` and `pnpm start` still work locally.
  output: "standalone",
  reactStrictMode: true,
};

export default nextConfig;