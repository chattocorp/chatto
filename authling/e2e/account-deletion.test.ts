import { randomUUID } from 'node:crypto';
import { completeSignup } from './fixtures/signup';
import { restartAuthling } from './fixtures/stack';
import { expect, test } from './setup';

const password = 'correct horse battery staple';

test('deletes an account, denies other browsers, and permits a new identity after restart', async ({ browser, page, request, stack }, testInfo) => {
  const email = `erase-${randomUUID()}@example.invalid`;
  const oldID = await completeSignup(page, request, stack, email, password);
  const otherContext = await browser.newContext({ baseURL: stack.baseURL });
  const otherPage = await otherContext.newPage();
  await otherPage.goto('/login');
  await otherPage.getByLabel('Email address').fill(email);
  await otherPage.getByLabel('Password').fill(password);
  await otherPage.getByRole('button', { name: 'Sign in' }).click();
  await expect(otherPage.getByRole('heading', { name: 'Your account' })).toBeVisible();

  await page.getByRole('link', { name: 'Delete account', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Delete your account' })).toBeVisible();
  await expect(page.getByText('This does not delete your data or end sessions in other apps.', { exact: false })).toBeVisible();
  await page.getByLabel('Current password').fill('incorrect password');
  await page.getByRole('checkbox').check();
  await page.getByRole('button', { name: 'Permanently delete account' }).click();
  await expect(page.getByRole('alert')).toContainText('The password is incorrect');
  await page.getByLabel('Current password').fill(password);
  await page.getByRole('checkbox').check();
  await page.getByRole('button', { name: 'Permanently delete account' }).click();
  await expect(page.getByRole('heading', { name: 'Your account is closed' })).toBeVisible();

  await otherPage.goto('/account');
  await expect(otherPage).toHaveURL(/\/login$/);
  await otherContext.close();
  await restartAuthling(stack, testInfo);
  await page.goto('/login');
  await page.getByLabel('Email address').fill(email);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('alert')).toHaveText('The email address or password is incorrect.');
  const newID = await completeSignup(page, request, stack, email, password);
  expect(newID).not.toBe(oldID);
  await restartAuthling(stack, testInfo);
  await page.goto('/account');
  await expect(page.locator('code')).toHaveText(newID);
});
