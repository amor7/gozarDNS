import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";

// Two apps share one toolchain: `--mode admin` builds the hub's console and
// `--mode portal` the customer portal. Each lands in internal/web/dist/<app>,
// which the Go binary embeds.
export default defineConfig(({ mode }) => {
  const app = mode === "portal" ? "portal" : "admin";
  const backend = process.env.GOZAR_DEV_BACKEND || (app === "admin" ? "https://127.0.0.1:12053" : "https://127.0.0.1:18843");
  return {
    root: fileURLToPath(new URL(`./src/${app}`, import.meta.url)),
    base: "/",
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: { "@shared": fileURLToPath(new URL("./src/shared", import.meta.url)) },
    },
    build: {
      outDir: fileURLToPath(new URL(`../internal/web/dist/${app}`, import.meta.url)),
      emptyOutDir: true,
      assetsInlineLimit: 0,
      chunkSizeWarningLimit: 800,
    },
    server: {
      port: app === "admin" ? 5173 : 5174,
      proxy: { "/api": { target: backend, changeOrigin: true, secure: false } },
    },
  };
});
