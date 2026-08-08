import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  base: "/app/",
  plugins: [react()],
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8090",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test-setup.ts",
  },
});
