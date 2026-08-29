/// <reference types="node" />
import { defineConfig } from '@playwright/test';

export default defineConfig({
  globalSetup: './e2e/global-setup.ts',
  testDir: 'e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: [['list'], ['html', { open: 'never', outputFolder: 'playwright-report' }]],
  maxFailures: 5,
  timeout: 30_000,
  workers: 4,
  expect: {
    timeout: 15_000
  },
  use: {
    trace: 'on-first-retry'
  }
});
