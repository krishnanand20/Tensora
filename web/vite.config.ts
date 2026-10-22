import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    // `make dev` serves the API on :8080; same-origin in dev avoids CORS entirely.
    proxy: { "/v1": "http://127.0.0.1:8080" },
  },
});
