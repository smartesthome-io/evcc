import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

const backend = `http://127.0.0.1:${process.env.VITE_BACKEND_PORT || 7070}`;

export default defineConfig({
  plugins: [vue()],
  base: "./",
  build: { outDir: "dist", emptyOutDir: true },
  server: {
    port: Number(process.env.VITE_PORT || 7072),
    proxy: {
      "/api": backend,
      "/i18n": backend,
      "/providerauth": backend,
      "/globals.js": backend,
      "/ws": { target: backend, ws: true },
    },
  },
});
