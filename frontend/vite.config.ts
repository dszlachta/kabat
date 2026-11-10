import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import wails from "@wailsio/runtime/plugins/vite";

const root = fileURLToPath(new URL(".", import.meta.url));

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [vue(), wails("./bindings")],
  build: {
    rollupOptions: {
      input: {
        main: resolve(root, "index.html"),
        confirm: resolve(root, "confirm.html"),
        connect: resolve(root, "connect.html"),
        settings: resolve(root, "settings.html"),
      },
    },
  },
});
