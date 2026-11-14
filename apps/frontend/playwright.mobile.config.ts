import { defineConfig } from '@playwright/test';
import config from './playwright.config';

/** Run the same sidebar touch scenario on desktop engines with touch emulation. */
export default defineConfig({
  ...config,
  testMatch: 'mobile-sidebar-click-through.test.ts',
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
    { name: 'firefox', use: { browserName: 'firefox' } }
  ]
});
