/// <reference types="vitest/config" />
import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";

// Builds straight into internal/webui/dist/app (never committed) so Go's
// go:embed directive (which can't reach outside its own package directory)
// finds the built assets without a separate copy step. emptyOutDir only
// clears dist/app, leaving the tracked dist/.gitkeep in place.
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: "../internal/webui/dist/app",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/admin": "http://localhost:8090",
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
  },
});
