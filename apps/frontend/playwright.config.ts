/// <reference types="node" />
import { defineConfig, type ReporterDescription } from '@playwright/test';

const reporter: ReporterDescription[] = [
  ['list'],
  ['html', { open: 'never', outputFolder: 'playwright-report' }]
];
// Keep machine-readable timings when CI or a local comparison requests them.
if (process.env.PLAYWRIGHT_JSON_OUTPUT_FILE) reporter.push(['json']);

export default defineConfig({
  globalSetup: './e2e/global-setup.ts',
  testDir: 'e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter,
  maxFailures: 5,
  timeout: 30_000,
  // Hosted runners have enough idle time between browser interactions to
  // support more workers than local development without adding CI shards.
  workers: process.env.CI ? 6 : 4,
  expect: {
    timeout: 15_000
  },
  use: {
    trace: 'on-first-retry'
  }
});
