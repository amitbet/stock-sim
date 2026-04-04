import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const pkg = JSON.parse(readFileSync(path.join(__dirname, "package.json"), "utf-8"));

export default defineConfig({
  define: {
    "import.meta.env.VITE_UI_PKG_VERSION": JSON.stringify(pkg.version)
  },
  plugins: [react()],
  server: {
    port: Number(process.env.VITE_PORT || 5173),
    strictPort: true,
    proxy: {
      "/api": {
        target: "http://localhost:3002",
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: "../internal/httpapi/dist",
    emptyOutDir: true
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.js"
  }
});
