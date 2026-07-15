/// <reference types="node" />
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: 'e2e',
  testMatch: 'remote-compatibility.compat.ts',
  fullyParallel: false,
  retries: 1,
  workers: 1,
  timeout: 90_000,
  reporter: [
    ['list'],
    ['html', { open: 'never', outputFolder: 'playwright-report-remote-compatibility' }]
  ],
  expect: {
    timeout: 15_000
  },
  use: {
    trace: 'on-first-retry'
  }
});
