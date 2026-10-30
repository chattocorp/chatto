// Exercise the admin directory through real signup, sessions, and isolated storage.
import { test as base, expect } from '@playwright/test';
import { restartAuthling, startStack, stopStack, type TestStack } from './fixtures/stack';
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

for (const javaScriptEnabled of [true, false]) {
  test.describe(`appearance with JavaScript ${javaScriptEnabled ? 'enabled' : 'disabled'}`, () => {
    test.use({ javaScriptEnabled });

    test('previews without publishing and saves across a restart', async ({
      page,
      request,
      stack
    }, testInfo) => {
      await completeSignup(
        page,
        request,
        stack,
        'administrator@example.invalid',
        'a deliberately uncommon password'
      );
      await page.getByRole('link', { name: 'Administration', exact: true }).click();
      await page.getByRole('link', { name: 'Appearance', exact: true }).click();
      await page.evaluate(() => Reflect.set(window, 'appearanceDocument', true));
      await expect(page.getByLabel('Site name', { exact: true })).toHaveValue('Test Accounts');
      await page.getByLabel('Site name', { exact: true }).fill('Our accounts');
      await page.getByLabel('Short description').fill('One identity for our community.');
      await page
        .getByLabel('Sign-in introduction', { exact: true })
        .fill('Welcome <script>alert(1)</script>');
      if (!javaScriptEnabled) {
        await page.getByRole('button', { name: 'Preview', exact: true }).click();
      } else {
        await expect(page.getByRole('button', { name: 'Preview', exact: true })).toBeHidden();
        await expect(page.getByLabel('Sign-in introduction', { exact: true })).toBeFocused();
      }
      if (javaScriptEnabled) {
        expect(await page.evaluate(() => Reflect.get(window, 'appearanceDocument'))).toBe(true);
      }
      const preview = page.getByRole('complementary', { name: 'Sign-in preview' });
      await expect(
        preview.getByText('Welcome <script>alert(1)</script>', { exact: true })
      ).toBeVisible();
      await expect(
        page.getByRole('banner').getByRole('link', { name: 'Test Accounts', exact: true })
      ).toBeVisible();
      expect(await (await request.get(`${stack.baseURL}/login`)).text()).not.toContain(
        'Our accounts'
      );
      await expect(preview.locator('script')).toHaveCount(0);
      if (javaScriptEnabled) {
        await expect(page.getByLabel('Sign-in introduction', { exact: true })).toBeFocused();
        await page
          .getByLabel('Sign-in introduction', { exact: true })
          .fill('Invalid\nintroduction');
        await expect(preview.getByRole('alert')).toBeVisible();
        await expect(page.getByLabel('Sign-in introduction', { exact: true })).toHaveValue(
          'Invalid\nintroduction'
        );
        await page.getByLabel('Sign-in introduction', { exact: true }).fill('');
        await expect(
          preview.getByText('Sign in to Our accounts with your email address and password.')
        ).toBeVisible();
      }
      for (const colorScheme of ['light', 'dark'] as const) {
        await page.emulateMedia({ colorScheme });
        await page.setViewportSize({ width: 375, height: 812 });
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true
        );
      }
      await page.setViewportSize({ width: 1440, height: 1080 });
      const staleEditor = await page.context().newPage();
      await staleEditor.goto(`${stack.baseURL}/admin/appearance`);
      await page
        .getByLabel('Sign-in introduction', { exact: true })
        .fill('Welcome back. Your community is waiting.');
      await page.getByRole('button', { name: 'Save changes', exact: true }).click();
      await expect(page.getByText('Appearance saved.', { exact: true })).toBeVisible();
      await expect(
        page.getByRole('banner').getByRole('link', { name: 'Our accounts', exact: true })
      ).toBeVisible();
      if (javaScriptEnabled) {
        expect(await page.evaluate(() => Reflect.get(window, 'appearanceDocument'))).toBe(true);
      }
      await staleEditor.getByLabel('Site name', { exact: true }).fill('Stale edit');
      await staleEditor.getByRole('button', { name: 'Save changes', exact: true }).click();
      await expect(
        staleEditor.getByText(/Someone changed these settings while you were editing/)
      ).toBeVisible();
      await expect(staleEditor.getByLabel('Site name', { exact: true })).toHaveValue('Stale edit');
      await staleEditor.close();
      await restartAuthling(stack, testInfo);
      await page.context().clearCookies();
      await page.goto('/admin/appearance');
      await expect(page).toHaveURL(/\/login\?return_to=/);
      await expect(
        page.getByText('Welcome back. Your community is waiting.', { exact: true })
      ).toBeVisible();
      await expect(page.locator('meta[name="description"]')).toHaveAttribute(
        'content',
        'One identity for our community.'
      );
      await page.getByLabel('Email address').fill('administrator@example.invalid');
      await page.getByLabel('Password', { exact: true }).fill('a deliberately uncommon password');
      await page.getByRole('button', { name: 'Sign in', exact: true }).click();
      await expect(page).toHaveURL(`${stack.baseURL}/admin/appearance`);
      await expect(page.getByLabel('Site name', { exact: true })).toHaveValue('Our accounts');
    });
  });
}

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
    await page.evaluate(() => document.dispatchEvent(new Event('htmx:afterSettle')));
    await expect(page.getByRole('region', { name: 'Registered users' })).toBeFocused();
  }
  await page.context().clearCookies();
  await page.goto('/admin');
  await page.getByLabel('Email address').fill('administrator@example.invalid');
  await page.getByLabel('Password', { exact: true }).fill('a deliberately uncommon password');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page).toHaveURL(`${stack.baseURL}/admin`);
});
