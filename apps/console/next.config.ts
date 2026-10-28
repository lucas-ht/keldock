import type { NextConfig } from "next";
import { fileURLToPath } from "node:url";

const upstream = process.env.KELDOCK_API_ORIGIN ?? "http://127.0.0.1:3801";
const parsed = new URL(upstream);
if (
  parsed.username ||
  parsed.password ||
  parsed.search ||
  parsed.hash ||
  parsed.pathname !== "/"
) {
  throw new Error(
    "KELDOCK_API_ORIGIN must be an origin without credentials or a path",
  );
}

const config: NextConfig = {
  distDir: process.env.KELDOCK_NEXT_DIST_DIR ?? ".next",
  output: "standalone",
  outputFileTracingRoot: fileURLToPath(new URL("../..", import.meta.url)),
  poweredByHeader: false,
  devIndicators: false,
  async rewrites() {
    return ["/v1/:path*", "/oauth/:path*", "/.well-known/:path*", "/mcp"].map(
      (source) => ({
        source,
        destination: `${parsed.origin}${source}`,
      }),
    );
  },
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "Referrer-Policy", value: "no-referrer" },
          { key: "X-Frame-Options", value: "DENY" },
          {
            key: "Permissions-Policy",
            value: "camera=(), microphone=(), geolocation=()",
          },
        ],
      },
    ];
  },
};
export default config;
