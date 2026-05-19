/** @type {import('next').NextConfig} */
// Static export so the Go binary can serve frontend/out as plain static files
// (locked decision #4). All API traffic same-origin through `/api/v1`.
const nextConfig = {
  output: 'export',
  reactStrictMode: true,
  images: {
    // Static export cannot run the image optimiser at runtime.
    unoptimized: true,
  },
  trailingSlash: false,
  // We intentionally do NOT set basePath/assetPrefix; same-origin only.
};

module.exports = nextConfig;
