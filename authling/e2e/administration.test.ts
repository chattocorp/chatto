// Exercise the admin directory through real signup, sessions, and isolated storage.
import { test as base, expect } from '@playwright/test';
import { startStack, stopStack, type TestStack } from './fixtures/stack';
import { completeSignup } from './fixtures/signup';

const test = base.extend<{ stack: TestStack }>({
  stack: async ({}, use, testInfo) => {
    const stack = await startStack(testInfo, {
      additionalConfig: '\n[administration]\nadmin_emails = ["administrator@example.invalid"]\n'
    });
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

test('limits the user directory to administrators and renders it on small screens', async ({
  page,
  request,
  stack
}) => {
  await page.goto('/admin');
  await expect(page).toHaveURL(/\/login\?return_to=/);
  await completeSignup(
    page,
    request,
    stack,
    'ordinary@example.invalid',
    'a deliberately uncommon password'
  );
  await expect(page.getByRole('link', { name: 'Administration', exact: true })).toHaveCount(0);
  const denied = await page.goto('/admin');
  expect(denied?.status()).toBe(403);
  await expect(page.getByText('ordinary@example.invalid')).toHaveCount(0);
  await page.context().clearCookies();
  await completeSignup(
    page,
    request,
    stack,
    'administrator@example.invalid',
    'a deliberately uncommon password'
  );
  await page.getByRole('link', { name: 'Administration', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Registered users' })).toBeVisible();
  const table = page.getByRole('table', { name: 'Registered users' });
  await expect(table.getByRole('columnheader')).toHaveText([
    'Email',
    'Name',
    'Access',
    'Registered',
    'Account ID'
  ]);
  await expect(table.getByRole('rowheader', { name: /ordinary@example.invalid/ })).toBeVisible();
  await expect(table.getByTitle('Via server configuration', { exact: true })).toBeVisible();
  const row = table.getByRole('row').filter({ hasText: 'ordinary@example.invalid' });
  await expect(row.locator('code')).toBeVisible();
  for (const userRow of await table.locator('tbody tr').all()) {
    expect((await userRow.boundingBox())!.height).toBeLessThanOrEqual(56);
  }
  await page.getByRole('banner').getByRole('link', { name: 'Test Accounts', exact: true }).click();
  await expect(page).toHaveURL(`${stack.baseURL}/account`);
  await page.getByRole('link', { name: 'Administration', exact: true }).click();
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme });
    await page.setViewportSize({ width: 320, height: 740 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true
    );
    await page.getByRole('region', { name: 'Registered users' }).focus();
    await expect(page.getByRole('region', { name: 'Registered users' })).toBeFocused();
  }
  await page.context().clearCookies();
  await page.goto('/admin');
  await page.getByLabel('Email address').fill('administrator@example.invalid');
  await page.getByLabel('Password', { exact: true }).fill('a deliberately uncommon password');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page).toHaveURL(`${stack.baseURL}/admin`);
});
