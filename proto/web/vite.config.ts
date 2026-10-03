import { svelte } from "@sveltejs/vite-plugin-svelte";
import { defineConfig } from "vite";

// In development the Go server (proto/server) runs on :8080 and serves the
// relay websocket and /api; Vite proxies both so the app sees one origin.
const server = process.env.ALHO_SERVER ?? "http://localhost:8080";

export default defineConfig({
  plugins: [svelte()],
  server: {
    proxy: {
      "/api": { target: server },
      "/relay": { target: server.replace(/^http/, "ws"), ws: true, rewrite: () => "/" },
    },
  },
});
