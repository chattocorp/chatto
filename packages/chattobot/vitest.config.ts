import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    include: ['**/*.test.ts'],
    exclude: ['**/node_modules/**', '**/.runling/**'],
    testTimeout: 15000,
    setupFiles: ['./test-setup.ts'],
    fileParallelism: false
  }
});
