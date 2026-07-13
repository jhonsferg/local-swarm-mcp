/// <reference types="vitest/config" />
import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

// Builds straight into internal/webui/dist so Go's go:embed directive
// (which can't reach outside its own package directory) finds the built
// assets without a separate copy step.
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: "../internal/webui/dist",
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
