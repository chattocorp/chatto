import type { APIRequestContext, Page } from '@playwright/test';
import { expect } from '@playwright/test';
import { waitForVerificationCode } from './mailpit';
import type { TestStack } from './stack';

export async function completeSignup(
  page: Page,
  request: APIRequestContext,
  stack: TestStack,
  email: string,
  password: string
): Promise<string> {
  await page.goto('/signup');
  await page.getByLabel('Email address').fill(email);
  await page.getByRole('button', { name: 'Email me a code' }).click();
  const code = await waitForVerificationCode(request, stack.mailpitURL);
  await page.getByLabel('Verification code').fill(code);
  await page.getByRole('button', { name: 'Verify email' }).click();
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByLabel('Confirm password').fill(password);
  await page.getByRole('button', { name: 'Create account' }).click();
  await expect(page.getByRole('heading', { name: 'Your account' })).toBeVisible();
  await expect(page.getByRole('link', { name: `Signed in as ${email}. View your account.` })).toBeVisible();
  const accountID = await page.locator('code').textContent();
  expect(accountID).toMatch(/^acc_[a-z0-9]+$/);
  return accountID ?? '';
}
