import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

// Tests run in Node, but Svelte adapter tests need the client runtime, where
// effects run. Resolve Svelte with its browser export conditions.
const conditions = ['@chatto/source', 'browser'];

export default defineConfig({
  plugins: [svelte({ dynamicCompileOptions: () => ({ generate: 'client' }) })],
  resolve: { conditions },
  ssr: { resolve: { conditions, externalConditions: conditions } },
  test: {
    include: ['src/**/*.test.ts'],
    environment: 'node'
  }
});
