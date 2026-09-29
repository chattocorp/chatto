import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

// Svelte adapter tests need the client runtime, where effects run. Resolve
// Svelte with its browser export conditions and compile test modules for it.
const conditions = ['@chatto/source', 'browser'];

export default defineConfig({
  plugins: [svelte({ dynamicCompileOptions: () => ({ generate: 'client' }) })],
  resolve: { conditions },
  ssr: { resolve: { conditions, externalConditions: conditions } },
  test: {
    include: ['src/**/*.{test,spec}.ts'],
    // Most store tests cover browser behavior, such as the origin server,
    // storage, and page lifecycle. Tests of Node hosts opt into the `node`
    // environment with a `@vitest-environment node` comment.
    environment: 'happy-dom',
    environmentOptions: { happyDOM: { url: 'http://localhost:3000/' } },
    setupFiles: ['./src/testing/setup.ts'],
    testTimeout: 10000,
    // A floor just under the measured coverage. Raise it when coverage grows;
    // do not lower it to make a change pass.
    coverage: {
      include: ['src/**/*.ts'],
      exclude: [
        'src/**/*.spec.ts',
        'src/**/*.test.ts',
        'src/testing/**',
        'src/types/**',
        'src/types.ts'
      ],
      provider: 'v8',
      reporter: ['text-summary'],
      thresholds: {
        branches: 84,
        functions: 95,
        lines: 95,
        statements: 92
      }
    }
  }
});
