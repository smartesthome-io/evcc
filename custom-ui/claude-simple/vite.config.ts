import { defineConfig } from "vite-plus";
import vue from "@vitejs/plugin-vue";

const backendUrl = `http://localhost:${Number(process.env["VITE_BACKEND_PORT"]) || 7070}`;

export default defineConfig({
  plugins: [vue()],
  base: "./",
  server: {
    port: Number(process.env["VITE_PORT"]) || 7073,
    proxy: {
      "/api": backendUrl,
      "/i18n": backendUrl,
      "/providerauth": backendUrl,
      "/globals.js": backendUrl,
      "/ws": { target: backendUrl.replace("http", "ws"), ws: true },
    },
  },
  build: { outDir: "dist", emptyOutDir: true },
  test: { environment: "node", include: ["src/**/*.test.ts"] },
});
