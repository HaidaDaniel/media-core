import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// The dev server proxies the API to the local Go server; the production
// build is static and is served same-origin by the Go binary (task 0047).
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8787",
        changeOrigin: false,
      },
    },
  },
  test: {
    environment: "jsdom",
  },
  build: {
    outDir: "dist",
  },
});
