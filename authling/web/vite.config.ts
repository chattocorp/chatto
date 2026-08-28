import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('.', import.meta.url));

export default defineConfig({
  base: '/assets/',
  plugins: [tailwindcss()],
  build: {
    emptyOutDir: true,
    outDir: resolve(root, '../internal/web/assets'),
    rollupOptions: {
      input: resolve(root, 'src/app.css'),
      output: {
        assetFileNames: '[name][extname]'
      }
    }
  }
});
