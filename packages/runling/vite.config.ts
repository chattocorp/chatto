import { sveltekit } from "@sveltejs/kit/vite";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [tailwindcss(), sveltekit()],
  ssr: { external: ["runling", "runling/agents", "runling/web", "runling/config-reloader"] },
  // Config files and the server must share one Runling event runtime.
  build: {
    rollupOptions: {
      external: ["runling", "runling/agents", "runling/web", "runling/config-reloader"],
    },
  },
});
