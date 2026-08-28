import { test as base } from '@playwright/test';
import { startStack, stopStack, type TestStack } from './fixtures/stack';

export const test = base.extend<{ stack: TestStack }>({
  stack: async ({}, use, testInfo) => {
    const stack = await startStack(testInfo);
    try {
      await use(stack);
    } finally {
      await stopStack(stack, testInfo);
    }
  },
  baseURL: async ({ stack }, use) => {
    await use(stack.baseURL);
  }
});

export { expect } from '@playwright/test';
