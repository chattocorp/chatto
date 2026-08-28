/// <reference types="node" />
import { defineConfig } from '@playwright/test';

export default defineConfig({
  globalSetup: './e2e/global-setup.ts',
  testDir: 'e2e-performance',
  fullyParallel: false,
  retries: 0,
  workers: 1,
  timeout: 30 * 60_000,
  expect: {
    timeout: 30_000
  },
  outputDir: 'test-results/performance',
  reporter: [['list'], ['html', { open: 'never', outputFolder: 'playwright-report-performance' }]],
  use: {
    trace: 'retain-on-failure'
  }
});
