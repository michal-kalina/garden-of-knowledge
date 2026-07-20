/** @type {import('next').NextConfig} */
const nextConfig = {
  // Standalone output keeps the Docker image small (no node_modules copy).
  output: "standalone",
  // The browser talks only to the Next.js origin; these rewrites proxy API
  // calls server-side, so no CORS configuration is needed anywhere.
  async rewrites() {
    const api = process.env.API_URL ?? "http://localhost:8080";
    return [{ source: "/backend/:path*", destination: `${api}/:path*` }];
  },
};

export default nextConfig;
